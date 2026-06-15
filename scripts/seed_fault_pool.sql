-- 故障池种子数据
-- 前置：先启动一次 server 让 AutoMigrate 建好 fault_rule / fault_event 表
--       并给 metric_definition 补上 direction/warn_threshold/crit_threshold/normal_min/normal_max 列。
-- 然后执行本脚本：mysql -u<user> -p<pwd> <db> < scripts/seed_fault_pool.sql

-- 1) 给关键指标补结构化阈值（驱动需求1的异常判定 + 阈值类故障）
UPDATE metric_definition SET direction='high_bad', warn_threshold=80,  crit_threshold=87  WHERE metric_key='DCGM_FI_DEV_GPU_TEMP';
UPDATE metric_definition SET direction='high_bad', warn_threshold=85,  crit_threshold=95  WHERE metric_key='DCGM_FI_DEV_MEMORY_TEMP';
UPDATE metric_definition SET direction='high_bad', warn_threshold=500, crit_threshold=700 WHERE metric_key='DCGM_FI_DEV_POWER_USAGE';
UPDATE metric_definition SET direction='low_bad',  warn_threshold=1500,crit_threshold=1000 WHERE metric_key='DCGM_FI_DEV_SM_CLOCK';
UPDATE metric_definition SET direction='high_bad', warn_threshold=10,  crit_threshold=500 WHERE metric_key='DCGM_FI_DEV_ECC_SBE_VOL_TOTAL';
UPDATE metric_definition SET direction='high_bad', warn_threshold=1,   crit_threshold=1   WHERE metric_key='DCGM_FI_DEV_ECC_DBE_VOL_TOTAL';
UPDATE metric_definition SET direction='high_bad', warn_threshold=1,   crit_threshold=100 WHERE metric_key='DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL';
UPDATE metric_definition SET direction='high_bad', warn_threshold=1,   crit_threshold=10  WHERE metric_key='DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL';

-- 2) 故障规则（threshold 类；门限留空表示用指标自身的 crit_threshold）
--    DBE双比特 / 致命XID 已由评分的"一票否决"自动进故障池，无需重复建规则。
INSERT INTO fault_rule (name, trigger_type, metric_key, operator, threshold, severity, knowledge_id, enabled, created_at, updated_at) VALUES
('GPU核心温度过高','threshold','DCGM_FI_DEV_GPU_TEMP','', NULL,'critical',
   (SELECT id FROM fault_knowledge WHERE fault_type='散热异常' LIMIT 1), 1, NOW(), NOW()),
('显存温度过高','threshold','DCGM_FI_DEV_MEMORY_TEMP','', NULL,'critical',
   (SELECT id FROM fault_knowledge WHERE fault_type='散热异常' LIMIT 1), 1, NOW(), NOW()),
('GPU功耗超限','threshold','DCGM_FI_DEV_POWER_USAGE','', NULL,'warning', NULL, 1, NOW(), NOW()),
('SM时钟降频','threshold','DCGM_FI_DEV_SM_CLOCK','', NULL,'warning',
   (SELECT id FROM fault_knowledge WHERE fault_type='散热异常' LIMIT 1), 1, NOW(), NOW()),
('ECC单比特错误过多','threshold','DCGM_FI_DEV_ECC_SBE_VOL_TOTAL','', NULL,'warning', NULL, 1, NOW(), NOW()),
('NVLink CRC错误过多','threshold','DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL','', NULL,'critical',
   (SELECT id FROM fault_knowledge WHERE fault_type='NVLink互连故障' LIMIT 1), 1, NOW(), NOW());
