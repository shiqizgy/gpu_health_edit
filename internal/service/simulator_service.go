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

// ── DCGM 22 个指标（全量）──
var dcgmMetricKeys = []string{
	"DCGM_FI_DEV_GPU_TEMP",
	"DCGM_FI_DEV_MEMORY_TEMP",
	"DCGM_FI_DEV_POWER_USAGE",
	"DCGM_FI_DEV_THERMAL_VIOLATION",
	"DCGM_FI_DEV_POWER_VIOLATION",
	"DCGM_FI_DEV_LOW_UTIL_VIOLATION",
	"DCGM_FI_DEV_BOARD_LIMIT_VIOLATION",
	"DCGM_FI_DEV_SYNC_BOOST_VIOLATION",
	"DCGM_FI_DEV_RELIABILITY_VIOLATION",
	"DCGM_FI_PROF_GR_ENGINE_ACTIVE",
	"DCGM_FI_PROF_SM_ACTIVE",
	"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE",
	"DCGM_FI_PROF_DRAM_ACTIVE",
	"DCGM_FI_DEV_SM_CLOCK",
	"DCGM_FI_DEV_FB_USED_PERCENT",
	"DCGM_FI_DEV_ECC_SBE_VOL_TOTAL",
	"DCGM_FI_DEV_ECC_DBE_VOL_TOTAL",
	"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS",
	"DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS",
	"DCGM_FI_DEV_ROW_REMAP_FAILURE",
	"DCGM_FI_DEV_PCIE_REPLAY_COUNTER",
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL",
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL",
	"DCGM_FI_DEV_FABRIC_HEALTH_MASK",
	"DCGM_FI_DEV_XID_ERRORS",
	"DCGM_FI_DEV_CLOCKS_EVENT_REASONS",
	"DCGM_FI_DEV_GPU_RESET_COUNT",
}

// ── NPU 74 个指标（全量）──
var npuMetricKeys = []string{
	// npu_pcie (4)
	"NPU_PCIE_REPLAY_COUNTER", "NPU_PCIE_CRC_ERROR", "NPU_PCIE_TXRX_ERROR", "NPU_PCIE_LINK_WIDTH_GEN",
	// npu_memory (14)
	"NPU_ECC_ENABLE", "NPU_HBM_SBE_RATE", "NPU_HBM_DBE", "NPU_DDR_ECC_ERROR",
	"NPU_ISOLATED_PAGES", "NPU_ISOLATION_EXHAUSTED", "NPU_ISOLATION_PENDING",
	"NPU_HBM_TOTAL", "NPU_HBM_USAGE_RATE", "NPU_HBM_IDLE_RESIDUAL",
	"NPU_DDR_USAGE_RATE", "NPU_DDR_HUGEPAGES_RATE", "NPU_PROC_MEM_USAGE",
	// npu_thermal (6)
	"NPU_CHIP_TEMP", "NPU_HBM_TEMP", "NPU_DUAL_CHIP_TEMP_DIFF",
	"NPU_BOARD_SENSOR_TEMP", "NPU_TEMP_VIOALTION_RATIO", "NPU_FAN_STATUS",
	// npu_power (6)
	"NPU_POWER_USAGE", "NPU_POWER_Ratio_R", "NPU_CHIP_VOLTAGE",
	"NPU_LOW_POWER_DERATE", "NPU_MCU_STATUS", "NPU_POWER_FAULT_CODE",
	// npu_interconnect (14)
	"NPU_HCCS_LINK_STATE", "NPU_HCCS_LINK_SPEED", "NPU_HCCS_BW_ACHIEVE",
	"NPU_ROCE_LINK_STATE", "NPU_ROCE_NET_HEALTH", "NPU_ROCE_LINK_SPEED",
	"NPU_ROCE_PACKET_LOSS", "NPU_PFC_ANOMALY", "NPU_ROCE_PORT_RATE",
	"NPU_OPTICAL_TEMP", "NPU_OPTICAL_POWER", "NPU_OPTICAL_PRESENT",
	"NPU_P2P_ENABLE", "NPU_TOPOLOGY",
	// npu_reliability (18)
	"NPU_HEALTH_STATUS", "NPU_HEALTH_STATUS_BINARY", "NPU_ERROR_CODE", "NPU_ERROR_COUNT",
	"NPU_VISIBILITY", "NPU_DEVICE_OS_HEARTBEAT", "NPU_RESET_COUNT", "NPU_UPTIME_SINCE_FAULT",
	"NPU_VERSION_CONSISTENCY", "NPU_DRIVER_STATUS", "NPU_FLASH_STATUS",
	"NPU_I2C_CHECK", "NPU_WORKMODE", "NPU_VNPVMODE", "NPU_LICENSE_STATUS",
	"NPU_COLLECT_HEALTH", "NPU_FIRST_POWER_DATE",
	// npu_auxiliary (10)
	"NPU_AICORE_UTIL", "NPU_AIVECTOR_UTIL", "NPU_AICPU_USAGE", "NPU_MEMORY_USAGE",
	"NPU_DDR_BW_USAGE", "NPU_HBM_BW_USAGE", "NPU_HUGEPAGES_USAGE",
	"NPU_DEVICE_SHARE", "NPU_AICPU_CORE_CONFIG", "NPU_PROCESS_INFO",
	// npu_compute (8)
	"NPU_AICORE_FREQ", "NPU_AICORE_CURFREQ", "NPU_AICORE_FREQ_ACHIEVE",
	"NPU_AICORE_COUNT", "NPU_FLOPs_ACHIEVE", "NPU_HBM_BW_ACHIEVE",
	"NPU_CTRL_CPU_USAGE", "NPU_OUTLIER_ZSCORE",
}

