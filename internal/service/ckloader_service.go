package service

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/scoring"
	"github.com/gpu-health/platform/internal/types"
	"github.com/gpu-health/platform/pkg/logger"
	"gorm.io/gorm/clause"
)

const missThreshold = 3

var unitConvert = map[string]float64{
	"DCGM_FI_DEV_THERMAL_VIOLATION":     1e-6,
	"DCGM_FI_DEV_POWER_VIOLATION":       1e-6,
	"DCGM_FI_DEV_LOW_UTIL_VIOLATION":    1e-6,
	"DCGM_FI_DEV_BOARD_LIMIT_VIOLATION": 1e-6,
	"DCGM_FI_DEV_SYNC_BOOST_VIOLATION":  1e-6,
	"DCGM_FI_DEV_RELIABILITY_VIOLATION": 1e-6,
}

// nodeEntry 缓存节点 ID 和 gpuCount，避免每轮无谓 UPDATE
type nodeEntry struct {
	id       uint64
	gpuCount int
}

// counterMeta 累计类指标的速率换算信息
type counterMeta struct {
	windowSeconds float64 // 目标单位对应的时间窗口秒数(次/天=86400，μs/s=1)
	valueScale    float64 // 值换算(μs→s=1e-6，其余1)
	useWindow     bool    // true=窗口累计(低频)，false=瞬时速率(高频)
}

// deltaSample 一次采样的增量点(用于滑动窗口累计)
type deltaSample struct {
	ts    int64   // 采样时刻(秒)
	delta float64 // 该次采样相对上次的增量(已乘 valueScale)
}

type CKLoaderService struct {
	cfg          config.CKConfig
	ck           *ckclient.Client
	topo         *repository.TopologyRepo
	metricRepo   *repository.MetricRepo
	strategyRepo *repository.StrategyRepo
	clusterCache map[string]uint64
	nodeCache    map[string]nodeEntry // 改：记录 id + gpuCount
	missCount    map[string]int

	prevValues  map[string]map[string]float64       //上次原始累计值（用于计算delta）
	prevTS      map[string]int64                    //每张卡上次采样的时间戳
	counterKeys map[string]counterMeta              //metricKey -> 换算元数据
	deltaHist   map[string]map[string][]deltaSample // uuid -> metricKey -> 增量历史(滑动窗口)

	sampledUUIDs map[string]struct{} //gpu_limit 固定抽样集合（nil=未初始化/全量）
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
		topo:         topo,
		metricRepo:   metricRepo,
		strategyRepo: strategyRepo,
		clusterCache: map[string]uint64{},
		nodeCache:    map[string]nodeEntry{},
		missCount:    map[string]int{},
		prevValues:   map[string]map[string]float64{},
		prevTS:       map[string]int64{},
		deltaHist:    map[string]map[string][]deltaSample{},
	}
}

func gpuUUID(sn, tags string) string {
	sn, tags = strings.TrimSpace(sn), strings.TrimSpace(tags)
	if sn == "" || tags == "" {
		return ""
	}
	return sn + ":" + tags
}

func normalizeMIB(mib string) string { return mib }

// detectDevice 返回 (vendor, cardType)
func detectDevice(metrics map[string]float64) (string, string) {
	dcgm, npu := 0, 0
	for key := range metrics {
		switch {
		case strings.HasPrefix(key, "DCGM_"):
			dcgm++
		case strings.HasPrefix(key, "npu_"), strings.HasPrefix(key, "container_npu_"):
			npu++
		}
	}
	if npu > dcgm && npu > 0 {
		return "huawei", "NPU"
	}
	if dcgm > 0 {
		return "nvidia", "GPU"
	}
	return "unknown", "" // ★ 空字符串 = 判不出来
}

