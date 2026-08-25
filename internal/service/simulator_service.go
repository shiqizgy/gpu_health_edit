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

// ── DCGM 指标（58 个）──
// 参评 42 个（is_health_key=1）+ 归因/容量 16 个（仅供详情页展示，不参与评分）
// 本地保持与 accel_metric_scoring.metric_name 严格一致
var dcgmMetricKeys = []string{
	// —— memory显存可靠性（参评 16）——
	"DCGM_FI_DEV_FB_RESERVED",
	"DCGM_FI_DEV_ECC_SBE_VOL_TOTAL",
	"DCGM_FI_DEV_ECC_DBE_VOL_TOTAL",
	"DCGM_FI_DEV_ECC_SBE_AGG_TOTAL",
	"DCGM_FI_DEV_ECC_DBE_AGG_TOTAL",
	"DCGM_FI_DEV_ECC_DBE_VOL_DEV",
	"DCGM_FI_DEV_ECC_DBE_AGG_DEV",
	"DCGM_FI_DEV_ECC_DBE_AGG_SRM",
	"DCGM_FI_DEV_ECC_DBE_VOL_SRM",
	"DCGM_FI_DEV_RETIRED_SBE",
	"DCGM_FI_DEV_RETIRED_DBE",
	"DCGM_FI_DEV_RETIRED_PENDING",
	"DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS",
	"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS",
	"DCGM_FI_DEV_ROW_REMAP_FAILURE",
	"DCGM_FI_DEV_ROW_REMAP_PENDING",

	// —— nvlink片间互连（参评 10）——
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL",
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL",
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL",
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL",
	"DCGM_FI_DEV_GPU_NVLINK_ERRORS",
	"DCGM_FI_DEV_NVSWITCH_LINK_STATUS",
	"DCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS",
	"DCGM_FI_DEV_NVSWITCH_LINK_NON_FATAL_ERRORS",
	"DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS",
	"DCGM_FI_DEV_NVSWITCH_RESET_REQUIRED",

	// —— thermal温度散热（参评 4）——
	"DCGM_FI_DEV_GPU_TEMP",
	"DCGM_FI_DEV_MEMORY_TEMP",
	"DCGM_FI_DEV_THERMAL_VIOLATION",
	"DCGM_FI_DEV_NVSWITCH_TEMPERATURE_CURRENT",

	// —— power功耗电源（参评 3）——
	"DCGM_FI_DEV_POWER_VIOLATION",
	"DCGM_FI_DEV_BOARD_LIMIT_VIOLATION",
	"DCGM_FI_DEV_CLOCK_THROTTLE_REASONS",

	// —— pcie总线（参评 3）——
	"DCGM_FI_DEV_PCIE_REPLAY_COUNTER",
	"DCGM_FI_DEV_PCIE_LINK_GEN",
	"DCGM_FI_DEV_PCIE_LINK_WIDTH",

	// —— driver驱动（参评 2）——
	"DCGM_FI_DEV_XID_ERRORS",
	"DCGM_FI_DEV_RELIABILITY_VIOLATION",

	// —— compute算力性能（参评 2）——
	"DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION",
	"DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION",

	// —— stability运行稳定（参评 2）——
	"DCGM_EXP_XID_ERRORS_COUNT",
	"DCGM_FI_DEV_HWR_COUNTER",

	// —— 以下不参与评分，仅供前端详情页/归因分析 ——
	"DCGM_FI_DEV_POWER_USAGE",
	"DCGM_FI_DEV_ENFORCED_POWER_LIMIT",
	"DCGM_FI_DEV_VOLTAGE",
	"DCGM_FI_DEV_SLOWDOWN_TEMP",
	"DCGM_FI_DEV_SHUTDOWN_TEMP",
	"DCGM_FI_DEV_FAN_SPEED",
	"DCGM_FI_DEV_SM_CLOCK",
	"DCGM_FI_DEV_MEM_CLOCK",
	"DCGM_FI_DEV_GPU_UTIL",
	"DCGM_FI_PROF_SM_ACTIVE",
	"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE",
	"DCGM_FI_DEV_PSTATE",
	"DCGM_FI_DEV_FB_TOTAL",
	"DCGM_FI_DEV_FB_USED",
	"DCGM_FI_DEV_FB_FREE",
	"DCGM_EXP_GPU_HEALTH_STATUS",
	"DCGM_FI_DEV_PCIE_MAX_LINK_GEN",
	"DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH",
}

