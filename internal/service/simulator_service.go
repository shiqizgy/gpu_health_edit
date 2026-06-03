package service

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/redisclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/logger"
)

// SimulatorService 数据仿真服务。
// 模拟 2000 张 GPU，每分钟生成一条全量指标，正态分布，
// 正常值参考 DCGM 阈值，随机注入 10% 异常卡。
//
// 设计要点（保证本地负载小、运行快）：
//   - 仿真状态（每卡的累计计数器）常驻内存，不落库。
//   - 计数器型指标只累加（Add），保证单调递增——评分侧若用增量才不会出错。
//   - 异常卡在初始化时随机选定 10%，也支持通过 Redis 故障注入临时改变某卡状态。
//   - 写 Redis 用 Pipeline 批量，2000 卡一次往返完成。
type SimulatorService struct {
	cfg          config.SimulatorConfig
	redis        *redisclient.Client
	topo         *repository.TopologyRepo
	metricRepo   *repository.MetricRepo // 新增
	fleet        []*simGPU
	extraMetrics []model.MetricDefinition // 缓存DB里的自定义指标
}

// simGPU 单卡仿真状态
type simGPU struct {
	UUID      string
	ClusterID uint64
	NodeID    uint64
	GPUIndex  int
	Model     string
	IsAnomaly bool // 初始化时随机选中的异常卡

	// 计数器累计值（只增不减）
	eccSBE, eccDBE        float64
	pcieReplay, nvlinkCRC float64
	nvlinkRecovery        float64
	thermalViolation      float64
	correctableRemap      float64
	uncorrectableRemap    float64
	resetCount            float64
}

func NewSimulatorService(cfg config.SimulatorConfig, rc *redisclient.Client, topo *repository.TopologyRepo, metricRepo *repository.MetricRepo) *SimulatorService {
	return &SimulatorService{cfg: cfg, redis: rc, topo: topo, metricRepo: metricRepo}
}

// InitFleet 初始化仿真机群：建立集群/节点/GPU 拓扑并写入数据库，同时构建内存仿真状态。
//
// 拓扑规划：gpu_count 张卡，每节点 gpu_per_node 张，节点平均分到若干集群。
func (s *SimulatorService) InitFleet(ctx context.Context) error {
	total := s.cfg.GPUCount
	perNode := s.cfg.GPUPerNode
	if perNode <= 0 {
		perNode = 8
	}
	nodeCount := (total + perNode - 1) / perNode
	// 简单规划：每 32 个节点一个集群
	nodesPerCluster := 32
	clusterCount := (nodeCount + nodesPerCluster - 1) / nodesPerCluster

	models := []string{"H100-SXM5-80GB", "A100-SXM4-80GB", "H800-SXM5-80GB", "L20-PCIe-48GB"}

	// 建集群
	clusterIDs := make([]uint64, clusterCount)
	for i := 0; i < clusterCount; i++ {
		c := &model.Cluster{
			Code:   fmt.Sprintf("cluster-%03d", i+1),
			Name:   fmt.Sprintf("集群 %d", i+1),
			Region: fmt.Sprintf("region-%d", i%3+1),
		}
		if err := s.topo.CreateCluster(c); err != nil {
			// 已存在则查回（重复初始化容错）
			var existing model.Cluster
			if err := s.topo.DB().WithContext(ctx).Where("code=?", c.Code).First(&existing).Error; err != nil {
				return fmt.Errorf("查询集群 %s 失败: %w", c.Code, err)
			}
			c.ID = existing.ID
		}
		clusterIDs[i] = c.ID
	}

	s.fleet = make([]*simGPU, 0, total)
	gpuSeq := 0
	for n := 0; n < nodeCount && gpuSeq < total; n++ {
		clusterIdx := n / nodesPerCluster
		clusterID := clusterIDs[clusterIdx]
		node := &model.Node{
			ClusterID: clusterID,
			Hostname:  fmt.Sprintf("host-%05d", n),
			IP:        fmt.Sprintf("10.0.%d.%d", n/256, n%256),
			GPUCount:  perNode,
		}
		if err := s.topo.CreateNode(node); err != nil {
			var existing model.Node
			if err := s.topo.DB().WithContext(ctx).Where("hostname = ?", node.Hostname).First(&existing).Error; err != nil {
				return fmt.Errorf("查询节点 %s 失败: %w", node.Hostname, err)
			}
			node.ID = existing.ID
		}

		for j := 0; j < perNode && gpuSeq < total; j++ {
			uuid := fmt.Sprintf("GPU-%012d", gpuSeq)
			g := &model.GPUCard{
				UUID: uuid, NodeID: node.ID, ClusterID: clusterID,
				GPUIndex: j, Model: models[gpuSeq%len(models)], Status: "online",
			}
			if err := s.topo.UpsertGPU(g); err != nil {
				logger.L.Warnf("写 GPU 卡失败 %s: %v", uuid, err)
			}
			s.fleet = append(s.fleet, &simGPU{
				UUID: uuid, ClusterID: clusterID, NodeID: node.ID,
				GPUIndex: j, Model: g.Model,
				IsAnomaly: rand.Float64() < s.cfg.AnomalyRate,
			})
			gpuSeq++
		}
	}
	logger.L.Infof("仿真机群初始化完成：%d 集群 / %d 节点 / %d GPU（异常率 %.0f%%）",
		clusterCount, nodeCount, len(s.fleet), s.cfg.AnomalyRate*100)
	return nil
}