func (s *CKLoaderService) loadCounterKeys() {
	var defs []model.MetricDefinition
	if err := s.metricRepo.DB().Where("value_type_code IN ?", []int{3, 4, 5}).Where("is_health_key = ?", true).Find(&defs).Error; err != nil {
		logger.L.Warnf("加载 counter 类指标失败: %v", err)
		return
	}
	m := make(map[string]counterMeta, len(defs))
	var levelKeys []string
	for _, d := range defs {
		sec, scale, isRate := scoring.ParseRateUnit(d.NormalRateUnit)
		if !isRate {
			levelKeys = append(levelKeys, d.MetricName)
			continue // ★ 存量型不入 counterKeys，保持原始累计值
		}
		m[d.MetricName] = counterMeta{
			valueScale:    scale,
			windowSeconds: sec,
			useWindow:     sec >= 60, // ≥1分钟按滑动窗口累计，否则按每秒速率
		}
	}
	s.counterKeys = m
	logger.L.Infof("counter 指标加载完成：速率型 %d 个，存量型 %d 个（不做增量转换）：%v",
		len(m), len(levelKeys), levelKeys)
}

// 将“累计计数”类指标转换为“增长速率”
func (s *CKLoaderService) applyDelta(frames map[string]*types.MetricFrame) {
	if len(s.counterKeys) == 0 {
		return
	}
	newPrev := make(map[string]map[string]float64, len(frames))
	newPrevTS := make(map[string]int64, len(frames))

	for uuid, f := range frames {
		prev := s.prevValues[uuid]
		prevTS := s.prevTS[uuid]
		intervalSec := float64(f.TS - prevTS)
		raw := make(map[string]float64, len(f.Metrics))

		for key, val := range f.Metrics {
			raw[key] = val
			meta, isCounter := s.counterKeys[key]
			if !isCounter {
				continue
			}

			// 计算本次原始增量
			var delta float64
			if prev != nil {
				if oldVal, ok := prev[key]; ok {
					delta = val - oldVal
					if delta < 0 {
						delta = 0 // 计数器重置
					}
				}
			}
			delta *= meta.valueScale

			if meta.useWindow {
				// 低频累计：把本次增量入历史，求滑动窗口内总增量
				f.Metrics[key] = s.windowSum(uuid, key, f.TS, delta, meta.windowSeconds)
			} else {
				// 高频速率：delta ÷ 采样间隔 × 单位秒数(=1)
				if prev == nil || intervalSec <= 0 {
					f.Metrics[key] = 0
				} else {
					f.Metrics[key] = delta / intervalSec * meta.windowSeconds
				}
			}
		}
		newPrev[uuid] = raw
		newPrevTS[uuid] = f.TS
	}
	s.prevValues = newPrev
	s.prevTS = newPrevTS
}