type gpuState struct {
	source, sn, ip, group string
	index                 int
	counters              map[string]float64
	faultMode             string
	faultTicks            int
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
	estimatedRows := len(s.fleet) * (len(dcgmMetricKeys) + len(npuMetricKeys))
	rows := make([]ckclient.MetricRow, 0, estimatedRows)

	for _, st := range s.fleet {
		s.stepFault(st)

		// 判断该卡是 DCGM 还是 NPU：基于 source 名称约定
		// source 含 "npu" → NPU 卡；否则 → DCGM 卡
		isNPU := containsNPU(st.source)

		if isNPU {
			for _, mib := range npuMetricKeys {
				rows = append(rows, ckclient.MetricRow{
					Timestamp: now, IP: st.ip, SN: st.sn, Source: st.source,
					MIB: mib, Tags: strconv.Itoa(st.index),
					Value: s.genNPUValue(st, mib), DT: now, NodeGroup: st.group,
				})
			}
		} else {
			for _, mib := range dcgmMetricKeys {
				rows = append(rows, ckclient.MetricRow{
					Timestamp: now, IP: st.ip, SN: st.sn, Source: st.source,
					MIB: mib, Tags: strconv.Itoa(st.index),
					Value: s.genDCGMValue(st, mib), DT: now, NodeGroup: st.group,
				})
			}
		}
	}
	return s.ck.InsertSamples(ctx, s.table, rows)
}

func containsNPU(source string) bool {
	return len(source) >= 3 && (source[0:3] == "npu" || containsCI(source, "npu"))
}

