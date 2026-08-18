-- ============================================================================
-- 维度重划种子：DCGM 7 维度 + NPU 8 维度（方案B，key 完全独立，均带厂商前缀）
-- 执行：mysql ... gpu_health < scripts/seed_dimensions.sql
-- 本轮只做维度归类，字段类型(normal_range等)后续统一整改。
-- ============================================================================
USE gpu_health;

-- ----------------------------------------------------------------------------
-- 1. 更新现有 DCGM 指标的维度（4维 → 7维，key 带 dcgm_ 前缀）
-- ----------------------------------------------------------------------------
-- dcgm_pcie
UPDATE metric_definition SET dimension='dcgm_pcie'    WHERE metric_key='DCGM_FI_DEV_PCIE_REPLAY_COUNTER';
-- dcgm_memory（显存与ECC）
UPDATE metric_definition SET dimension='dcgm_memory'  WHERE metric_key IN (
  'DCGM_FI_DEV_ECC_SBE_VOL_TOTAL','DCGM_FI_DEV_ECC_DBE_VOL_TOTAL',
  'DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS','DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS',
  'DCGM_FI_DEV_ROW_REMAP_FAILURE');
-- dcgm_thermal（温度散热）
UPDATE metric_definition SET dimension='dcgm_thermal' WHERE metric_key IN (
  'DCGM_FI_DEV_GPU_TEMP','DCGM_FI_DEV_MEMORY_TEMP','DCGM_FI_DEV_THERMAL_VIOLATION');
-- dcgm_power（功耗电源）
UPDATE metric_definition SET dimension='dcgm_power'   WHERE metric_key='DCGM_FI_DEV_POWER_USAGE';
-- dcgm_nvlink（NVLink片间互连）
UPDATE metric_definition SET dimension='dcgm_nvlink'  WHERE metric_key IN (
  'DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL',
  'DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL','DCGM_FI_DEV_FABRIC_HEALTH_MASK');
-- dcgm_driver（驱动/XID稳定性）
UPDATE metric_definition SET dimension='dcgm_driver'  WHERE metric_key IN (
  'DCGM_FI_DEV_XID_ERRORS','DCGM_FI_DEV_CLOCKS_EVENT_REASONS','DCGM_FI_DEV_GPU_RESET_COUNT');
-- dcgm_compute（算力性能）
UPDATE metric_definition SET dimension='dcgm_compute' WHERE metric_key IN (
  'DCGM_FI_PROF_GR_ENGINE_ACTIVE','DCGM_FI_PROF_SM_ACTIVE','DCGM_FI_PROF_PIPE_TENSOR_ACTIVE',
  'DCGM_FI_PROF_DRAM_ACTIVE','DCGM_FI_DEV_SM_CLOCK','DCGM_FI_DEV_FB_USED_PERCENT');

-- 更新 DCGM 默认策略的维度权重（7维，硬件敏感，key 带 dcgm_ 前缀）
UPDATE scoring_strategy
SET dimension_weights='{"dcgm_memory":0.28,"dcgm_driver":0.22,"dcgm_nvlink":0.18,"dcgm_thermal":0.12,"dcgm_pcie":0.08,"dcgm_power":0.07,"dcgm_compute":0.05}'
WHERE code='default';

-- ----------------------------------------------------------------------------
-- 2. 新增 NPU 指标定义（8维度，device_type=npu, 本轮仅维度归类）
--    normal_range/abnormal_range 暂留空，后续统一整改
-- ----------------------------------------------------------------------------
INSERT IGNORE INTO metric_definition
 (metric_key, display_name, unit, metric_type, dimension, concept, device_type, normal_range, abnormal_range, remark, is_health_key, created_at, updated_at)