// 关于npu中通道等指标的聚合
var multiLaneGroups = map[string][]string{
	"npu_chip_info_hccs_bandwidth_info_tx_max": { //HCCS链路单链路发送带宽
		"npu_chip_info_hccs_bandwidth_info_tx_1",
		"npu_chip_info_hccs_bandwidth_info_tx_2",
		"npu_chip_info_hccs_bandwidth_info_tx_3",
		"npu_chip_info_hccs_bandwidth_info_tx_4",
		"npu_chip_info_hccs_bandwidth_info_tx_5",
		"npu_chip_info_hccs_bandwidth_info_tx_6",
		"npu_chip_info_hccs_bandwidth_info_tx_7",
	},
	"npu_chip_info_hccs_bandwidth_info_rx_max": { //HCCS链路单链路接收带宽
		"npu_chip_info_hccs_bandwidth_info_rx_1",
		"npu_chip_info_hccs_bandwidth_info_rx_2",
		"npu_chip_info_hccs_bandwidth_info_rx_3",
		"npu_chip_info_hccs_bandwidth_info_rx_4",
		"npu_chip_info_hccs_bandwidth_info_rx_5",
		"npu_chip_info_hccs_bandwidth_info_rx_6",
		"npu_chip_info_hccs_bandwidth_info_rx_7",
	},
	"npu_chip_optical_tx_power_max": { //光模块通道发送光功率
		"npu_chip_optical_tx_power_0",
		"npu_chip_optical_tx_power_1",
		"npu_chip_optical_tx_power_2",
		"npu_chip_optical_tx_power_3",
	},
	"npu_chip_optical_rx_power_max": { //光模块通道接收光功率
		"npu_chip_optical_rx_power_0",
		"npu_chip_optical_rx_power_1",
		"npu_chip_optical_rx_power_2",
		"npu_chip_optical_rx_power_3",
	},
	"npu_chip_info_hccs_statistic_info_tx_cnt_max": { //HCCS链路发送报文数(采集失败-i)
		"npu_chip_info_hccs_statistic_info_tx_cnt_1",
		"npu_chip_info_hccs_statistic_info_tx_cnt_2",
		"npu_chip_info_hccs_statistic_info_tx_cnt_3",
		"npu_chip_info_hccs_statistic_info_tx_cnt_4",
		"npu_chip_info_hccs_statistic_info_tx_cnt_5",
		"npu_chip_info_hccs_statistic_info_tx_cnt_6",
		"npu_chip_info_hccs_statistic_info_tx_cnt_7",
	},
	"npu_chip_info_hccs_statistic_info_rx_cnt": { //HCCS链路接收报文数
		"npu_chip_info_hccs_statistic_info_rx_cnt_1",
		"npu_chip_info_hccs_statistic_info_rx_cnt_2",
		"npu_chip_info_hccs_statistic_info_rx_cnt_3",
		"npu_chip_info_hccs_statistic_info_rx_cnt_4",
		"npu_chip_info_hccs_statistic_info_rx_cnt_5",
		"npu_chip_info_hccs_statistic_info_rx_cnt_6",
		"npu_chip_info_hccs_statistic_info_rx_cnt_7",
	},
	"npu_chip_info_hccs_statistic_info_crc_err_cnt_max": {
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_1",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_2",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_3",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_4",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_5",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_6",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_7",
	},
}

func aggregateMultiLane(f *types.MetricFrame) {
	for agg, members := range multiLaneGroups {
		worst, found := 0.0, false
		for _, m := range members {
			if v, ok := f.Metrics[m]; ok {
				found = true
				if v > worst {
					worst = v
				}
			}
			delete(f.Metrics, m) // 原始项不参与评分，但保留在 CK 里供排查
		}
		if found {
			f.Metrics[agg] = worst
		}
	}
}

// windowSum 把本次增量记入历史，并返回 [now-window, now] 窗口内的累计增量
func (s *CKLoaderService) windowSum(uuid, key string, now int64, delta, windowSec float64) float64 {
	if s.deltaHist[uuid] == nil {
		s.deltaHist[uuid] = map[string][]deltaSample{}
	}
	hist := append(s.deltaHist[uuid][key], deltaSample{ts: now, delta: delta})

	cutoff := now - int64(windowSec)
	var sum float64
	kept := hist[:0]
	for _, d := range hist {
		if d.ts < cutoff {
			continue // 滑出窗口，丢弃
		}
		kept = append(kept, d)
		sum += d.delta
	}
	s.deltaHist[uuid][key] = kept
	return sum
}

type meta struct{ source, sn, ip, tags string }