func containsCI(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ════════════════════════════════════════════════════════════
// DCGM 仿真值生成
// ════════════════════════════════════════════════════════════
func (s *SimulatorService) genDCGMValue(st *gpuState, mib string) float64 {
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

	// —— violation 指标（1e-6 转换单位，仿真原始值）——
	case "DCGM_FI_DEV_THERMAL_VIOLATION":
		return s.bump(st, mib, s.faultBump(st, mib))
	case "DCGM_FI_DEV_POWER_VIOLATION":
		return s.bump(st, mib, prob(0.01, 1))
	case "DCGM_FI_DEV_LOW_UTIL_VIOLATION":
		return s.bump(st, mib, prob(0.03, 1))
	case "DCGM_FI_DEV_BOARD_LIMIT_VIOLATION":
		return s.bump(st, mib, prob(0.01, 1))
	case "DCGM_FI_DEV_SYNC_BOOST_VIOLATION":
		return s.bump(st, mib, prob(0.005, 1))
	case "DCGM_FI_DEV_RELIABILITY_VIOLATION":
		return s.bump(st, mib, prob(0.005, 1))

	// —— 累计计数器：单调递增 ——
	case "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL":
		return s.bump(st, mib, prob(0.05, 1))
	case "DCGM_FI_DEV_PCIE_REPLAY_COUNTER":
		return s.bump(st, mib, prob(0.1, 1))
	case "DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL":
		return s.bump(st, mib, prob(0.08, 2))
	case "DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL",
		"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS":
		return s.bump(st, mib, prob(0.02, 1))
	case "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL", "DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS",
		"DCGM_FI_DEV_GPU_RESET_COUNT":
		return s.bump(st, mib, s.faultBump(st, mib))

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
			return 4
		}
		return 0
	case "DCGM_FI_DEV_CLOCKS_EVENT_REASONS":
		if st.faultMode == "thermal" {
			return 8
		}
		return 0
	}
	return 0
}