VALUES
-- npu_pcie
('NPU_PCIE_REPLAY_COUNTER','PCIe重传计数','count','counter','npu_pcie','PCIe重传计数','npu','','','',1,NOW(),NOW()),
('NPU_PCIE_CRC_ERROR','PCIe CRC校验错误','count','counter','npu_pcie','PCIe CRC校验错误','npu','','','',1,NOW(),NOW()),
('NPU_PCIE_TXRX_ERROR','PCIe收发错误','count','counter','npu_pcie','PCIe收发错误','npu','','','',1,NOW(),NOW()),
('NPU_PCIE_LINK_WIDTH_GEN','PCIe链路宽度与代数','enum','gauge','npu_pcie','PCIe链路宽度与代数','npu','','','',1,NOW(),NOW()),
-- npu_memory
('NPU_ECC_ENABLE','ECC使能状态','bool','gauge','npu_memory','ECC使能状态','npu','','','',1,NOW(),NOW()),
('NPU_HBM_SBE_RATE','HBM单比特错误率','rate','gauge','npu_memory','HBM单比特错误率','npu','','','',1,NOW(),NOW()),
('NPU_HBM_DBE','HBM双比特错误','count','counter','npu_memory','HBM双比特错误','npu','','','致命,一票否决',1,NOW(),NOW()),
('NPU_DDR_ECC_ERROR','DDR ECC错误','count','counter','npu_memory','DDR ECC错误','npu','','','',1,NOW(),NOW()),
('NPU_ISOLATED_PAGES','已隔离内存页数','count','gauge','npu_memory','已隔离内存页数','npu','','','',1,NOW(),NOW()),
('NPU_ISOLATION_EXHAUSTED','隔离资源耗尽状态','bool','gauge','npu_memory','隔离资源耗尽状态','npu','','','',1,NOW(),NOW()),
('NPU_ISOLATION_PENDING','待处理隔离页数','count','gauge','npu_memory','待处理隔离页数','npu','','','',1,NOW(),NOW()),
('NPU_HBM_TOTAL','HBM总容量','MB','gauge','npu_memory','HBM总容量','npu','','','',1,NOW(),NOW()),
('NPU_HBM_USAGE_RATE','HBM使用率','ratio','gauge','npu_memory','HBM使用率','npu','','','',1,NOW(),NOW()),
('NPU_HBM_IDLE_RESIDUAL','HBM空闲残留量','MB','gauge','npu_memory','HBM空闲残留量','npu','','','',1,NOW(),NOW()),
('NPU_DDR_USAGE_RATE','DDR使用率','ratio','gauge','npu_memory','DDR使用率','npu','','','',1,NOW(),NOW()),
('NPU_DDR_HUGEPAGES_RATE','DDR大页使用率','ratio','gauge','npu_memory','DDR大页使用率','npu','','','',1,NOW(),NOW()),
('NPU_PROC_MEM_USAGE','进程内存使用量','MB','gauge','npu_memory','进程内存使用量','npu','','','',1,NOW(),NOW()),
-- npu_thermal
('NPU_CHIP_TEMP','芯片温度','C','gauge','npu_thermal','芯片温度','npu','','','',1,NOW(),NOW()),
('NPU_HBM_TEMP','HBM温度','C','gauge','npu_thermal','HBM温度','npu','','','',1,NOW(),NOW()),
('NPU_DUAL_CHIP_TEMP_DIFF','双芯片温差','C','gauge','npu_thermal','双芯片温差','npu','','','',1,NOW(),NOW()),
('NPU_BOARD_SENSOR_TEMP','单板传感器温度','C','gauge','npu_thermal','单板传感器温度','npu','','','',1,NOW(),NOW()),
('NPU_TEMP_VIOALTION_RATIO','温度违规占比','ratio','gauge','npu_thermal','温度违规占比','npu','','','',1,NOW(),NOW()),
('NPU_FAN_STATUS','风扇状态','enum','gauge','npu_thermal','风扇状态','npu','','','',1,NOW(),NOW()),
-- npu_power
('NPU_POWER_USAGE','实时功耗','W','gauge','npu_power','实时功耗','npu','','','',1,NOW(),NOW()),
('NPU_POWER_Ratio_R','功耗比率R','ratio','gauge','npu_power','功耗比率R','npu','','','',1,NOW(),NOW()),
('NPU_CHIP_VOLTAGE','芯片电压','V','gauge','npu_power','芯片电压','npu','','','',1,NOW(),NOW()),
('NPU_LOW_POWER_DERATE','低功耗降额状态','enum','gauge','npu_power','低功耗降额状态','npu','','','',1,NOW(),NOW()),
('NPU_MCU_STATUS','MCU微控制器状态','enum','gauge','npu_power','MCU微控制器状态','npu','','','',1,NOW(),NOW()),
('NPU_POWER_FAULT_CODE','电源故障码','code','gauge','npu_power','电源故障码','npu','','','',1,NOW(),NOW()),
-- npu_interconnect（昇腾专有互连）
('NPU_HCCS_LINK_STATE','HCCS链路状态','enum','gauge','npu_interconnect','HCCS链路状态','npu','','','',1,NOW(),NOW()),
('NPU_HCCS_LINK_SPEED','HCCS链路速率','Gbps','gauge','npu_interconnect','HCCS链路速率','npu','','','',1,NOW(),NOW()),
('NPU_HCCS_BW_ACHIEVE','HCCS带宽达成率','ratio','gauge','npu_interconnect','HCCS带宽达成率','npu','','','',1,NOW(),NOW()),
('NPU_ROCE_LINK_STATE','RoCE链路状态','enum','gauge','npu_interconnect','RoCE链路状态','npu','','','',1,NOW(),NOW()),
('NPU_ROCE_NET_HEALTH','RoCE网络健康度','enum','gauge','npu_interconnect','RoCE网络健康度','npu','','','',1,NOW(),NOW()),
('NPU_ROCE_LINK_SPEED','RoCE链路速率','Gbps','gauge','npu_interconnect','RoCE链路速率','npu','','','',1,NOW(),NOW()),
('NPU_ROCE_PACKET_LOSS','RoCE丢包数','count','counter','npu_interconnect','RoCE丢包数','npu','','','',1,NOW(),NOW()),
('NPU_PFC_ANOMALY','PFC优先级流控异常','bool','gauge','npu_interconnect','PFC优先级流控异常','npu','','','',1,NOW(),NOW()),
('NPU_ROCE_PORT_RATE','RoCE端口速率','Gbps','gauge','npu_interconnect','RoCE端口速率','npu','','','',1,NOW(),NOW()),
('NPU_OPTICAL_TEMP','光模块温度','C','gauge','npu_interconnect','光模块温度','npu','','','',1,NOW(),NOW()),
('NPU_OPTICAL_POWER','光模块光功率','dBm','gauge','npu_interconnect','光模块光功率','npu','','','',1,NOW(),NOW()),
('NPU_OPTICAL_PRESENT','光模块在位状态','bool','gauge','npu_interconnect','光模块在位状态','npu','','','',1,NOW(),NOW()),
('NPU_P2P_ENABLE','P2P使能状态','bool','gauge','npu_interconnect','P2P使能状态','npu','','','',1,NOW(),NOW()),
('NPU_TOPOLOGY','拓扑信息','enum','gauge','npu_interconnect','拓扑信息','npu','','','',1,NOW(),NOW()),
-- npu_reliability（可靠性与运行状态）
('NPU_HEALTH_STATUS','健康状态','enum','gauge','npu_reliability','健康状态','npu','','','',1,NOW(),NOW()),
('NPU_HEALTH_STATUS_BINARY','健康状态二进制码','code','gauge','npu_reliability','健康状态二进制码','npu','','','',1,NOW(),NOW()),
('NPU_ERROR_CODE','错误码','code','gauge','npu_reliability','错误码','npu','','','',1,NOW(),NOW()),
('NPU_ERROR_COUNT','错误计数','count','counter','npu_reliability','错误计数','npu','','','',1,NOW(),NOW()),
('NPU_VISIBILITY','设备可见性','bool','gauge','npu_reliability','设备可见性','npu','','','',1,NOW(),NOW()),
('NPU_DEVICE_OS_HEARTBEAT','设备OS心跳','enum','gauge','npu_reliability','设备OS心跳','npu','','','',1,NOW(),NOW()),
('NPU_RESET_COUNT','复位次数','count','counter','npu_reliability','复位次数','npu','','','',1,NOW(),NOW()),
('NPU_UPTIME_SINCE_FAULT','故障后运行时长','s','gauge','npu_reliability','故障后运行时长','npu','','','',1,NOW(),NOW()),
('NPU_VERSION_CONSISTENCY','版本一致性','bool','gauge','npu_reliability','版本一致性','npu','','','',1,NOW(),NOW()),
('NPU_DRIVER_STATUS','驱动状态','enum','gauge','npu_reliability','驱动状态','npu','','','',1,NOW(),NOW()),
('NPU_FLASH_STATUS','Flash状态','enum','gauge','npu_reliability','Flash状态','npu','','','',1,NOW(),NOW()),
('NPU_I2C_CHECK','I2C总线检测','enum','gauge','npu_reliability','I2C总线检测','npu','','','',1,NOW(),NOW()),
('NPU_WORKMODE','工作模式','enum','gauge','npu_reliability','工作模式','npu','','','',1,NOW(),NOW()),
('NPU_VNPVMODE','VNPV模式','enum','gauge','npu_reliability','VNPV模式','npu','','','',1,NOW(),NOW()),
('NPU_LICENSE_STATUS','许可证状态','enum','gauge','npu_reliability','许可证状态','npu','','','',1,NOW(),NOW()),
('NPU_COLLECT_HEALTH','采集健康状态','enum','gauge','npu_reliability','采集健康状态','npu','','','',1,NOW(),NOW()),
('NPU_FIRST_POWER_DATE','首次上电日期','date','gauge','npu_reliability','首次上电日期','npu','','','',0,NOW(),NOW()),
-- npu_auxiliary（辅助与效率指标，不参与评分 is_health_key=0）
('NPU_AICORE_UTIL','AI核心利用率','ratio','gauge','npu_auxiliary','AI核心利用率','npu','','','',0,NOW(),NOW()),
('NPU_AIVECTOR_UTIL','AI向量单元利用率','ratio','gauge','npu_auxiliary','AI向量单元利用率','npu','','','',0,NOW(),NOW()),
('NPU_AICPU_USAGE','AI CPU使用率','ratio','gauge','npu_auxiliary','AI CPU使用率','npu','','','',0,NOW(),NOW()),
('NPU_MEMORY_USAGE','内存使用率','ratio','gauge','npu_auxiliary','内存使用率','npu','','','',0,NOW(),NOW()),
('NPU_DDR_BW_USAGE','DDR带宽使用率','ratio','gauge','npu_auxiliary','DDR带宽使用率','npu','','','',0,NOW(),NOW()),
('NPU_HBM_BW_USAGE','HBM带宽使用率','ratio','gauge','npu_auxiliary','HBM带宽使用率','npu','','','',0,NOW(),NOW()),
('NPU_HUGEPAGES_USAGE','大页使用率','ratio','gauge','npu_auxiliary','大页使用率','npu','','','',0,NOW(),NOW()),
('NPU_DEVICE_SHARE','设备共享状态','enum','gauge','npu_auxiliary','设备共享状态','npu','','','',0,NOW(),NOW()),
('NPU_AICPU_CORE_CONFIG','AI CPU核心配置','enum','gauge','npu_auxiliary','AI CPU核心配置','npu','','','',0,NOW(),NOW()),
('NPU_PROCESS_INFO','进程信息','enum','gauge','npu_auxiliary','进程信息','npu','','','',0,NOW(),NOW()),
-- npu_compute（算力性能）
('NPU_AICORE_FREQ','AI核心目标频率','MHz','gauge','npu_compute','AI核心目标频率','npu','','','',1,NOW(),NOW()),
('NPU_AICORE_CURFREQ','AI核心当前频率','MHz','gauge','npu_compute','AI核心当前频率','npu','','','',1,NOW(),NOW()),
('NPU_AICORE_FREQ_ACHIEVE','AI核心频率达成率','ratio','gauge','npu_compute','AI核心频率达成率','npu','','','',1,NOW(),NOW()),
('NPU_AICORE_COUNT','AI核心数量','count','gauge','npu_compute','AI核心数量','npu','','','',0,NOW(),NOW()),
('NPU_FLOPs_ACHIEVE','浮点运算性能达成率','ratio','gauge','npu_compute','浮点运算性能达成率','npu','','','',1,NOW(),NOW()),
('NPU_HBM_BW_ACHIEVE','HBM带宽达成率','ratio','gauge','npu_compute','HBM带宽达成率','npu','','','',1,NOW(),NOW()),
('NPU_CTRL_CPU_USAGE','控制CPU使用率','ratio','gauge','npu_compute','控制CPU使用率','npu','','','',0,NOW(),NOW()),
('NPU_OUTLIER_ZSCORE','离群Z-Score分数','score','gauge','npu_compute','离群Z-Score分数','npu','','','',1,NOW(),NOW());