func (s *CKLoaderService) Collect(ctx context.Context) ([]types.MetricFrame, error) {
	window := time.Duration(s.cfg.WindowSec) * time.Second //时间窗口设置
	if window <= 0 {
		window = 5 * time.Minute
	}

	//查出「当前时间窗口内」有活跃数据上报的所有 source（即集群）列表
	sources, err := s.ck.ListSources(ctx, s.cfg.Table, window)
	if err != nil {
		return nil, fmt.Errorf("查询 source 列表失败: %w", err)
	}
	if len(sources) == 0 {
		logger.L.Warn("CK 近窗口无数据(无活跃source)")
		return nil, nil
	}

	//下面内容为初步测试小数量的卡设置
	// ★ 新增：source 过滤，这里只采集“jiushu”的卡
	if s.cfg.Source != "" {
		found := false
		for _, src := range sources {
			if src == s.cfg.Source {
				found = true
				break
			}
		}
		if !found {
			logger.L.Warnf("配置的 source=%s 在 CK 中无活跃数据", s.cfg.Source)
			return nil, nil
		}
		sources = []string{s.cfg.Source}
		logger.L.Infof("source 过滤: 只采集 %s", s.cfg.Source)
	}

	//汇总所有集群查询结果的大切片
	var allRows []ckclient.SampleRow
	for _, src := range sources { //逐个集群遍历(当前只有jiushu)
		rows, err := s.ck.LatestSamplesBySource(ctx, s.cfg.Table, src, window) //查单集群
		if err != nil {
			logger.L.Warnf("CK 查询 source=%s 失败: %v，跳过", src, err) //单集群失败只跳过
			continue
		}
		allRows = append(allRows, rows...) // 拿到所有活跃卡的最新指标
	}
	if len(allRows) == 0 {
		logger.L.Warn("CK 近窗口无数据")
		return nil, nil
	}

	// ★ 新增：gpu_limit 随机抽样
	// gpu_limit 固定抽样：抽中的卡集合一次性确定后固定不变，
	// 保证 counter delta 连续、评分对象稳定；gpu_limit=0 时整段跳过=全量。
	if s.cfg.GPULimit > 0 {
		if s.sampledUUIDs == nil { //首次运行才初始化抽样集合
			s.loadOrInitSample(allRows) // 决定"锁定哪几张卡"
		}
		filtered := make([]ckclient.SampleRow, 0, len(allRows))
		for _, r := range allRows {
			if _, ok := s.sampledUUIDs[gpuUUID(r.SN, r.Tags)]; ok {
				filtered = append(filtered, r) // 只保留抽中卡的行
			}
		}
		logger.L.Infof("gpu_limit=%d 固定抽样: %d 行 -> %d 行(锁定 %d 张卡)",
			s.cfg.GPULimit, len(allRows), len(filtered), len(s.sampledUUIDs))
		allRows = filtered
	} else if s.sampledUUIDs != nil {
		// 运行期从抽样切回全量：清空集合，避免残留影响
		s.sampledUUIDs = nil
		logger.L.Info("gpu_limit=0 全量模式，清空抽样集合")
	}

	//不抽样的代码配置：
	//var allRows []ckclient.SampleRow
	//for _, src := range sources {
	//	rows, err := s.ck.LatestSamplesBySource(ctx, s.cfg.Table, src, window)
	//	if err != nil {
	//		logger.L.Warnf("CK 查询 source=%s 失败: %v，跳过", src, err)
	//		continue
	//	}
	//	allRows = append(allRows, rows...)
	//}
	//if len(allRows) == 0 {
	//	logger.L.Warn("CK 近窗口无数据")
	//	return nil, nil
	//}

	//counterKeys 是"累计计数器"类指标的集合(用于后续delta差值计算)
	if s.counterKeys == nil {
		s.loadCounterKeys()
	}

	// 下面就是将扁平乱序的 allRows 收拢成每卡一帧
	// frames:  以 uuid(卡) 为 key,聚合出每张卡的一帧指标数据 MetricFrame
	// metas:   以 uuid 为 key,记录该卡的元信息(source/sn/ip/tags),供后续同步拓扑用
	// liveKeys:本轮出现过的所有指标名集合,用于同步"当前存活的指标健康度 key"
	frames := map[string]*types.MetricFrame{}
	metas := map[string]meta{}
	liveKeys := map[string]struct{}{}
	now := time.Now().Unix() //统一时间戳,保证同一轮所有帧 TS 一致

	skipped := 0
	// 遍历所有扁平行(每行 = 某张卡的某个指标的最新值),按卡聚合
	for _, r := range allRows { //遍历每一条扁平行数据
		//tag为卡序号，为空说明世界点指标或上游漏打标签
		// 拼成的 uuid 会把整个节点的行合并成一张"幽灵卡"，必须丢弃，
		//todo 后续调整：用卡的uuid来指代
		if strings.TrimSpace(r.Tags) == "" {
			skipped++
			continue
		}
		uuid := gpuUUID(r.SN, r.Tags) //SN:Tags 拼出GPU卡的唯一标识

		// 该卡的帧第一次出现时初始化,并记录其元信息
		f, ok := frames[uuid]
		if !ok {
			f = &types.MetricFrame{UUID: uuid, TS: now, Metrics: map[string]float64{}}
			frames[uuid] = f
			metas[uuid] = meta{r.Source, r.SN, r.IP, r.Tags} //顺便记元信息(只记一次)，也就是这张卡的基本信息
		}
		mib := normalizeMIB(r.MIB) // 规范化指标名(统一命名/去噪)
		val := r.Value
		if factor, ok := unitConvert[mib]; ok {
			val *= factor //单位换算(如违规时长 μs→s，×1e-6)
		}
		f.Metrics[mib] = val       // 把指标写入该卡这一帧
		liveKeys[mib] = struct{}{} // 记录该指标本轮存活
	}
	if skipped > 0 {
		logger.L.Warnf("本轮丢弃 %d 行 tag 为空的数据（无法定位到具体卡）", skipped)
	}
	// 先把多通道指标聚合成单条，再做增量/速率换算
	for _, f := range frames {
		aggregateMultiLane(f)
	}
	s.applyDelta(frames)

	// 统计每个节点(sn)上聚合到的 GPU 数量,供拓扑同步时更新节点的卡数
	nodeGPUCount := map[string]int{}
	for _, m := range metas {
		nodeGPUCount[m.sn]++
	}

	// 批量同步拓扑：确保 cluster/node/gpu 记录存在并更新(替代逐卡 upsert,减少 DB 往返)
	s.syncTopologyBatch(metas, frames, nodeGPUCount)

	// 同步"存活指标"到指标健康度 key 表(标记哪些指标本轮有数据)
	s.syncMetricHealthKey(liveKeys)

	// 把 map 里的每张卡帧摊平成切片返回给调用方(下一步做评分)
	list := make([]types.MetricFrame, 0, len(frames))
	for _, f := range frames {
		list = append(list, *f)
	}
	logger.L.Infof("CK 采集完成：%d 个source, %d 张卡, %d 行原始数据", len(sources), len(list), len(allRows))
	return list, nil
}