// ════════════════════════════════════════════════════════════
// NPU 仿真值生成
// ════════════════════════════════════════════════════════════
func (s *SimulatorService) genNPUValue(st *gpuState, mib string) float64 {
	switch mib {
	// ── npu_pcie ──
	case "NPU_PCIE_REPLAY_COUNTER":
		return s.bump(st, mib, prob(0.08, 1))
	case "NPU_PCIE_CRC_ERROR":
		return s.bump(st, mib, prob(0.05, 1))
	case "NPU_PCIE_TXRX_ERROR":
		return s.bump(st, mib, prob(0.03, 1))
	case "NPU_PCIE_LINK_WIDTH_GEN":
		return 4 // PCIe Gen4 x16 = 4

	// ── npu_memory ──
	case "NPU_ECC_ENABLE":
		return 1
	case "NPU_HBM_SBE_RATE":
		return gauge(0.001, 0.002, 0, 0.1)
	case "NPU_HBM_DBE":
		return s.bump(st, mib, s.npuFaultBump(st, "hbm_dbe"))
	case "NPU_DDR_ECC_ERROR":
		return s.bump(st, mib, prob(0.02, 1))
	case "NPU_ISOLATED_PAGES":
		return gauge(5, 3, 0, 500)
	case "NPU_ISOLATION_EXHAUSTED":
		if st.faultMode == "iso_exhaust" {
			return 1
		}
		return 0
	case "NPU_ISOLATION_PENDING":
		return gauge(0, 1, 0, 50)
	case "NPU_HBM_TOTAL":
		return 32768 // 32GB HBM
	case "NPU_HBM_USAGE_RATE":
		return gauge(0.65, 0.2, 0, 1)
	case "NPU_HBM_IDLE_RESIDUAL":
		return gauge(8000, 2000, 0, 32768)
	case "NPU_DDR_USAGE_RATE":
		return gauge(0.4, 0.15, 0, 1)
	case "NPU_DDR_HUGEPAGES_RATE":
		return gauge(0.7, 0.15, 0, 1)
	case "NPU_PROC_MEM_USAGE":
		return gauge(2000, 800, 0, 16384)

	// ── npu_thermal ──
	case "NPU_CHIP_TEMP":
		return gauge(55, 10, 30, 95) + s.npuThermalBias(st)
	case "NPU_HBM_TEMP":
		return gauge(60, 10, 30, 100) + s.npuThermalBias(st)
	case "NPU_DUAL_CHIP_TEMP_DIFF":
		return gauge(2, 2, 0, 15)
	case "NPU_BOARD_SENSOR_TEMP":
		return gauge(45, 8, 20, 85)
	case "NPU_TEMP_VIOALTION_RATIO":
		if st.faultMode == "npu_thermal" {
			return gauge(0.3, 0.1, 0, 1)
		}
		return 0
	case "NPU_FAN_STATUS":
		return 1 // 正常

	// ── npu_power ──
	case "NPU_POWER_USAGE":
		return gauge(310, 80, 50, 700)
	case "NPU_POWER_Ratio_R":
		return gauge(0.75, 0.15, 0, 1)
	case "NPU_CHIP_VOLTAGE":
		return gauge(0.8, 0.02, 0.6, 1.0)
	case "NPU_LOW_POWER_DERATE":
		return 0 // 无降额
	case "NPU_MCU_STATUS":
		return 1 // 正常
	case "NPU_POWER_FAULT_CODE":
		if st.faultMode == "power_fault" {
			return 1
		}
		return 0

	// ── npu_interconnect ──
	case "NPU_HCCS_LINK_STATE":
		if st.faultMode == "hccs_down" {
			return 0
		}
		return 1
	case "NPU_HCCS_LINK_SPEED":
		if st.faultMode == "hccs_down" {
			return 0
		}
		return 392 // 392 Gbps
	case "NPU_HCCS_BW_ACHIEVE":
		if st.faultMode == "hccs_down" {
			return 0
		}
		return gauge(0.8, 0.15, 0, 1)
	case "NPU_ROCE_LINK_STATE":
		if st.faultMode == "roce_down" {
			return 0
		}
		return 1
	case "NPU_ROCE_NET_HEALTH":
		if st.faultMode == "roce_down" {
			return 0
		}
		return 1
	case "NPU_ROCE_LINK_SPEED":
		if st.faultMode == "roce_down" {
			return 0
		}
		return 200 // 200 Gbps
	case "NPU_ROCE_PACKET_LOSS":
		return s.bump(st, mib, prob(0.1, 1))
	case "NPU_PFC_ANOMALY":
		return 0
	case "NPU_ROCE_PORT_RATE":
		if st.faultMode == "roce_down" {
			return 0
		}
		return 200
	case "NPU_OPTICAL_TEMP":
		return gauge(40, 8, 10, 80)
	case "NPU_OPTICAL_POWER":
		return gauge(-3, 1.5, -10, 3)
	case "NPU_OPTICAL_PRESENT":
		return 1
	case "NPU_P2P_ENABLE":
		return 1
	case "NPU_TOPOLOGY":
		return 1 // full mesh

	// ── npu_reliability ──
	case "NPU_HEALTH_STATUS":
		if st.faultMode != "" {
			return 1 // abnormal
		}
		return 0 // normal
	case "NPU_HEALTH_STATUS_BINARY":
		if st.faultMode != "" {
			return float64(int(1) << rand.Intn(8))
		}
		return 0
	case "NPU_ERROR_CODE":
		if st.faultMode == "hbm_dbe" {
			return 1001
		}
		if st.faultMode == "npu_thermal" {
			return 2001
		}
		if st.faultMode == "hccs_down" {
			return 3001
		}
		return 0
	case "NPU_ERROR_COUNT":
		return s.bump(st, mib, s.npuFaultBump(st, "error"))
	case "NPU_VISIBILITY":
		return 1
	case "NPU_DEVICE_OS_HEARTBEAT":
		return 1
	case "NPU_RESET_COUNT":
		return s.bump(st, mib, s.npuFaultBump(st, "reset"))
	case "NPU_UPTIME_SINCE_FAULT":
		return gauge(86400, 3600, 0, 999999)
	case "NPU_VERSION_CONSISTENCY":
		return 1
	case "NPU_DRIVER_STATUS":
		if st.faultMode == "driver_err" {
			return 0
		}
		return 1
	case "NPU_FLASH_STATUS":
		return 1
	case "NPU_I2C_CHECK":
		return 1
	case "NPU_WORKMODE":
		return 1 // 训练模式
	case "NPU_VNPVMODE":
		return 0
	case "NPU_LICENSE_STATUS":
		return 1
	case "NPU_COLLECT_HEALTH":
		return 1
	case "NPU_FIRST_POWER_DATE":
		return 20230101

	// ── npu_auxiliary（is_health_key=0，不参与评分）──
	case "NPU_AICORE_UTIL":
		return gauge(0.7, 0.2, 0, 1)
	case "NPU_AIVECTOR_UTIL":
		return gauge(0.6, 0.2, 0, 1)
	case "NPU_AICPU_USAGE":
		return gauge(0.3, 0.1, 0, 1)
	case "NPU_MEMORY_USAGE":
		return gauge(0.5, 0.15, 0, 1)
	case "NPU_DDR_BW_USAGE":
		return gauge(0.4, 0.15, 0, 1)
	case "NPU_HBM_BW_USAGE":
		return gauge(0.6, 0.2, 0, 1)
	case "NPU_HUGEPAGES_USAGE":
		return gauge(0.7, 0.15, 0, 1)
	case "NPU_DEVICE_SHARE":
		return 0
	case "NPU_AICPU_CORE_CONFIG":
		return 8
	case "NPU_PROCESS_INFO":
		return 1

	// ── npu_compute ──
	case "NPU_AICORE_FREQ":
		return gauge(1800, 100, 200, 2200)
	case "NPU_AICORE_CURFREQ":
		return gauge(1750, 100, 200, 2200)
	case "NPU_AICORE_FREQ_ACHIEVE":
		return gauge(0.95, 0.03, 0, 1)
	case "NPU_AICORE_COUNT":
		return 32
	case "NPU_FLOPs_ACHIEVE":
		return gauge(0.8, 0.15, 0, 1)
	case "NPU_HBM_BW_ACHIEVE":
		return gauge(0.75, 0.15, 0, 1)
	case "NPU_CTRL_CPU_USAGE":
		return gauge(0.2, 0.1, 0, 1)
	case "NPU_OUTLIER_ZSCORE":
		return gauge(0, 1, -3, 3)
	}
	return 0
}

