-- 评分策略种子：GPU(DCGM) 7 维 + NPU(昇腾) 8 维
-- 前置：accel_metric_scoring.sql 已执行（metric_definition 有 166 行）
-- 执行：mysql -uroot -p gpu_health < scripts/seed_strategy.sql
-- ============================================================
SET NAMES utf8mb4;
USE gpu_health;

-- strategy_metric_rule 可能残留旧列（curve_type/curve_params NOT NULL），先重建
DROP TABLE IF EXISTS `strategy_metric_rule`;
CREATE TABLE `strategy_metric_rule` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `strategy_id`    BIGINT UNSIGNED NOT NULL,
  `metric_key`     VARCHAR(191)    NOT NULL COMMENT '对应 metric_definition.metric_name',
  `weight`         DECIMAL(6,3)    NOT NULL DEFAULT 1.000 COMMENT '指标在所属维度内的权重',
  `is_veto`        TINYINT(1)      NOT NULL DEFAULT 0,
  `veto_threshold` DOUBLE          NOT NULL DEFAULT 0 COMMENT '>0 时按此阈值否决；<=0 时按"落入故障档"否决',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_strategy_metric` (`strategy_id`,`metric_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

DELETE FROM `scoring_strategy` WHERE code IN ('default','npu_default');

-- ---------- GPU / DCGM 默认策略（7 维，和 = 1.000）----------
INSERT INTO `scoring_strategy`
  (code, name, description, dimension_weights, is_default, created_at, updated_at)
VALUES
('DCGM_default','DCGM默认策略','NVIDIA GPU 场景，',
 '{"memory显存可靠性":0.26,"driver驱动（DCGM）":0.20,"nvlink片间互连（DCGM）":0.16,"thermal温度散热":0.13,"pcie总线":0.10,"power功耗电源":0.09,"compute算力性能":0.06}',
 1, NOW(), NOW());
SET @gpu_sid = LAST_INSERT_ID();

-- ---------- NPU / 昇腾默认策略（8 维，和 = 1.000）----------
INSERT INTO `scoring_strategy`
  (code, name, description, dimension_weights, is_default, created_at, updated_at)
VALUES
('npu_default','昇腾NPU默认策略','华为昇腾 NPU 训练场景',
 '{"memory显存可靠性":0.24,"reliability昇腾可靠性与运行状态":0.20,"interconnect昇腾互连通信":0.16,"thermal温度散热":0.12,"pcie总线":0.09,"power功耗电源":0.08,"compute算力性能":0.07,"auxiliary辅助与效率指标":0.04}',
 0, NOW(), NOW());
SET @npu_sid = LAST_INSERT_ID();

-- ---------- 指标规则：按 health_purpose 给差异化初始权重 ----------
-- 1=核心 → 1.0，2=归因 → 0.5，3=配置合规 → 0.3，4=性能容量 → 0.3
-- veto_threshold 统一给 0，交由评分引擎按"落入故障档"判定（见第 5 步）
INSERT INTO `strategy_metric_rule` (strategy_id, metric_key, weight, is_veto, veto_threshold)
SELECT @gpu_sid, metric_name,
       CASE health_purpose WHEN 1 THEN 1.0 WHEN 2 THEN 0.5 ELSE 0.3 END,
       IFNULL(is_veto, 0), 0
FROM metric_definition
WHERE card_type = 'GPU' AND IFNULL(is_health_key, 1) = 1;

INSERT INTO `strategy_metric_rule` (strategy_id, metric_key, weight, is_veto, veto_threshold)
SELECT @npu_sid, metric_name,
       CASE health_purpose WHEN 1 THEN 1.0 WHEN 2 THEN 0.5 ELSE 0.3 END,
       IFNULL(is_veto, 0), 0
FROM metric_definition
WHERE card_type = 'NPU' AND IFNULL(is_health_key, 1) = 1;

SELECT code, name, (SELECT COUNT(*) FROM strategy_metric_rule r WHERE r.strategy_id = s.id) AS rule_cnt
FROM scoring_strategy s;

-- 降频原因位掩码：0x8 HW_SLOWDOWN / 0x40 HW_THERMAL / 0x80 HW_POWER_BRAKE 高危
--                 0x4 SW_POWER_CAP / 0x20 SW_THERMAL 中危
UPDATE metric_definition SET enum_score =
  '{"mode":"bitmask","critical":"0xC8","warning":"0x24"}'
WHERE metric_name = 'DCGM_FI_DEV_CLOCK_THROTTLE_REASONS';

-- P-State：0 最佳，>0 逐档扣分
UPDATE metric_definition SET enum_score =
  '{"mode":"enum","default":60,"map":{"0":100,"1":90,"2":80,"3":70}}'
WHERE metric_name = 'DCGM_FI_DEV_PSTATE';

-- PCIe 链路代数/宽度：低于额定即降级
UPDATE metric_definition SET enum_score =
  '{"mode":"enum","default":20,"map":{"4":100,"5":100,"3":60}}'
WHERE metric_name = 'DCGM_FI_DEV_PCIE_LINK_GEN';
UPDATE metric_definition SET enum_score =
  '{"mode":"enum","default":20,"map":{"16":100,"8":60,"4":20}}'
WHERE metric_name = 'DCGM_FI_DEV_PCIE_LINK_WIDTH';

-- NVSwitch 链路状态 / 致命错误
UPDATE metric_definition SET enum_score =
  '{"mode":"enum","default":20,"map":{"2":100,"1":60,"0":20}}'
WHERE metric_name = 'DCGM_FI_DEV_NVSWITCH_LINK_STATUS';
UPDATE metric_definition SET enum_score =
  '{"mode":"enum","default":20,"map":{"0":100}}'
WHERE metric_name IN ('DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS');

-- 昇腾：健康状态 / 错误码 / 网络状态 / HCCS 状态（0 或 OK 为健康）
UPDATE metric_definition SET enum_score =
  '{"mode":"enum","default":20,"map":{"0":100,"1":60}}'
WHERE metric_name IN (
  'npu_chip_info_health_status','npu_chip_info_error_code','健康告警码（error code）',
  'npu_chip_info_network_status','hccs health status');
PCIE_MAX_LINK_GEN / MAX_LINK_WIDTH 是设备常量，不用配，它们的 health_purpose=2（归因）权重本来就低。

