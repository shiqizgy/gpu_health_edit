-- ============================================================
-- metric 补丁：设定参与评分范围 + 补齐评分所需的边界与枚举规则
-- 前置：accel_metric_scoring.sql 已执行（219 行数据）
-- 幂等，可重复执行
-- ============================================================
SET NAMES utf8mb4;
USE gpu_health;

-- ---------- 1. 参与评分范围：核心用途 + 归属于本卡/链路/共享设施 ----------
UPDATE accel_metric_scoring SET is_health_key = 0;

UPDATE accel_metric_scoring SET is_health_key = 1
WHERE health_purpose_code = 1
  AND owner_subject_code IN (1, 2, 3);

-- ---------- 2. 手工排除：边界配置不合理，会造成稳定误判 ----------
-- SM_CLOCK 上下界都是 1340，但空闲时会掉到 210MHz 左右，必然误扣分
UPDATE accel_metric_scoring SET is_health_key = 0
WHERE metric_name = 'DCGM_FI_DEV_SM_CLOCK';

-- 额定容量类：上下界与告警界全部相等，只能判到警告档，参与评分无意义
UPDATE accel_metric_scoring SET is_health_key = 0
WHERE metric_name IN ('DCGM_FI_DEV_FB_TOTAL', 'npu_chip_info_hbm_total_memory');

-- HCCS 7 条链路聚合成一条（原始项保留在 CK 里供排查，不参与评分）
UPDATE accel_metric_scoring SET is_health_key = 0
WHERE metric_name LIKE 'npu_chip_info_hccs_statistic_info_crc_err_cnt_%'
  AND metric_name NOT LIKE '%_max';

-- ---------- 2.5 NPU 补充参评指标 ----------
-- 说明：NPU 的 health_purpose / owner_subject 标注与 GPU 口径不完全一致，
--       以下几类在语义上属于"单卡健康"，按 hp/os 机械筛选会被漏掉，此处显式补回。

-- ① 功耗与电压：NPU 侧没有 *_VIOLATION 类指标，power 维度否则完全为空
UPDATE accel_metric_scoring SET is_health_key = 1
WHERE metric_name IN ('npu_chip_info_power', 'npu_chip_info_voltage');

-- ② SIO 片间串行 IO 的 CRC 错误：与已参评的 HCCS CRC 完全同类
UPDATE accel_metric_scoring SET is_health_key = 1
WHERE metric_name IN ('npu_chip_info_sio_crc_tx_err_cnt',
                      'npu_chip_info_sio_crc_rx_err_cnt');

-- ③ 片上集成 RoCE 网口与光模块：昇腾网口集成在 NPU 上，链路异常即该卡不可用
UPDATE accel_metric_scoring SET is_health_key = 1
WHERE metric_name IN ('npu_chip_info_link_status',
                      'npu_chip_info_network_status',
                      'npu_chip_roce_rx_err_pkt_num',
                      'npu_chip_optical_state');
第 5 节的枚举规则追加三条（enum_result 里语义已写明，1=正常）：
sql
-- ⑧ 昇腾 RoCE 链路 / 网络状态 / 光模块在位：1 = 正常，0 = 异常
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"1":100,"0":20}}'
WHERE metric_name IN ('npu_chip_info_link_status',
                      'npu_chip_info_network_status',
                      'npu_chip_optical_state');

-- ---------- 3. 补 rate_unit：6 个 VIOLATION 类是 μs 累计时长 ----------
UPDATE accel_metric_scoring SET rate_unit = 'μs/s'
WHERE value_type_code = 4 AND (rate_unit IS NULL OR rate_unit = '');

-- ---------- 4. 补边界：一票否决指标必须能到达故障档 ----------
-- 4.1 连续量：缺告警上界
UPDATE accel_metric_scoring SET alert_upper = 100
WHERE metric_name = 'DCGM_FI_DEV_MEMORY_TEMP' AND alert_upper IS NULL;

-- 4.2 存量型（rate_unit 无时间分母，走 scoreRange）：缺告警上界，最差只能到 60 分
UPDATE accel_metric_scoring SET alert_upper = 0
WHERE metric_name IN ('DCGM_FI_DEV_ECC_DBE_AGG_TOTAL',
                      'DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS');

-- 4.3 速率型：alert_rate=1 时，值为 1 只判警告、否决不触发
--     DBE/XID 这类"发生一次即严重"的，告警速率应为 0
UPDATE accel_metric_scoring SET alert_rate = 0
WHERE is_veto = 1 AND alert_rate = 1;

-- ---------- 5. 枚举 / 位掩码评分规则 ----------
-- ① 降频原因位掩码：0x8 HW_SLOWDOWN / 0x40 HW_THERMAL / 0x80 HW_POWER_BRAKE 高危
--                   0x4 SW_POWER_CAP / 0x20 SW_THERMAL 中危
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"bitmask","critical":"0xC8","warning":"0x24"}'
WHERE metric_name = 'DCGM_FI_DEV_CLOCK_THROTTLE_REASONS';

-- ② PCIe 链路代数：enum_result "4=正常，<4 降代"
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"5":100,"4":100,"3":60,"2":20,"1":20}}'
WHERE metric_name = 'DCGM_FI_DEV_PCIE_LINK_GEN';

-- ③ PCIe 链路宽度：enum_result "16=正常，降至 x8/x4/x1"
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"16":100,"8":60,"4":20,"1":20}}'
WHERE metric_name = 'DCGM_FI_DEV_PCIE_LINK_WIDTH';

-- ④ NVSwitch 链路状态：enum_result "2=ACTIVE 1=SAFE 0=OFF 3=ERROR"
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"2":100,"1":60,"0":20,"3":20}}'
WHERE metric_name = 'DCGM_FI_DEV_NVSWITCH_LINK_STATUS';

-- ⑤ NVSwitch 致命错误：enum_result "无 SXid 上报 / 上报任意致命 SXid"
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"0":100}}'
WHERE metric_name = 'DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS';

-- ⑥ 昇腾健康状态：enum_result 明确 "1 = OK（npu-exporter 中 1 表示健康）；0 = 不健康"
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"1":100,"0":20}}'
WHERE metric_name = 'npu_chip_info_health_status';

-- ⑦ 昇腾错误码：enum_result "0；≠ 0"
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"0":100}}'
WHERE metric_name = 'npu_chip_info_error_code';

-- ---------- 6. 新增聚合指标定义（HCCS 7 链路取最差）----------
INSERT INTO accel_metric_scoring
  (seq_no, official_no, card_type, owner_subject_code, metric_name, value_type_code,
   health_purpose_code, concept, dimension, unit, normal_rate, alert_rate, rate_unit,
   is_veto, is_health_key, vendor)
VALUES
  (9001, 'npu-smi info -t hccs（7 条链路取最大值）', 'NPU', 2,
   'npu_chip_info_hccs_statistic_info_crc_err_cnt_max', 3, 1,
   'HCCS 7 条链路 CRC 错误计数的最大值，任一链路异常即反映在此',
   'interconnect昇腾互连通信', '次', 0, 0, '次/小时', 0, 1, '华为昇腾')
ON DUPLICATE KEY UPDATE is_health_key = 1;

-- ---------- 验证 ----------
SELECT card_type, dimension, COUNT(*) AS cnt
FROM accel_metric_scoring WHERE is_health_key = 1
GROUP BY card_type, dimension ORDER BY card_type, cnt DESC;
