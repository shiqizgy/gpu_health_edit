SET NAMES utf8mb4;
USE gpu_health;

-- 0 = 不参与评分；1 = 参与（卡自身+链路）；2 = 参与但降权（共享基础设施）
ALTER TABLE `accel_metric_scoring`
  ADD COLUMN IF NOT EXISTS `score_scope` TINYINT NOT NULL DEFAULT 0
  COMMENT '评分范围：0不参与 1参与 2参与但降权(共享设施)' AFTER `is_veto`;

-- ---------- 批量置位 ----------
UPDATE accel_metric_scoring SET score_scope = 0;

UPDATE accel_metric_scoring SET score_scope = 1
WHERE health_purpose_code = 1 AND owner_subject_code IN (1, 2);

UPDATE accel_metric_scoring SET score_scope = 2
WHERE health_purpose_code = 1 AND owner_subject_code = 3;

-- ---------- 手工排除：边界配置不合理，会造成稳定误扣分 ----------
UPDATE accel_metric_scoring SET score_scope = 0
WHERE metric_name = 'DCGM_FI_DEV_SM_CLOCK';
-- 若想保留，改成下面这条（A100 空闲频率约 210MHz，满载 1410MHz）
-- UPDATE accel_metric_scoring SET lower_bound = 210, alert_lower = 0, upper_bound = 1410, alert_upper = 1410
-- WHERE metric_name = 'DCGM_FI_DEV_SM_CLOCK';

-- ---------- 同步 is_health_key（前端"仅参与评分"筛选用）----------
UPDATE accel_metric_scoring SET is_health_key = (score_scope > 0);

-- ---------- 验证 ----------
SELECT card_type, dimension, COUNT(*) FROM accel_metric_scoring
WHERE score_scope > 0 GROUP BY card_type, dimension ORDER BY card_type, COUNT(*) DESC;
然后 seed_strategy.sql 里生成规则的两条 INSERT 改成按 score_scope 筛，并且用它决定初始权重：
sql
INSERT INTO `strategy_metric_rule` (strategy_id, metric_key, weight, is_veto, veto_threshold)
SELECT @gpu_sid, metric_name,
       CASE score_scope WHEN 1 THEN 1.0 WHEN 2 THEN 0.4 END,   -- 共享设施降权
       IFNULL(is_veto, 0), 0
FROM accel_metric_scoring
WHERE card_type = 'GPU' AND score_scope > 0;

INSERT INTO `strategy_metric_rule` (strategy_id, metric_key, weight, is_veto, veto_threshold)
SELECT @npu_sid, metric_name,
       CASE score_scope WHEN 1 THEN 1.0 WHEN 2 THEN 0.4 END,
       IFNULL(is_veto, 0), 0
FROM accel_metric_scoring
WHERE card_type = 'NPU' AND score_scope > 0;


UPDATE accel_metric_scoring SET rate_unit = 'μs/s'
WHERE value_type_code = 4 AND (rate_unit IS NULL OR rate_unit = '');


-- ① 降频原因位掩码（hp=1 os=1）
--    enum_result: 0x0无降频 0x1 GPU_IDLE 0x2 CLOCKS_SETTING（正常）
--                 0x4 SW_POWER_CAP 0x20 SW_THERMAL（中危）
--                 0x8 HW_SLOWDOWN 0x40 HW_THERMAL 0x80 HW_POWER_BRAKE（高危）
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"bitmask","critical":"0xC8","warning":"0x24"}'
WHERE metric_name = 'DCGM_FI_DEV_CLOCK_THROTTLE_REASONS';

-- ② PCIe 链路代数（hp=1 os=2）  enum_result: 4=正常，<4 降代
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":100,"map":{"4":100,"5":100,"3":60,"2":20,"1":20}}'
WHERE metric_name = 'DCGM_FI_DEV_PCIE_LINK_GEN';

-- ③ PCIe 链路宽度（hp=1 os=2）  enum_result: 16=正常，降至 x8/x4/x1
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"16":100,"8":60,"4":20,"1":20}}'
WHERE metric_name = 'DCGM_FI_DEV_PCIE_LINK_WIDTH';

-- ④ NVSwitch 链路状态（hp=1 os=3）  enum_result: 2=ACTIVE 1=SAFE 0=OFF 3=ERROR -1=UNKNOWN
--    注意 -1 会被 validValue() 当无效采样过滤，这里的 map 用不到
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"2":100,"1":60,"0":20,"3":20}}'
WHERE metric_name = 'DCGM_FI_DEV_NVSWITCH_LINK_STATUS';

-- ⑤ 昇腾健康状态（hp=1 os=1，一票否决）  enum_result: 1=OK 0=不健康
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"1":100,"0":20}}'
WHERE metric_name = 'npu_chip_info_health_status';

-- ⑥ 昇腾错误码（hp=1 os=1）  enum_result: 0 正常；≠0 异常
UPDATE accel_metric_scoring SET enum_score =
  '{"mode":"enum","default":20,"map":{"0":100}}'
WHERE metric_name = 'npu_chip_info_error_code';
