package service

import (
	"context"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
)

var metricKeys = []string{
	"DCGM_FI_DEV_GPU_TEMP",                          //GPU 核心温度
	"DCGM_FI_DEV_MEMORY_TEMP",                       //GPU 显存温度
	"DCGM_FI_DEV_POWER_USAGE",                       //GPU 实时功耗
	"DCGM_FI_DEV_THERMAL_VIOLATION",                 //热违规/温度过高时间
	"DCGM_FI_PROF_GR_ENGINE_ACTIVE",                 //图形/计算引擎活跃时间占比
	"DCGM_FI_PROF_SM_ACTIVE",                        //SM（流式多处理器）上活跃线程束的时间占比
	"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE",               //Tensor Core（张量核心）流水线活跃时间占比
	"DCGM_FI_PROF_DRAM_ACTIVE",                      //DRAM（显存）带宽利用率
	"DCGM_FI_DEV_SM_CLOCK",                          //SM（流式多处理器）时钟频率
	"DCGM_FI_DEV_ECC_SBE_VOL_TOTAL",                 //单比特易失性错误总数
	"DCGM_FI_DEV_ECC_DBE_VOL_TOTAL",                 //双比特易失性错误总数
	"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS",         //可纠正的已重映射行数
	"DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS",       //不可纠正的已重映射行数
	"DCGM_FI_DEV_ROW_REMAP_FAILURE",                 //行重映射失败次数
	"DCGM_FI_DEV_PCIE_REPLAY_COUNTER",               //PCIe链路重试总数
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL", //流控制单元（FLIT）的CRC校验错误总数
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL", //链路错误恢复成功次数
	"DCGM_FI_DEV_XID_ERRORS",                        //最后发生的XID错误号

	//下面是原始22个指标中CK数据中没有的指标
	//DCGM_FI_DEV_FB_USED_PERCENT、
	//DCGM_FI_DEV_FABRIC_HEALTH_MASK、
	//DCGM_FI_DEV_CLOCKS_EVENT_REASONS、
	//DCGM_FI_DEV_GPU_RESET_COUNT
}

type gpuState struct {
	source, sn, ip, group string
	index                 int
	counters              map[string]float64 //累计计数器当前值
	faultMode             string             //""表示健康；否则为注入的故障模式
	faultTicks            int                //故障剩余持续轮数
}

type SimulatorService struct {
	cfg   config.SimulatorConfig
	table string
	ck    *ckclient.Client
	fleet []*gpuState
}

func NewSimulatorService(cfg config.SimulatorConfig, table string, ck *ckclient.Client) *SimulatorService {
	s := &SimulatorService{cfg: cfg, table: table, ck: ck}
	s.buildFleet()
	return s
}

// buildFleet按照配置生成source× node × gpu 的机群(一次性，内存态)
func (s *SimulatorService) buildFleet() {
	for _, src := range s.cfg.Clusters {
		for n := 0; n < s.cfg.NodesPerCluster; n++ {
			sn := src + "-node-" + strconv.Itoa(n)
			ip := "10.0" + strconv.Itoa(rand.Intn(255)) + "." + strconv.Itoa(n+1)
			for g := 0; g < s.cfg.GPUsPerNode; g++ {
				s.fleet = append(s.fleet, &gpuState{
					source: src, sn: sn, ip: ip, group: s.cfg.NodeGroup,
					index: g, counters: map[string]float64{},
				})
			}
		}
	}
}

func (s *SimulatorService) GenerateAndInsert(ctx context.Context) error {
	now := time.Now()
	rows := make([]ckclient.MetricRow, 0, len(s.fleet)*len(metricKeys))
	for _, st := range s.fleet {
		s.stepFault(st) // 概率起新故障 / 递减持续轮数
		for _, mib := range metricKeys {
			rows = append(rows, ckclient.MetricRow{
				Timestamp: now, IP: st.ip, SN: st.sn, Source: st.source,
				MIB: mib, Tags: strconv.Itoa(st.index),
				Value: s.genValue(st, mib), DT: now, NodeGroup: st.group,
			})
		}
	}
	return s.ck.InsertSamples(ctx, s.table, rows)
}