// GenerateOnce 生成一轮全量指标并写入 Redis。
func (s *SimulatorService) GenerateOnce(ctx context.Context) error {
	//每轮先从数据库同步在线GPU列表，捕获前端动态新增/下线的卡
	s.syncFleetFromDB()

	// ★ 刷新自定义指标列表（感知新增指标）
	s.refreshExtraMetrics()

	if len(s.fleet) == 0 {
		logger.L.Warn("仿真机群为空，请先 InitFleet")
		return nil
	}

	start := time.Now()
	ttl := time.Duration(s.cfg.MetricTTL) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	// 读取故障注入状态（演示用）
	faults, _ := s.redis.ListFaults(ctx)

	frames := make([]redisclient.MetricFrame, 0, len(s.fleet))
	now := time.Now().Unix()
	for _, g := range s.fleet {
		mode := faults[g.UUID] // 手动注入的故障优先
		m := s.sampleGPU(g, mode)
		frames = append(frames, redisclient.MetricFrame{
			UUID: g.UUID, TS: now, Metrics: m,
		})
	}

	if err := s.redis.WriteFramePipeline(ctx, frames, ttl); err != nil {
		logger.L.Errorf("写 Redis 失败: %v", err)
		return err
	}
	logger.L.Infof("仿真生成完成：%d 张卡，耗时 %s", len(frames), time.Since(start))
	return nil
}

// syncFleetFromDB 把数据库里的在线 GPU 同步进内存 fleet。
// 新增的卡补进来（给默认仿真状态），让前端扩容的卡也能被仿真生成指标。
func (s *SimulatorService) syncFleetFromDB() {
	gpus, err := s.topo.AllOnlineGPUs()
	if err != nil {
		logger.L.Warnf("同步fleet读取数据库失败: %v", err)
		return
	}
	//现有fleet建索引
	existing := make(map[string]bool, len(s.fleet))
	for _, g := range s.fleet {
		existing[g.UUID] = true
	}
	//新增的卡补进来
	for _, g := range gpus {
		if existing[g.UUID] {
			continue
		}
		s.fleet = append(s.fleet, &simGPU{
			UUID:      g.UUID,
			ClusterID: g.ClusterID,
			NodeID:    g.NodeID,
			GPUIndex:  g.GPUIndex,
			Model:     g.Model,
			IsAnomaly: false, //新增默认健康
		})
		logger.L.Infof("捕获新增 GPU: %s", g.UUID)
	}
}

func (s *SimulatorService) refreshExtraMetrics() {
	// 22个已知指标的key
	knownKeys := map[string]bool{
		"DCGM_FI_DEV_GPU_TEMP": true, "DCGM_FI_DEV_MEMORY_TEMP": true,
		"DCGM_FI_DEV_POWER_USAGE": true, "DCGM_FI_DEV_THERMAL_VIOLATION": true,
		"DCGM_FI_PROF_GR_ENGINE_ACTIVE": true, "DCGM_FI_PROF_SM_ACTIVE": true,
		"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE": true, "DCGM_FI_PROF_DRAM_ACTIVE": true,
		"DCGM_FI_DEV_SM_CLOCK": true, "DCGM_FI_DEV_FB_USED_PERCENT": true,
		"DCGM_FI_DEV_ECC_SBE_VOL_TOTAL": true, "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL": true,
		"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS": true, "DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS": true,
		"DCGM_FI_DEV_ROW_REMAP_FAILURE": true, "DCGM_FI_DEV_PCIE_REPLAY_COUNTER": true,
		"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL": true, "DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL": true,
		"DCGM_FI_DEV_FABRIC_HEALTH_MASK": true, "DCGM_FI_DEV_XID_ERRORS": true,
		"DCGM_FI_DEV_CLOCKS_EVENT_REASONS": true, "DCGM_FI_DEV_GPU_RESET_COUNT": true,
	}
	all, err := s.metricRepo.ListHealthKeys()
	if err != nil {
		logger.L.Warnf("刷新自定义指标失败: %v", err)
		return
	}
	extra := make([]model.MetricDefinition, 0)
	for _, m := range all {
		if !knownKeys[m.MetricKey] {
			extra = append(extra, m)
		}
	}
	s.extraMetrics = extra
}