// ── NPU 指标（30 个）──
// 参评 12 个 + HCCS 7 通道原始项（由 aggregateMultiLane 聚合成 _max 参评）+ 归因 11 个
var npuMetricKeys = []string{
	// —— memory显存可靠性（参评 6）——
	"npu_chip_info_hbm_ecc_single_bit_error_cnt",
	"npu_chip_info_hbm_ecc_double_bit_error_cnt",
	"npu_chip_info_hbm_ecc_total_single_bit_error_cnt",
	"npu_chip_info_hbm_ecc_total_double_bit_error_cnt",
	"npu_chip_info_hbm_ecc_single_bit_isolated_pages_cnt",
	"npu_chip_info_hbm_ecc_double_bit_isolated_pages_cnt",

	// —— thermal温度散热（参评 2）——
	"npu_chip_info_temperature",
	"npu_chip_info_hbm_temperature",

	// —— compute算力性能（参评 2）——
	"npu_chip_info_utilization",
	"npu_chip_info_aicore_current_freq",

	// —— reliability昇腾可靠性与运行状态（参评 2）——
	"npu_chip_info_health_status",
	"npu_chip_info_error_code",

	// —— interconnect：7 条 HCCS 通道原始项 ——
	// 采集侧由 aggregateMultiLane 聚合成 ..._crc_err_cnt_max 参与评分，原始项本身 is_health_key=0
	"npu_chip_info_hccs_statistic_info_crc_err_cnt_1",
	"npu_chip_info_hccs_statistic_info_crc_err_cnt_2",
	"npu_chip_info_hccs_statistic_info_crc_err_cnt_3",
	"npu_chip_info_hccs_statistic_info_crc_err_cnt_4",
	"npu_chip_info_hccs_statistic_info_crc_err_cnt_5",
	"npu_chip_info_hccs_statistic_info_crc_err_cnt_6",
	"npu_chip_info_hccs_statistic_info_crc_err_cnt_7",

	// —— 以下不参与评分，仅供前端详情页/归因分析 ——
	"npu_chip_info_power",
	"npu_chip_info_voltage",
	"npu_chip_info_aicore_freq",
	"npu_chip_info_hbm_freq",
	"npu_chip_info_hbm_total_memory",
	"npu_chip_info_hbm_used_memory",
	"npu_chip_info_vector_utilization",
	"npu_chip_info_hccs_bandwidth_info_total_tx",
	"npu_chip_info_hccs_bandwidth_info_total_rx",
	"npu_chip_info_sio_crc_tx_err_cnt",
	"npu_chip_info_sio_crc_rx_err_cnt",
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

// DCGM 仿真值生成
// 基线全部对齐 accel_metric_scoring 的 work_range / 上下界
func (s *SimulatorService) genDCGMValue(st *gpuState, mib string) float64 {
	switch mib {

	// ── 连续量：基线取工作区间中位，抖动不越界 ──
	case "DCGM_FI_DEV_GPU_TEMP": // 20~85，告警上界 92
		return gauge(62, 10, 25, 84) + s.thermalBias(st)
	case "DCGM_FI_DEV_MEMORY_TEMP": // 20~95，告警上界 100
		return gauge(68, 10, 25, 94) + s.thermalBias(st)
	case "DCGM_FI_DEV_NVSWITCH_TEMPERATURE_CURRENT": // 20~90，告警上界 100
		return gauge(58, 9, 25, 88) + s.thermalBias(st)
	case "DCGM_FI_DEV_FB_RESERVED": // 0~2048
		return gauge(1024, 300, 0, 2040)
	case "DCGM_FI_DEV_POWER_USAGE": // 20~400
		return gauge(280, 60, 60, 395)
	case "DCGM_FI_DEV_ENFORCED_POWER_LIMIT":
		return 400
	case "DCGM_FI_DEV_VOLTAGE": // 650~1100 mV
		return gauge(880, 60, 660, 1090)
	case "DCGM_FI_DEV_SLOWDOWN_TEMP":
		return 89
	case "DCGM_FI_DEV_SHUTDOWN_TEMP":
		return 92
	case "DCGM_FI_DEV_FAN_SPEED": // 0~90 %
		return gauge(55, 15, 5, 88)
	case "DCGM_FI_DEV_SM_CLOCK": // 满载 1340，空闲会掉到 210
		if st.faultMode == "thermal" {
			return gauge(900, 100, 210, 1340)
		}
		return gauge(1320, 60, 210, 1410)
	case "DCGM_FI_DEV_MEM_CLOCK": // 1154~1215
		return gauge(1190, 25, 1150, 1215)
	case "DCGM_FI_DEV_GPU_UTIL": // 0~90 %
		return gauge(72, 15, 0, 90)
	case "DCGM_FI_PROF_SM_ACTIVE": // 0.6~1.0
		return gauge(0.82, 0.12, 0.6, 1)
	case "DCGM_FI_PROF_PIPE_TENSOR_ACTIVE": // 0.3~0.8
		return gauge(0.55, 0.12, 0.3, 0.8)
	case "DCGM_FI_DEV_FB_TOTAL":
		return 81920
	case "DCGM_FI_DEV_FB_USED": // 0~77824
		return gauge(52000, 12000, 0, 77000)
	case "DCGM_FI_DEV_FB_FREE": // 4096~81920
		return gauge(29000, 12000, 4200, 81920)

	// ── VIOLATION 累计时长（μs，rate_unit=μs/s，正常速率 0）──
	// 只有对应故障注入时才增长；平时严格不增长，否则会稳定扣分
	case "DCGM_FI_DEV_THERMAL_VIOLATION":
		if st.faultMode == "thermal" {
			return s.bump(st, mib, 20000+rand.Float64()*30000)
		}
		return s.bump(st, mib, 0)
	case "DCGM_FI_DEV_POWER_VIOLATION":
		if st.faultMode == "power_cap" {
			return s.bump(st, mib, 15000+rand.Float64()*25000)
		}
		return s.bump(st, mib, 0)
	case "DCGM_FI_DEV_BOARD_LIMIT_VIOLATION":
		if st.faultMode == "power_cap" {
			return s.bump(st, mib, 10000+rand.Float64()*20000)
		}
		return s.bump(st, mib, 0)
	case "DCGM_FI_DEV_RELIABILITY_VIOLATION":
		if st.faultMode == "reliability" {
			return s.bump(st, mib, 10000+rand.Float64()*20000)
		}
		return s.bump(st, mib, 0)
	case "DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION",
		"DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION":
		if st.faultMode == "thermal" || st.faultMode == "power_cap" {
			return s.bump(st, mib, 12000+rand.Float64()*20000)
		}
		return s.bump(st, mib, 0)

	// ── ECC / 显存：SBE 允许低速增长，DBE 只在故障时增长 ──
	case "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL", "DCGM_FI_DEV_ECC_SBE_AGG_TOTAL":
		return s.bump(st, mib, prob(0.10, 1)) // 约 24 次/天，落在正常速率内
	case "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL", "DCGM_FI_DEV_ECC_DBE_AGG_TOTAL",
		"DCGM_FI_DEV_ECC_DBE_VOL_DEV", "DCGM_FI_DEV_ECC_DBE_AGG_DEV",
		"DCGM_FI_DEV_ECC_DBE_AGG_SRM", "DCGM_FI_DEV_ECC_DBE_VOL_SRM":
		if st.faultMode == "ecc_dbe" {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)
	case "DCGM_FI_DEV_RETIRED_SBE":
		return s.bump(st, mib, prob(0.02, 1))
	case "DCGM_FI_DEV_RETIRED_DBE", "DCGM_FI_DEV_RETIRED_PENDING":
		if st.faultMode == "ecc_dbe" {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)
	case "DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS":
		return s.bump(st, mib, prob(0.02, 1))
	case "DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS":
		// 存量型：一旦发生永久 ≥1，不可恢复
		if st.faultMode == "remap_fail" {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)

	// ── 布尔标志 ──
	case "DCGM_FI_DEV_ROW_REMAP_FAILURE":
		if st.faultMode == "remap_fail" {
			return 1
		}
		return 0
	case "DCGM_FI_DEV_ROW_REMAP_PENDING":
		if st.faultMode == "remap_fail" || st.faultMode == "ecc_dbe" {
			return 1
		}
		return 0
	case "DCGM_FI_DEV_NVSWITCH_RESET_REQUIRED":
		if st.faultMode == "nvswitch" {
			return 1
		}
		return 0

	// ── NVLink / NVSwitch 计数 ──
	case "DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL",
		"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL",
		"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL":
		if st.faultMode == "nvlink" {
			return s.bump(st, mib, 3+rand.Float64()*8) // 超过 10 次/分钟告警线
		}
		return s.bump(st, mib, prob(0.05, 1))
	case "DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL",
		"DCGM_FI_DEV_GPU_NVLINK_ERRORS":
		// 正常速率 0：平时严格不增长
		if st.faultMode == "nvlink" {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)
	case "DCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS":
		if st.faultMode == "nvswitch" {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)
	case "DCGM_FI_DEV_NVSWITCH_LINK_NON_FATAL_ERRORS":
		// 存量型，上界 10
		return s.bump(st, mib, prob(0.03, 1))

	// ── PCIe ──
	case "DCGM_FI_DEV_PCIE_REPLAY_COUNTER":
		if st.faultMode == "pcie" {
			return s.bump(st, mib, 3+rand.Float64()*8) // 超过 8 次/分钟告警线
		}
		return s.bump(st, mib, prob(0.06, 1))
	case "DCGM_FI_DEV_PCIE_LINK_GEN":
		if st.faultMode == "pcie" {
			return 3 // 降代 → 60 分
		}
		return 4
	case "DCGM_FI_DEV_PCIE_LINK_WIDTH":
		if st.faultMode == "pcie" {
			return 8 // 降至 x8 → 60 分
		}
		return 16
	case "DCGM_FI_DEV_PCIE_MAX_LINK_GEN":
		return 4
	case "DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH":
		return 16

	// ── 枚举 / 位掩码 ──
	case "DCGM_FI_DEV_CLOCK_THROTTLE_REASONS":
		// 0x8 HW_SLOWDOWN、0x40 HW_THERMAL、0x80 HW_POWER_BRAKE → critical (0xC8)
		// 0x4 SW_POWER_CAP、0x20 SW_THERMAL              → warning  (0x24)
		switch st.faultMode {
		case "thermal":
			return 0x40
		case "power_cap":
			return 0x4
		}
		return 0
	case "DCGM_FI_DEV_NVSWITCH_LINK_STATUS":
		// 2=ACTIVE 1=SAFE 0=OFF 3=ERROR
		if st.faultMode == "nvswitch" {
			return 3
		}
		return 2
	case "DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS":
		if st.faultMode == "nvswitch" {
			return 1 // 非 0 即 critical
		}
		return 0
	case "DCGM_FI_DEV_PSTATE":
		if st.faultMode == "thermal" {
			return 3
		}
		return 0
	case "DCGM_EXP_GPU_HEALTH_STATUS":
		// 0=HEALTHY 10=WARNING 20=FAILURE
		if st.faultMode != "" {
			return 10
		}
		return 0

	// ── XID：value 即错误码，评分走 xidScore 查表 ──
	case "DCGM_FI_DEV_XID_ERRORS":
		return s.xidValue(st)
	case "DCGM_EXP_XID_ERRORS_COUNT":
		if s.xidValue(st) > 0 {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)
	}
	return 0
}

// ════════════════════════════════════════════════════════════
// NPU（昇腾）仿真值生成
// ════════════════════════════════════════════════════════════
func (s *SimulatorService) genNPUValue(st *gpuState, mib string) float64 {
	switch mib {

	// ── thermal：20~85 正常，告警上界 95 ──
	case "npu_chip_info_temperature":
		return gauge(62, 10, 25, 84) + s.npuThermalBias(st)
	case "npu_chip_info_hbm_temperature":
		return gauge(66, 10, 25, 84) + s.npuThermalBias(st)

	// ── compute ──
	case "npu_chip_info_utilization": // 60~99，告警上界 100
		if st.faultMode == "npu_derate" {
			return gauge(35, 10, 0, 59) // 跌破下界 60 → 警告
		}
		return gauge(85, 10, 60, 99)
	case "npu_chip_info_aicore_current_freq": // 1710~1800
		if st.faultMode == "npu_derate" {
			return gauge(1200, 150, 300, 1700)
		}
		return gauge(1770, 25, 1710, 1800)
	case "npu_chip_info_aicore_freq":
		return 1800
	case "npu_chip_info_hbm_freq": // 1520~1600
		return gauge(1570, 25, 1520, 1600)

	// ── memory：SBE 允许低速增长，DBE 只在故障时增长 ──
	case "npu_chip_info_hbm_ecc_single_bit_error_cnt",
		"npu_chip_info_hbm_ecc_total_single_bit_error_cnt":
		return s.bump(st, mib, prob(0.10, 1)) // 约 24 次/天，正常速率内
	case "npu_chip_info_hbm_ecc_double_bit_error_cnt",
		"npu_chip_info_hbm_ecc_total_double_bit_error_cnt":
		if st.faultMode == "hbm_dbe" {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)
	case "npu_chip_info_hbm_ecc_single_bit_isolated_pages_cnt":
		// 存量型，上界 64
		return s.bump(st, mib, prob(0.01, 1))
	case "npu_chip_info_hbm_ecc_double_bit_isolated_pages_cnt":
		if st.faultMode == "hbm_dbe" {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)
	case "npu_chip_info_hbm_total_memory":
		return 65536
	case "npu_chip_info_hbm_used_memory": // 3300~62259
		return gauge(38000, 10000, 3400, 62000)

	// ── reliability：枚举 ──
	case "npu_chip_info_health_status":
		// npu-exporter 语义：1 = 健康，0 = 不健康
		if st.faultMode != "" {
			return 0
		}
		return 1
	case "npu_chip_info_error_code":
		switch st.faultMode {
		case "hbm_dbe":
			return 1001
		case "npu_thermal":
			return 2001
		case "hccs_crc":
			return 3001
		case "npu_derate":
			return 4001
		}
		return 0

	// ── interconnect：7 条 HCCS 通道 CRC 计数（正常速率 0，次/小时）──
	case "npu_chip_info_hccs_statistic_info_crc_err_cnt_1",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_2",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_3",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_4",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_5",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_6",
		"npu_chip_info_hccs_statistic_info_crc_err_cnt_7":
		// 故障时只让其中一条通道报错，验证 aggregateMultiLane 的"取最差"是否生效
		if st.faultMode == "hccs_crc" && mib == "npu_chip_info_hccs_statistic_info_crc_err_cnt_3" {
			return s.bump(st, mib, 1)
		}
		return s.bump(st, mib, 0)
	case "npu_chip_info_hccs_bandwidth_info_total_tx",
		"npu_chip_info_hccs_bandwidth_info_total_rx": // 23.6~112 GB/s
		if st.faultMode == "hccs_crc" {
			return gauge(20, 5, 5, 40)
		}
		return gauge(75, 20, 24, 112)
	case "npu_chip_info_sio_crc_tx_err_cnt",
		"npu_chip_info_sio_crc_rx_err_cnt":
		return s.bump(st, mib, 0)

	// ── power / 其他归因 ──
	case "npu_chip_info_power": // 90~400
		return gauge(240, 60, 95, 395)
	case "npu_chip_info_voltage": // 0.76~0.84
		return gauge(0.80, 0.02, 0.76, 0.84)
	case "npu_chip_info_vector_utilization": // 0~100
		return gauge(60, 20, 0, 100)
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
			modes := []string{"hbm_dbe", "npu_thermal", "hccs_crc", "npu_derate"}
			st.faultMode = modes[rand.Intn(len(modes))]
		} else {
			modes := []string{"thermal", "power_cap", "ecc_dbe", "xid_fatal",
				"nvlink", "nvswitch", "pcie", "remap_fail", "reliability"}
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
		return 79 // GPU 掉出总线，致命
	case "ecc_dbe":
		return 48 // DBE，致命
	case "nvlink":
		return 74 // NVLink 内部错误，致命
	case "nvswitch":
		return 64 // 行重映射失败，致命
	case "reliability":
		return 31 // 应用侧内存越界，按 xidWarnSet 判警告
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
