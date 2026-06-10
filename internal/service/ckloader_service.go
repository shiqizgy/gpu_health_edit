package service

import (
	"context"
	"strconv"
	"time"

	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/logger"
)

// 滑动窗口：指标连续 missThreshold 轮未在 CK 出现，才关闭 is_health_key
// 防止 CK 偶发漏数据导致评分指标抖动
const missThreshold = 3

//  从ClickHouse加载GPU采样指标，完成两件事：
//  1. 同步拓扑：根据指标中的source/sn/ip/tags信息，确保MySQL中集群（Cluster）、节点（Node）、GPU卡（GPUCard）记录存在（幂等）；
//  2. 写入Redis：将指标以MetricFrame形式写入Redis，供评分服务读取。
//  内部维护clusterCache/nodeCache两级内存缓存，避免每次重复查询数据库。

type CKLoaderService struct {
	cfg          config.CKConfig
	ck           *ckclient.Client
	redis        *redisclient.Client
	topo         *repository.TopologyRepo
	metricRepo   *repository.MetricRepo
	strategyRepo *repository.StrategyRepo
	clusterCache map[string]uint64 //source->cluster_id
	nodeCache    map[string]uint64 //source->node_id

	//指标动态同步状态
	missCount map[string]int //metric_key -> 连续未出现轮数
}

// 创建CKLoaderService实例

func NewCKLoaderService(
	cfg config.CKConfig,
	ck *ckclient.Client,
	rc *redisclient.Client,
	topo *repository.TopologyRepo,
	metricRepo *repository.MetricRepo,
	strategyRepo *repository.StrategyRepo) *CKLoaderService {
	return &CKLoaderService{
		cfg:          cfg,
		ck:           ck,
		redis:        rc,
		topo:         topo,
		metricRepo:   metricRepo,
		strategyRepo: strategyRepo,
		clusterCache: map[string]uint64{},
		nodeCache:    map[string]uint64{},
		missCount:    map[string]int{}}
}

// gpuUUID根据设备序列号和标签生成GPU全局唯一标识，格式为"sn:tags"。
// 该UUID在Redis key、拓扑表、评分快照中作为主键关联使用。
func gpuUUID(sn, tags string) string {
	return sn + ":" + tags
}

// normalizeMIB对MIB指标名称做标准化映射。
// 当前为占位实现（直接返回原名），若ClickHouse中使用短名，
// 可在此处添加映射表将短名转为标准名。
func normalizeMIB(mib string) string { return mib }

//  LoadOnce执行一次完整的CK数据加载流程，包含以下步骤：
//  1. 从ClickHouse查询最近window时间窗口内的最新采样数据（window由配置WindowSec 决定，默认 5 分钟）；
//  2. 按GPU UUID 分组，将同一张卡的多个指标合并为一个MetricFrame；
//  3. 同步拓扑：遍历每张卡，确保对应的Cluster、Node、GPUCard记录在MySQL中存在（不存在则创建），实现动态扩容；
//  4. 将所有 MetricFrame 批量写入 Redis（Pipeline 模式），带 TTL 防止脏数据堆积。
//  参数：
//    - ctx: 上下文，用于控制超时和取消
//  返回：成功时返回nil，失败时返回具体错误。
//  若 CK 近窗口无数据，仅打印警告日志并返回 nil（不视为错误）

func (s *CKLoaderService) LoadOnce(ctx context.Context) error {
	window := time.Duration(s.cfg.WindowSec) * time.Second
	if window <= 0 {
		window = 5 * time.Minute
	}
	rows, err := s.ck.LatestSamples(ctx, s.cfg.Table, window)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		logger.L.Warn("CK 近窗口无数据")
		return nil
	}

	type meta struct{ source, sn, ip, tags string }
	frames := map[string]*redisclient.MetricFrame{}
	metas := map[string]meta{}
	liveKeys := map[string]struct{}{}
	now := time.Now().Unix()
	for _, r := range rows {
		uuid := gpuUUID(r.SN, r.Tags)
		f, ok := frames[uuid]
		if !ok {
			f = &redisclient.MetricFrame{UUID: uuid, TS: now, Metrics: map[string]float64{}}
			frames[uuid] = f
			metas[uuid] = meta{r.Source, r.SN, r.IP, r.Tags}
		}
		mib := normalizeMIB(r.MIB)
		f.Metrics[normalizeMIB(r.MIB)] = r.Value
		liveKeys[mib] = struct{}{}
	}

	// 1) 同步拓扑：source→cluster, sn→node, sn:tags→gpu
	for uuid, m := range metas {
		clusterID, err := s.ensureCluster(m.source)
		if err != nil {
			logger.L.Warnf("cluster %s: %v", m.source, err)
			continue
		}
		nodeID, err := s.ensureNode(clusterID, m.sn, m.ip)
		if err != nil {
			logger.L.Warnf("node %s: %v", m.sn, err)
			continue
		}
		idx, _ := strconv.Atoi(m.tags)
		if err := s.topo.UpsertGPU(&model.GPUCard{
			UUID: uuid, NodeID: nodeID, ClusterID: clusterID,
			GPUIndex: idx, SN: m.sn, Status: "online", // ← 新增 SN: m.sn
		}); err != nil {
			logger.L.Warnf("upsert gpu %s: %v", uuid, err)
		}
	}

	//同步metric_defination.is_health_key状态
	s.syncMetricHealthKey(liveKeys)

	// 2) 写Redis（和simulator完全相同的通道）
	ttl := time.Duration(s.cfg.MetricTTL) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	list := make([]redisclient.MetricFrame, 0, len(frames))
	for _, f := range frames {
		list = append(list, *f)
	}
	if err := s.redis.WriteFramePipeline(ctx, list, ttl); err != nil {
		return err
	}
	logger.L.Infof("CK 接入完成：%d 张卡", len(list))
	return nil
}