// loadOrInitSample 初始化固定抽样集合：
//  1. 优先从 DB 读取该 source 下 status=online 的卡(之前已锁定的抽样集合)；
//  2. DB 无 online 卡(首次部署)时，从本轮 allRows 随机选 GPULimit 张作为集合，
//     并立即落库(选中=online/未选=offline)，之后固定。
func (s *CKLoaderService) loadOrInitSample(allRows []ckclient.SampleRow) {
	s.sampledUUIDs = map[string]struct{}{}

	// 1) 先尝试从 DB 读已锁定的 online 抽样集合(限定当前 source 集群)
	db := s.topo.DB().Model(&model.GPUCard{}).Where("status = ?", "online")
	if s.cfg.Source != "" {
		db = db.Where("cluster_id IN (?)",
			s.topo.DB().Model(&model.Cluster{}).Select("id").Where("code = ?", s.cfg.Source))
	}
	var uuids []string
	if err := db.Pluck("uuid", &uuids).Error; err != nil {
		logger.L.Warnf("读取已锁定抽样集合失败: %v", err)
	}
	if len(uuids) > 0 {
		for _, u := range uuids {
			s.sampledUUIDs[u] = struct{}{}
		}
		logger.L.Infof("固定抽样集合从DB加载: %d 张卡", len(s.sampledUUIDs))
		return
	}

	// 2) DB 无 online 卡 → 首轮随机初始化
	uuidSet := make(map[string]struct{}, len(allRows))
	for _, r := range allRows {
		uuidSet[gpuUUID(r.SN, r.Tags)] = struct{}{}
	}
	all := make([]string, 0, len(uuidSet))
	for u := range uuidSet {
		all = append(all, u)
	}
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	limit := s.cfg.GPULimit
	if limit > len(all) {
		limit = len(all)
	}
	for _, u := range all[:limit] {
		s.sampledUUIDs[u] = struct{}{}
	}
	// 立即把本轮全量卡落库：抽中=online，未抽中=offline，固定抽样集合
	s.persistSampleStatus(allRows)

	logger.L.Infof("固定抽样集合首轮随机初始化: 从 %d 张卡选 %d 张", len(all), len(s.sampledUUIDs))
}