// genValue按照指标语意生成数值；累计计数器会更新st.counters
func (s *SimulatorService) genValue(st *gpuState, mib string) float64 {
	switch mib {
	// —— gauge：基线 + 噪声 ——
	case "DCGM_FI_DEV_GPU_TEMP":
		return gauge(45, 8, 25, 90) + s.thermalBias(st)
	case "DCGM_FI_DEV_MEMORY_TEMP":
		return gauge(50, 8, 25, 95) + s.thermalBias(st)
	case "DCGM_FI_DEV_POWER_USAGE":
		return gauge(250, 60, 80, 700)
	case "DCGM_FI_DEV_SM_CLOCK":
		return gauge(1400, 100, 200, 1980)
	case "DCGM_FI_DEV_FB_USED_PERCENT":
		return gauge(60, 20, 0, 100)
	case "DCGM_FI_PROF_GR_ENGINE_ACTIVE", "DCGM_FI_PROF_SM_ACTIVE",
		"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE", "DCGM_FI_PROF_DRAM_ACTIVE":
		return gauge(0.7, 0.2, 0, 1)

	// —— 累计计数器：单调递增 ——
	case "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL":
		return s.bump(st, mib, prob(0.05, 1)) // 偶发可纠正错误
	case "DCGM_FI_DEV_PCIE_REPLAY_COUNTER":
		return s.bump(st, mib, prob(0.1, 1))
	case "DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL":
		return s.bump(st, mib, prob(0.08, 2))
	case "DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL",
		"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS":
		return s.bump(st, mib, prob(0.02, 1))
	case "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL", "DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS",
		"DCGM_FI_DEV_THERMAL_VIOLATION", "DCGM_FI_DEV_GPU_RESET_COUNT":
		return s.bump(st, mib, s.faultBump(st, mib)) // 正常不增，故障时才增

	// —— XID 特例：value = 错误码 ——
	case "DCGM_FI_DEV_XID_ERRORS":
		return s.xidValue(st)

	// —— 标志 / 位掩码 ——
	case "DCGM_FI_DEV_ROW_REMAP_FAILURE":
		if st.faultMode == "remap_fail" {
			return 1
		}
		return 0
	case "DCGM_FI_DEV_FABRIC_HEALTH_MASK":
		if st.faultMode == "fabric" {
			return 4 // 置某一位
		}
		return 0
	case "DCGM_FI_DEV_CLOCKS_EVENT_REASONS":
		if st.faultMode == "thermal" {
			return 8 // HW thermal slowdown 位
		}
		return 0
	}
	return 0
}

// 辅助函数
func gauge(base, jitter, lo, hi float64) float64 {
	v := base + (rand.Float64()*2-1)*jitter
	return math.Max(lo, math.Min(hi, v))
}

func prob(p, inc float64) float64 {
	if rand.Float64() < p {
		return inc
	}
	return 0
}

func (s *SimulatorService) bump(st *gpuState, mib string, inc float64) float64 {
	st.counters[mib] += inc
	return st.counters[mib]
}

// stepFault / faultBump / xidValue / thermalBias 是故障注入逻辑
// stepFault：每轮以fault_rate概率给健康卡起一个故障，持续若干轮后自愈
func (s *SimulatorService) stepFault(st *gpuState) {
	if st.faultMode != "" {
		st.faultTicks--
		if st.faultTicks <= 0 {
			st.faultMode = ""
		}
		return
	}
	if rand.Float64() < s.cfg.FaultRate {
		modes := []string{"thermal", "ecc_dbe", "xid_fatal", "nvlink", "remap_fail", "fabric"}
		st.faultMode = modes[rand.Intn(len(modes))]
		st.faultTicks = 3 + rand.Intn(8) //持续3-10轮
	}
}

func (s *SimulatorService) thermalBias(st *gpuState) float64 {
	if st.faultMode == "thermal" {
		return 35 // 过温 +35℃
	}
	return 0
}

func (s *SimulatorService) faultBump(st *gpuState, mib string) float64 {
	switch {
	case st.faultMode == "ecc_dbe" && mib == "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL":
		return 1
	case st.faultMode == "thermal" && mib == "DCGM_FI_DEV_THERMAL_VIOLATION":
		return 1
	}
	return 0
}

// xidValue：健康=0；故障时输出对应错误码(持续 faultTicks 轮，模拟持续故障)
func (s *SimulatorService) xidValue(st *gpuState) float64 {
	switch st.faultMode {
	case "xid_fatal":
		return 79 // 掉总线
	case "ecc_dbe":
		return 48 // DBE
	case "nvlink":
		return 74
	}
	return 0
}
