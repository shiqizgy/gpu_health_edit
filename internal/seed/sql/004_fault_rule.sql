SET NAMES utf8mb4;

DELETE FROM fault_rule;
INSERT INTO fault_rule
  (name, trigger_type, metric_key, operator, threshold, severity, enabled, created_at, updated_at)
VALUES
  ('GPU核心温度过高','threshold','DCGM_FI_DEV_GPU_TEMP','',NULL,'critical',1,NOW(),NOW()),
  ('显存温度过高','threshold','DCGM_FI_DEV_MEMORY_TEMP','',NULL,'critical',1,NOW(),NOW()),
  ('GPU功耗超限','threshold','DCGM_FI_DEV_POWER_USAGE','',NULL,'warning',1,NOW(),NOW()),
  ('SM时钟降频','threshold','DCGM_FI_DEV_SM_CLOCK','',NULL,'warning',1,NOW(),NOW()),
  ('ECC单比特错误过多','threshold','DCGM_FI_DEV_ECC_SBE_VOL_TOTAL','',NULL,'warning',1,NOW(),NOW()),
  ('ECC双比特错误','threshold','DCGM_FI_DEV_ECC_DBE_VOL_TOTAL','',NULL,'critical',1,NOW(),NOW()),
  ('NVLink CRC错误过多','threshold','DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL','',NULL,'critical',1,NOW(),NOW()),
  ('昇腾健康状态异常','threshold','npu_chip_info_health_status','',NULL,'critical',1,NOW(),NOW()),
  ('昇腾错误码异常','threshold','npu_chip_info_error_code','',NULL,'critical',1,NOW(),NOW());