// syncTopologyBatch 批量同步拓扑，替代原来的逐卡 ensureCluster/ensureNode/UpsertGPU
func (s *CKLoaderService) syncTopologyBatch(metas map[string]meta, frames map[string]*types.MetricFrame, nodeGPUCount map[string]int) {
	// 1. 批量确保 Cluster（集群数极少）
	sourceSet := map[string]struct{}{}
	for _, m := range metas {
		sourceSet[m.source] = struct{}{}
	}
	for src := range sourceSet {
		if _, err := s.ensureCluster(src); err != nil {
			logger.L.Warnf("cluster %s: %v", src, err)
		}
	}

	// 2. 批量确保 Node
	// 2a. 收集所有需要写入的节点（不在缓存中 或 gpuCount 变化的）
	type nodeInfo struct {
		clusterID uint64
		sn        string
		ip        string
		gpuCount  int
	}
	needUpsertNodes := map[string]nodeInfo{}
	for _, m := range metas {
		cid, ok := s.clusterCache[m.source]
		if !ok {
			continue
		}
		gpuCnt := nodeGPUCount[m.sn]
		if entry, cached := s.nodeCache[m.sn]; cached {
			if entry.gpuCount == gpuCnt {
				continue // 缓存命中且 gpuCount 没变，跳过
			}
		}
		needUpsertNodes[m.sn] = nodeInfo{clusterID: cid, sn: m.sn, ip: m.ip, gpuCount: gpuCnt}
	}

	// 2b. 批量 upsert 节点
	if len(needUpsertNodes) > 0 {
		batch := make([]model.Node, 0, len(needUpsertNodes))
		for _, n := range needUpsertNodes {
			batch = append(batch, model.Node{
				ClusterID: n.clusterID, Hostname: n.sn, IP: n.ip, GPUCount: n.gpuCount,
			})
		}
		if err := s.topo.DB().Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "hostname"}},
			DoUpdates: clause.AssignmentColumns([]string{"gpu_count", "ip", "updated_at"}),
		}).CreateInBatches(batch, 500).Error; err != nil {
			logger.L.Errorf("批量 upsert node 失败: %v", err)
		}

		// 2c. 刷新缓存：查出所有刚写入的节点 ID
		hostnames := make([]string, 0, len(needUpsertNodes))
		for sn := range needUpsertNodes {
			hostnames = append(hostnames, sn)
		}
		var nodes []model.Node
		s.topo.DB().Where("hostname IN ?", hostnames).Find(&nodes)
		for _, n := range nodes {
			s.nodeCache[n.Hostname] = nodeEntry{id: n.ID, gpuCount: n.GPUCount}
		}
	}

	// 3. 批量 upsert GPU 卡
	gpus := make([]model.GPUCard, 0, len(metas))
	for uuid, m := range metas {
		cid, ok := s.clusterCache[m.source]
		if !ok {
			continue
		}
		entry, ok := s.nodeCache[m.sn]
		if !ok {
			continue
		}
		idx, err := strconv.Atoi(m.tags)

		if err != nil {
			logger.L.Warnf("卡 %s 的 tags=%q 不是合法卡序号，跳过入库", uuid, m.tags)
			continue
		}

		vendor, cardType := detectDevice(frames[uuid].Metrics)
		if cardType == "" {
			logger.L.Warnf("卡 %s 无法识别设备类型（指标数=%d），本轮跳过评分",
				uuid, len(frames[uuid].Metrics))
			continue // ★ 判不出来就别入库，更别去评分
		}
		gpus = append(gpus, model.GPUCard{
			UUID: uuid, NodeID: entry.id, ClusterID: cid,
			GPUIndex: idx, SN: m.sn, Status: "online", Vendor: vendor, CardType: cardType,
		})
	}
	if len(gpus) > 0 {
		if err := s.topo.DB().Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "uuid"}},
			DoUpdates: clause.AssignmentColumns([]string{"node_id", "cluster_id", "gpu_index", "status", "vendor", "card_type", "updated_at"}),
		}).CreateInBatches(gpus, 500).Error; err != nil {
			logger.L.Errorf("批量 upsert gpu 失败: %v", err)
		}
	}
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

	if n, err := s.metricRepo.UpdateAliveByMetricKeys(enableKeys, true); err != nil {
		logger.L.Warnf("启用指标评分失败: %v", err)
	} else if n > 0 {
		logger.L.Infof("启用 %d 个指标参与评分: %v", n, enableKeys)
		//s.bumpStrategiesByKeys(enableKeys)
	}

	if n, err := s.metricRepo.UpdateAliveByMetricKeys(disableKeys, false); err != nil {
		logger.L.Warnf("关闭指标评分失败: %v", err)
	} else if n > 0 {
		logger.L.Infof("关闭 %d 个指标参与评分(连续 %d 轮未出现): %v",
			n, missThreshold, disableKeys)
		//s.bumpStrategiesByKeys(disableKeys)
		for _, k := range disableKeys {
			s.missCount[k] = 0
		}
	}
}