// sampleGPU 生成单卡一帧指标。mode 为手动注入的故障模式（空则按初始 IsAnomaly 决定）。
func (s *SimulatorService) sampleGPU(g *simGPU, mode string) map[string]float64 {
	// 决定本帧的"故障态"：手动注入 > 初始异常标记
	faultMode := mode
	if faultMode == "" && g.IsAnomaly {
		// 初始异常卡：随机表现为高温或 ECC 之一
		if rand.Float64() < 0.5 {
			faultMode = "high_temp"
		} else {
			faultMode = "ecc"
		}
	}

	// ===== 基线（正常态，参考 DCGM 正常范围，正态分布）=====
	temp := normal(62, 6)                   // GPU 温度 ~62℃
	memTemp := normal(70, 5)                // HBM 温度
	power := normal(350, 60)                // 功耗 W
	smClock := normal(1830, 30)             // SM 时钟 MHz
	fbUsed := clamp(normal(0.5, 0.2), 0, 1) // 显存使用率
	grActive := clamp(normal(0.6, 0.15), 0, 1)
	smActive := clamp(grActive*0.95, 0, 1)
	tensorActive := clamp(normal(grActive*0.85, 0.1), 0, 1)
	dramActive := clamp(grActive*0.6, 0, 1)
	xid := 0.0
	clockEvent := 0.0
	fabricMask := 0.0

	// 正常态：计数器偶发小幅累加
	if rand.Float64() < 0.05 {
		g.pcieReplay += float64(rand.Intn(2))
	}
	if rand.Float64() < 0.03 {
		g.eccSBE += float64(rand.Intn(2))
	}

	// ===== 故障态：按模式叠加特征性偏移 =====
	switch faultMode {
	case "high_temp": // 高温 + 降频
		temp = normal(96, 2)
		memTemp = normal(100, 3)
		smClock = normal(1200, 30)
		clockEvent = 8 // HW_SLOWDOWN
		g.thermalViolation += float64(rand.Intn(50))
	case "xid": // XID 致命错误
		choices := []float64{48, 64, 74, 79, 119}
		xid = choices[rand.Intn(len(choices))]
	case "ecc": // ECC 错误累加
		g.eccSBE += float64(2 + rand.Intn(5))
		if rand.Float64() < 0.3 {
			g.eccDBE += 1 // 双比特错误，一票否决
		}
	case "link_down": // 互连异常
		g.nvlinkCRC += float64(5 + rand.Intn(20))
		g.nvlinkRecovery += float64(rand.Intn(3))
		fabricMask = 1
		grActive = 0
		tensorActive = 0
	case "remap_fail": // 行重映射失败，一票否决
		g.uncorrectableRemap += 1
	}

	m := map[string]float64{
		// environment
		"DCGM_FI_DEV_GPU_TEMP":          round1(temp),
		"DCGM_FI_DEV_MEMORY_TEMP":       round1(memTemp),
		"DCGM_FI_DEV_POWER_USAGE":       round1(power),
		"DCGM_FI_DEV_THERMAL_VIOLATION": g.thermalViolation,
		// performance
		"DCGM_FI_PROF_GR_ENGINE_ACTIVE":   round3(grActive),
		"DCGM_FI_PROF_SM_ACTIVE":          round3(smActive),
		"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE": round3(tensorActive),
		"DCGM_FI_PROF_DRAM_ACTIVE":        round3(dramActive),
		"DCGM_FI_DEV_SM_CLOCK":            round1(smClock),
		"DCGM_FI_DEV_FB_USED_PERCENT":     round3(fbUsed),
		// hardware
		"DCGM_FI_DEV_ECC_SBE_VOL_TOTAL":                 g.eccSBE,
		"DCGM_FI_DEV_ECC_DBE_VOL_TOTAL":                 g.eccDBE,
		"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS":         g.correctableRemap,
		"DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS":       g.uncorrectableRemap,
		"DCGM_FI_DEV_ROW_REMAP_FAILURE":                 boolToF(g.uncorrectableRemap > 0),
		"DCGM_FI_DEV_PCIE_REPLAY_COUNTER":               g.pcieReplay,
		"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL": g.nvlinkCRC,
		"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL": g.nvlinkRecovery,
		"DCGM_FI_DEV_FABRIC_HEALTH_MASK":                fabricMask,
		// stability
		"DCGM_FI_DEV_XID_ERRORS":           xid,
		"DCGM_FI_DEV_CLOCKS_EVENT_REASONS": clockEvent,
		"DCGM_FI_DEV_GPU_RESET_COUNT":      g.resetCount,
	}
	// 为数据库里额外定义的指标生成默认仿真值
	for _, em := range s.extraMetrics {
		if _, exists := m[em.MetricKey]; !exists {
			// counter类型只增不减，gauge类型每次随机
			if em.MetricType == "counter" {
				m[em.MetricKey] = math.Round(rand.Float64() * 100)
			} else {
				m[em.MetricKey] = round3(clamp(normal(0.5, 0.2), 0, 1))
			}
		}
	}

	return m
}

// ---- 工具函数 ----
func normal(mean, std float64) float64 { return mean + rand.NormFloat64()*std }
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func boolToF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
