package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/logger"
)

const missThreshold = 3

// 单位转换表：CK 原始值乘以该系数后，得到评分引擎期望的单位
var unitConvert = map[string]float64{
	"DCGM_FI_DEV_THERMAL_VIOLATION":     1e-6, // μs → s
	"DCGM_FI_DEV_POWER_VIOLATION":       1e-6, // μs → s
	"DCGM_FI_DEV_LOW_UTIL_VIOLATION":    1e-6, // μs → s
	"DCGM_FI_DEV_BOARD_LIMIT_VIOLATION": 1e-6,
	"DCGM_FI_DEV_SYNC_BOOST_VIOLATION":  1e-6,
	"DCGM_FI_DEV_RELIABILITY_VIOLATION": 1e-6,
}

type CKLoaderService struct {
	cfg          config.CKConfig
	ck           *ckclient.Client
	topo         *repository.TopologyRepo
	metricRepo   *repository.MetricRepo
	strategyRepo *repository.StrategyRepo
	clusterCache map[string]uint64
	nodeCache    map[string]uint64
	missCount    map[string]int

	// counter 增量计算
	prevValues  map[string]map[string]float64 // uuid -> metric_key -> 上轮原始值
	counterKeys map[string]bool               // counter 类型的 metric_key 集合
}

func NewCKLoaderService(
	cfg config.CKConfig,
	ck *ckclient.Client,
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
		missCount:    map[string]int{},
		prevValues:   map[string]map[string]float64{},
	}
}

func gpuUUID(sn, tags string) string { return sn + ":" + tags }

func normalizeMIB(mib string) string { return mib }

// detectVendor 根据指标前缀判断卡的厂商
func detectVendor(metrics map[string]float64) string {
	dcgm, npu := 0, 0
	for key := range metrics {
		if strings.HasPrefix(key, "DCGM_FI_") {
			dcgm++
		}
		if strings.HasPrefix(key, "npu_chip_") || strings.HasPrefix(key, "container_npu_") {
			npu++
		}
	}
	if npu > dcgm {
		return "huawei"
	}
	if dcgm > 0 {
		return "nvidia"
	}
	return "unknown"
}

// loadCounterKeys 从 DB 加载所有 counter 类型的指标 key（启动时 + 定期刷新）
func (s *CKLoaderService) loadCounterKeys() {
	var defs []model.MetricDefinition
	if err := s.metricRepo.DB().Where("metric_type = ?", "counter").Find(&defs).Error; err != nil {
		logger.L.Warnf("加载 counter 类指标失败: %v", err)
		return
	}
	m := make(map[string]bool, len(defs))
	for _, d := range defs {
		m[d.MetricKey] = true
	}
	s.counterKeys = m
}

// applyDelta 对 counter 类指标计算增量，并更新 prevValues
func (s *CKLoaderService) applyDelta(frames map[string]*redisclient.MetricFrame) {
	if s.counterKeys == nil || len(s.counterKeys) == 0 {
		return
	}

	newPrev := make(map[string]map[string]float64, len(frames))
	for uuid, f := range frames {
		prev := s.prevValues[uuid]
		raw := make(map[string]float64, len(f.Metrics))

		for key, val := range f.Metrics {
			raw[key] = val // 保存原始值

			if !s.counterKeys[key] {
				continue
			}

			if prev != nil {
				if oldVal, ok := prev[key]; ok {
					delta := val - oldVal
					if delta < 0 {
						delta = 0 // counter 重置保护
					}
					f.Metrics[key] = delta
				} else {
					f.Metrics[key] = 0
				}
			} else {
				f.Metrics[key] = 0
			}
		}
		newPrev[uuid] = raw
	}
	s.prevValues = newPrev
}

func (s *CKLoaderService) Collect(ctx context.Context) ([]redisclient.MetricFrame, error) {
	window := time.Duration(s.cfg.WindowSec) * time.Second
	if window <= 0 {
		window = 5 * time.Minute
	}
	rows, err := s.ck.LatestSamples(ctx, s.cfg.Table, window)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		logger.L.Warn("CK 近窗口无数据")
		return nil, nil
	}

	// 首次或定期刷新 counter keys
	if s.counterKeys == nil {
		s.loadCounterKeys()
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
		val := r.Value
		if factor, ok := unitConvert[mib]; ok {
			val *= factor
		}
		f.Metrics[mib] = val
		liveKeys[mib] = struct{}{}
	}

	// counter 增量计算（在单位转换之后、拓扑同步之前）
	s.applyDelta(frames)

	nodeGPUCount := map[string]int{}
	for _, m := range metas {
		nodeGPUCount[m.sn]++
	}

	// 同步拓扑 + 写入 Vendor
	for uuid, m := range metas {
		clusterID, err := s.ensureCluster(m.source)
		if err != nil {
			logger.L.Warnf("cluster %s: %v", m.source, err)
			continue
		}
		nodeID, err := s.ensureNode(clusterID, m.sn, m.ip, nodeGPUCount[m.sn])
		if err != nil {
			logger.L.Warnf("node %s: %v", m.sn, err)
			continue
		}
		idx, _ := strconv.Atoi(m.tags)
		vendor := detectVendor(frames[uuid].Metrics)
		if err := s.topo.UpsertGPU(&model.GPUCard{
			UUID: uuid, NodeID: nodeID, ClusterID: clusterID,
			GPUIndex: idx, SN: m.sn, Status: "online", Vendor: vendor,
		}); err != nil {
			logger.L.Warnf("upsert gpu %s: %v", uuid, err)
		}
	}

	s.syncMetricHealthKey(liveKeys)

	list := make([]redisclient.MetricFrame, 0, len(frames))
	for _, f := range frames {
		list = append(list, *f)
	}
	logger.L.Infof("CK 采集完成：%d 张卡", len(list))
	return list, nil
}

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

	var enableKeys []string
	for k := range liveKeys {
		if _, ok := definedSet[k]; ok {
			enableKeys = append(enableKeys, k)
			s.missCount[k] = 0
		} else {
			logger.L.Warnf("CK 出现未定义指标 [%s]，请到指标系统补充 dimension 等元数据", k)
		}
	}

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
		for _, k := range disableKeys {
			s.missCount[k] = 0
		}
	}
}

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
	c := model.Cluster{Code: source, Name: source}
	if err := s.topo.DB().Where("code = ?", source).FirstOrCreate(&c).Error; err != nil {
		return 0, err
	}
	s.clusterCache[source] = c.ID
	return c.ID, nil
}

func (s *CKLoaderService) ensureNode(clusterID uint64, sn, ip string, gpuCount int) (uint64, error) {
	if id, ok := s.nodeCache[sn]; ok {
		s.topo.DB().Model(&model.Node{}).Where("hostname = ?", sn).
			Update("gpu_count", gpuCount)
		return id, nil
	}
	n := model.Node{ClusterID: clusterID, Hostname: sn, IP: ip, GPUCount: gpuCount}
	if err := s.topo.DB().Where("hostname = ?", sn).
		Assign(map[string]any{"gpu_count": gpuCount, "ip": ip}).
		FirstOrCreate(&n).Error; err != nil {
		return 0, err
	}
	s.nodeCache[sn] = n.ID
	return n.ID, nil
}