-- ----------------------------------------------------------------------------
-- 3. NPU 默认策略（8维，硬件敏感训练场景，和=1.0）
--    curve/veto 本轮先留 none，字段统一整改后再配曲线
--    注意：scoring_strategy 已无 version 列，不要写入
-- ----------------------------------------------------------------------------
INSERT INTO scoring_strategy (code, name, description, dimension_weights, is_default, created_at, updated_at)
VALUES ('npu_default','NPU默认策略(训练严格)','昇腾NPU训练场景，硬件敏感',
  '{"npu_memory":0.24,"npu_reliability":0.20,"npu_interconnect":0.16,"npu_thermal":0.12,"npu_pcie":0.08,"npu_power":0.08,"npu_compute":0.07,"npu_auxiliary":0.05}',
  0, NOW(), NOW());

SET @npu_sid = LAST_INSERT_ID();

-- NPU 策略指标规则（本轮先用 none 曲线占位，参与评分的指标 is_health_key=1 的全部纳入）
INSERT INTO strategy_metric_rule (strategy_id, metric_key, weight, curve_type, curve_params, is_veto, veto_threshold)
SELECT @npu_sid, metric_key, 1.0, 'none', NULL, 0, 0
FROM metric_definition
WHERE device_type='npu' AND is_health_key=1;