//func (s *CKLoaderService) bumpStrategiesByKeys(keys []string) {
//	for _, k := range keys {
//		if err := s.strategyRepo.BumpVersionByMetricKey(k); err != nil {
//			logger.L.Warnf("BumpVersion[%s]失败: %v", k, err)
//		}
//	}
//}

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

// persistSampleStatus 首轮初始化时，把本轮全量卡按是否抽中落库 status。
// 需先确保 cluster/node 存在，故复用 syncTopologyBatch 的前置逻辑较重；
// 这里简化为直接按 uuid upsert status(卡记录已由 syncTopologyBatch 建立，
// 首轮可能尚未建立，故用 online/offline 两批 UPDATE 兜底)。
func (s *CKLoaderService) persistSampleStatus(allRows []ckclient.SampleRow) {
	online := make([]string, 0, len(s.sampledUUIDs))
	offline := make([]string, 0)
	seen := map[string]struct{}{}
	for _, r := range allRows {
		u := gpuUUID(r.SN, r.Tags)
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		if _, hit := s.sampledUUIDs[u]; hit {
			online = append(online, u)
		} else {
			offline = append(offline, u)
		}
	}
	if len(offline) > 0 {
		if err := s.topo.DB().Model(&model.GPUCard{}).
			Where("uuid IN ?", offline).Update("status", "offline").Error; err != nil {
			logger.L.Warnf("标记未抽中卡 offline 失败: %v", err)
		}
	}
	if len(online) > 0 {
		if err := s.topo.DB().Model(&model.GPUCard{}).
			Where("uuid IN ?", online).Update("status", "online").Error; err != nil {
			logger.L.Warnf("标记抽中卡 online 失败: %v", err)
		}
	}
}