// ════════════════════════════════════════════════════════════
// 辅助函数
// ════════════════════════════════════════════════════════════
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

// ── DCGM 故障注入 ──
func (s *SimulatorService) stepFault(st *gpuState) {
	if st.faultMode != "" {
		st.faultTicks--
		if st.faultTicks <= 0 {
			st.faultMode = ""
		}
		return
	}
	if rand.Float64() < s.cfg.FaultRate {
		if containsNPU(st.source) {
			modes := []string{"hbm_dbe", "npu_thermal", "hccs_down", "roce_down", "power_fault", "driver_err", "iso_exhaust"}
			st.faultMode = modes[rand.Intn(len(modes))]
		} else {
			modes := []string{"thermal", "ecc_dbe", "xid_fatal", "nvlink", "remap_fail", "fabric"}
			st.faultMode = modes[rand.Intn(len(modes))]
		}
		st.faultTicks = 3 + rand.Intn(8)
	}
}

func (s *SimulatorService) thermalBias(st *gpuState) float64 {
	if st.faultMode == "thermal" {
		return 35
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

func (s *SimulatorService) xidValue(st *gpuState) float64 {
	switch st.faultMode {
	case "xid_fatal":
		return 79
	case "ecc_dbe":
		return 48
	case "nvlink":
		return 74
	}
	return 0
}

// ── NPU 故障注入 ──
func (s *SimulatorService) npuFaultBump(st *gpuState, trigger string) float64 {
	switch trigger {
	case "hbm_dbe":
		if st.faultMode == "hbm_dbe" {
			return 1
		}
	case "error":
		if st.faultMode != "" {
			return 1
		}
	case "reset":
		if st.faultMode == "driver_err" || st.faultMode == "hbm_dbe" {
			return 1
		}
	}
	return 0
}

func (s *SimulatorService) npuThermalBias(st *gpuState) float64 {
	if st.faultMode == "npu_thermal" {
		return 30
	}
	return 0
}