// syncMetricHealthKey根据本轮CK实测到的指标，动态调整metric_definition.is_health_key。
// 策略：
//   - liveKeys 中存在 + 已有定义 → is_health_key = true（立即启用）
//   - 已有定义但不在 liveKeys 中 → missCount[key]++，达到阈值才置 false（抖动保护）
//   - liveKeys 中存在但无定义 → 仅打日志，不自动创建（缺少 dimension/曲线信息）
func (s *CKLoaderService) syncMetricHealthKey(liveKeys map[string]struct{}) {
	definedKeys, err := s.metricRepo.ListAllKeys()
	if err != nil {
		logger.L.Warnf("查询 metric_definition 失败: %v", err)
		return
	}
	definedSet := make(map[string]struct{}, len(definedKeys))
	for _, k := range definedKeys {
		definedSet[k] = struct{}{}
	}

	// 需要启用评分的指标
	var enableKeys []string
	for k := range liveKeys {
		if _, ok := definedSet[k]; ok {
			enableKeys = append(enableKeys, k)
			s.missCount[k] = 0 // 命中即清零
		} else {
			// CK 出现了未定义的新指标，告警但不自动创建
			logger.L.Warnf("CK 出现未定义指标 [%s]，请到指标系统补充 dimension 等元数据", k)
		}
	}

	// 已定义但本轮未出现的指标：累计 miss 次数
	var disableKeys []string
	for _, k := range definedKeys {
		if _, alive := liveKeys[k]; alive {
			continue
		}
		s.missCount[k]++
		if s.missCount[k] >= missThreshold {
			disableKeys = append(disableKeys, k)
		}
	}

	// 批量执行
	if n, err := s.metricRepo.UpdateHealthKeyByMetricKeys(enableKeys, true); err != nil {
		logger.L.Warnf("启用指标评分失败: %v", err)
	} else if n > 0 {
		logger.L.Infof("启用 %d 个指标参与评分: %v", n, enableKeys)
		s.bumpStrategiesByKeys(enableKeys)
	}

	if n, err := s.metricRepo.UpdateHealthKeyByMetricKeys(disableKeys, false); err != nil {
		logger.L.Warnf("关闭指标评分失败: %v", err)
	} else if n > 0 {
		logger.L.Infof("关闭 %d 个指标参与评分(连续 %d 轮未出现): %v",
			n, missThreshold, disableKeys)
		s.bumpStrategiesByKeys(disableKeys)
		// 关闭后清零 missCount，避免恢复出现时误判
		for _, k := range disableKeys {
			s.missCount[k] = 0
		}
	}
}

// bumpStrategiesByKeys 让评分服务下轮热加载策略
func (s *CKLoaderService) bumpStrategiesByKeys(keys []string) {
	for _, k := range keys {
		if err := s.strategyRepo.BumpVersionByMetricKey(k); err != nil {
			logger.L.Warnf("BumpVersion[%s]失败: %v", k, err)
		}
	}
}

func (s *CKLoaderService) ensureCluster(source string) (uint64, error) {
	if id, ok := s.clusterCache[source]; ok {
		return id, nil
	}
	c := &model.Cluster{Code: source, Name: source}
	if err := s.topo.CreateCluster(c); err != nil {
		var ex model.Cluster
		if e := s.topo.DB().Where("code=?", source).First(&ex).Error; e != nil {
			return 0, e
		}
		c.ID = ex.ID
	}
	s.clusterCache[source] = c.ID
	return c.ID, nil
}

func (s *CKLoaderService) ensureNode(clusterID uint64, sn, ip string) (uint64, error) {
	if id, ok := s.nodeCache[sn]; ok {
		return id, nil
	}
	n := &model.Node{ClusterID: clusterID, Hostname: sn, IP: ip, GPUCount: 8}
	if err := s.topo.CreateNode(n); err != nil {
		var ex model.Node
		if e := s.topo.DB().Where("hostname=?", sn).First(&ex).Error; e != nil {
			return 0, e
		}
		n.ID = ex.ID
	}
	s.nodeCache[sn] = n.ID
	return n.ID, nil
}
