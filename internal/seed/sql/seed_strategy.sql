SET NAMES utf8mb4;
USE gpu_health;

DROP TABLE IF EXISTS `strategy_metric_rule`;
CREATE TABLE `strategy_metric_rule` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `strategy_id`    BIGINT UNSIGNED NOT NULL,
  `metric_key`     VARCHAR(191)    NOT NULL COMMENT '对应 accel_metric_scoring.metric_name',
  `weight`         DECIMAL(6,3)    NOT NULL DEFAULT 1.000 COMMENT '维度内权重，暂统一 1.0',
  `is_veto`        TINYINT(1)      NOT NULL DEFAULT 0,
  `veto_threshold` DOUBLE          NOT NULL DEFAULT 0 COMMENT '>0 按此阈值否决；<=0 按落入故障档否决',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_strategy_metric` (`strategy_id`,`metric_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

DELETE FROM `scoring_strategy` WHERE code IN ('DCGM_default','npu_default');

-- GPU：8 个维度（新数据多了 stability运行稳定），和 = 1.000
INSERT INTO `scoring_strategy`
  (code, name, description, dimension_weights, is_default, created_at, updated_at)
VALUES
('DCGM_default','DCGM默认策略','NVIDIA GPU 训练场景',
 '{"memory显存可靠性":0.26,"driver驱动（DCGM）":0.16,"nvlink片间互连（DCGM）":0.15,"stability运行稳定":0.12,"thermal温度散热":0.12,"pcie总线":0.08,"power功耗电源":0.07,"compute算力性能":0.04}',
 1, NOW(), NOW());
SET @gpu_sid = LAST_INSERT_ID();

-- NPU：6 个维度（只保留真正有参评指标的），和 = 1.000
INSERT INTO `scoring_strategy`
  (code, name, description, dimension_weights, is_default, created_at, updated_at)
VALUES
('npu_default','昇腾NPU默认策略','华为昇腾 NPU 训练场景',
 '{"memory显存可靠性":0.28,"reliability昇腾可靠性与运行状态":0.22,"interconnect昇腾互连通信":0.20,"thermal温度散热":0.14,"power功耗电源":0.10,"compute算力性能":0.06}',
 0, NOW(), NOW());
SET @npu_sid = LAST_INSERT_ID();

-- 规则：权重统一 1.0，否决沿用指标定义
INSERT INTO `strategy_metric_rule` (strategy_id, metric_key, weight, is_veto, veto_threshold)
SELECT @gpu_sid, metric_name, 1.0, IFNULL(is_veto, 0), 0
FROM accel_metric_scoring WHERE card_type = 'GPU' AND is_health_key = 1;

INSERT INTO `strategy_metric_rule` (strategy_id, metric_key, weight, is_veto, veto_threshold)
SELECT @npu_sid, metric_name, 1.0, IFNULL(is_veto, 0), 0
FROM accel_metric_scoring WHERE card_type = 'NPU' AND is_health_key = 1;

SELECT s.code, s.name, COUNT(r.id) AS rule_cnt
FROM scoring_strategy s LEFT JOIN strategy_metric_rule r ON r.strategy_id = s.id
GROUP BY s.id;
-- 执行顺序（podman-seed.sh）：accel_metric_scoring.sql → 003_metric_patch.sql → seed_strategy.sql → seed_fault_pool.sql。
