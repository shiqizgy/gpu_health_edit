-- ============================================================================
-- GPU 健康度平台 种子数据
-- 表结构由 GORM AutoMigrate 自动创建，本文件只灌初始数据。
-- 执行时机：server 启动(自动建表)之后，simulator 之前。
-- ============================================================================
USE gpu_health;

-- ---------------------------------------------------------------------------
-- 1. 指标定义（25 个，对应文档四个维度）
-- ---------------------------------------------------------------------------
INSERT INTO metric_definition
 (metric_key, display_name, unit, metric_type, dimension, concept, device_type, normal_range, abnormal_range, remark, is_health_key, created_at, updated_at)
VALUES
-- environment 运行环境
('DCGM_FI_DEV_GPU_TEMP','GPU 核心温度','C','gauge','environment','GPU die 温度','gpu','<80','>=87','H100 降频87C/关机92C',1,NOW(),NOW()),
('DCGM_FI_DEV_MEMORY_TEMP','HBM 显存温度','C','gauge','environment','HBM 显存温度','gpu','<85','>=95','HBM3 关机约105C',1,NOW(),NOW()),
('DCGM_FI_DEV_POWER_USAGE','GPU 功耗','W','gauge','environment','瞬时功耗','gpu','<500','>700','',1,NOW(),NOW()),
('DCGM_FI_DEV_THERMAL_VIOLATION','热降频时长','s','counter','environment','累计热降频时长','gpu','0','>0','',1,NOW(),NOW()),
-- performance 性能表现
('DCGM_FI_PROF_GR_ENGINE_ACTIVE','图形引擎活跃度','ratio','gauge','performance','图形引擎活跃比例','gpu','0-1','','利用率主指标',1,NOW(),NOW()),
('DCGM_FI_PROF_SM_ACTIVE','SM 活跃度','ratio','gauge','performance','SM 活跃比例','gpu','0-1','','',1,NOW(),NOW()),
('DCGM_FI_PROF_PIPE_TENSOR_ACTIVE','Tensor Core 活跃度','ratio','gauge','performance','Tensor Core 活跃比例','gpu','0-1','','AI训练核心',1,NOW(),NOW()),
('DCGM_FI_PROF_DRAM_ACTIVE','显存带宽活跃度','ratio','gauge','performance','显存带宽活跃比例','gpu','0-1','','',1,NOW(),NOW()),
('DCGM_FI_DEV_SM_CLOCK','SM 时钟频率','MHz','gauge','performance','SM 时钟','gpu','>1500','<1000','下降意味降频',1,NOW(),NOW()),
('DCGM_FI_DEV_FB_USED_PERCENT','显存使用率','ratio','gauge','performance','显存使用比例','gpu','<0.9','>0.98','',1,NOW(),NOW()),
-- hardware 硬件健康
('DCGM_FI_DEV_ECC_SBE_VOL_TOTAL','ECC 单比特错误','count','counter','hardware','累计单比特ECC错误','gpu','<10','>500','可纠正',1,NOW(),NOW()),
('DCGM_FI_DEV_ECC_DBE_VOL_TOTAL','ECC 双比特错误','count','counter','hardware','累计双比特ECC错误','gpu','0','>=1','致命,一票否决',1,NOW(),NOW()),
('DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS','可纠正行重映射','count','counter','hardware','可纠正行重映射数','gpu','0','>8','',1,NOW(),NOW()),
('DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS','不可纠正行重映射','count','counter','hardware','不可纠正行重映射数','gpu','0','>=1','致命,一票否决',1,NOW(),NOW()),
('DCGM_FI_DEV_ROW_REMAP_FAILURE','行重映射失败','bool','gauge','hardware','行重映射失败标志','gpu','0','1','致命,RMA必要条件',1,NOW(),NOW()),
('DCGM_FI_DEV_PCIE_REPLAY_COUNTER','PCIe 重传','count','counter','hardware','PCIe 重传计数','gpu','<50','>500','',1,NOW(),NOW()),
('DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL','NVLink CRC 错误','count','counter','hardware','NVLink CRC错误计数','gpu','<1','>100','',1,NOW(),NOW()),
('DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL','NVLink 恢复错误','count','counter','hardware','NVLink恢复错误计数','gpu','0','>10','',1,NOW(),NOW()),
('DCGM_FI_DEV_FABRIC_HEALTH_MASK','Fabric 健康掩码','bitmask','gauge','hardware','Fabric健康位掩码','gpu','0','!=0','非0表示异常',1,NOW(),NOW()),
-- stability 运行稳定性
('DCGM_FI_DEV_XID_ERRORS','XID 错误码','code','gauge','stability','最近XID错误码','gpu','0','致命码','见XID扣分表',1,NOW(),NOW()),
('DCGM_FI_DEV_CLOCKS_EVENT_REASONS','时钟事件原因','bitmask','gauge','stability','时钟事件位掩码','gpu','0','!=0','非0有throttle',1,NOW(),NOW()),
('DCGM_FI_DEV_GPU_RESET_COUNT','GPU Reset 次数','count','counter','stability','GPU重置次数','gpu','0','>=1','24h窗口',1,NOW(),NOW());

-- ---------------------------------------------------------------------------
-- 2. 默认评分策略
-- ---------------------------------------------------------------------------
INSERT INTO scoring_strategy (code, name, description, dimension_weights, is_default, version, created_at, updated_at)
VALUES ('default','默认策略(训练严格)','大模型训练场景，对硬件健康极度敏感',
  '{"hardware":0.45,"stability":0.25,"performance":0.20,"environment":0.10}', 1, 1, NOW(), NOW());

SET @sid = LAST_INSERT_ID();

-- ---------------------------------------------------------------------------
-- 3. 默认策略的指标规则（权重 + 曲线 + 一票否决）
-- ---------------------------------------------------------------------------
INSERT INTO strategy_metric_rule (strategy_id, metric_key, weight, curve_type, curve_params, is_veto, veto_threshold) VALUES
-- environment
(@sid,'DCGM_FI_DEV_GPU_TEMP',3.0,'piecewise','{"points":[[0,100],[80,100],[85,90],[90,70],[95,30],[100,0]]}',0,0),
(@sid,'DCGM_FI_DEV_MEMORY_TEMP',2.0,'piecewise','{"points":[[0,100],[85,100],[95,80],[100,40],[105,0]]}',0,0),
(@sid,'DCGM_FI_DEV_POWER_USAGE',1.0,'piecewise','{"points":[[0,100],[500,100],[600,90],[700,70]]}',0,0),
(@sid,'DCGM_FI_DEV_THERMAL_VIOLATION',1.0,'log','{"threshold":[1,300]}',0,0),
-- performance（利用率类用 none 不扣分；时钟/显存用曲线）
(@sid,'DCGM_FI_PROF_GR_ENGINE_ACTIVE',2.0,'none',NULL,0,0),
(@sid,'DCGM_FI_PROF_SM_ACTIVE',1.0,'none',NULL,0,0),
(@sid,'DCGM_FI_PROF_PIPE_TENSOR_ACTIVE',1.5,'none',NULL,0,0),
(@sid,'DCGM_FI_PROF_DRAM_ACTIVE',1.0,'none',NULL,0,0),
(@sid,'DCGM_FI_DEV_SM_CLOCK',1.5,'piecewise','{"points":[[0,0],[1000,40],[1500,70],[1800,100]]}',0,0),
(@sid,'DCGM_FI_DEV_FB_USED_PERCENT',1.0,'piecewise','{"points":[[0,100],[0.9,100],[0.95,80],[0.98,50],[1.0,0]]}',0,0),
-- hardware
(@sid,'DCGM_FI_DEV_ECC_SBE_VOL_TOTAL',2.0,'log','{"threshold":[10,500]}',0,0),
(@sid,'DCGM_FI_DEV_ECC_DBE_VOL_TOTAL',5.0,'veto',NULL,1,1),
(@sid,'DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS',2.0,'log','{"threshold":[1,8]}',0,0),
(@sid,'DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS',5.0,'veto',NULL,1,1),
(@sid,'DCGM_FI_DEV_ROW_REMAP_FAILURE',5.0,'veto',NULL,1,1),
(@sid,'DCGM_FI_DEV_PCIE_REPLAY_COUNTER',2.0,'log','{"threshold":[50,500]}',0,0),
(@sid,'DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL',2.0,'log','{"threshold":[1,100]}',0,0),
(@sid,'DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL',3.0,'log','{"threshold":[1,10]}',0,0),
(@sid,'DCGM_FI_DEV_FABRIC_HEALTH_MASK',2.0,'piecewise','{"points":[[0,100],[1,30]]}',0,0),
-- stability
(@sid,'DCGM_FI_DEV_XID_ERRORS',5.0,'xid_table',NULL,1,1),
(@sid,'DCGM_FI_DEV_CLOCKS_EVENT_REASONS',2.0,'piecewise','{"points":[[0,100],[1,60]]}',0,0),
(@sid,'DCGM_FI_DEV_GPU_RESET_COUNT',3.0,'piecewise','{"points":[[0,100],[1,60],[3,20],[5,0]]}',0,0);

-- ---------------------------------------------------------------------------
-- 4. 故障知识图谱种子（几条示例）
-- ---------------------------------------------------------------------------
INSERT INTO fault_knowledge (fault_type, x_id_code, symptom, possible_cause, metric_changes, related_metrics, severity, suggestion, reference, created_at, updated_at) VALUES
('显存双比特错误','48','应用报uncorrectable ECC error，任务崩溃','HBM存储单元物理退化，宇宙射线翻转','ECC DBE计数从0变为>=1','["DCGM_FI_DEV_ECC_DBE_VOL_TOTAL"]','fatal','排空任务并重置GPU；持续出现需RMA','https://docs.nvidia.com/deploy/xid-errors/',NOW(),NOW()),
('GPU掉总线','79','nvidia-smi无法识别该卡，GPU has fallen off the bus','PCIe链路硬件故障或供电问题','XID=79，PCIe重传可能先升高','["DCGM_FI_DEV_XID_ERRORS","DCGM_FI_DEV_PCIE_REPLAY_COUNTER"]','fatal','冷重启；重新插拔；持续则RMA','https://docs.nvidia.com/deploy/xid-errors/',NOW(),NOW()),
('散热异常','','分数骤降+温度过高+降频事件','散热失效、风扇故障、机架气流受阻','温度>90C，SM时钟下降，时钟事件原因非0','["DCGM_FI_DEV_GPU_TEMP","DCGM_FI_DEV_SM_CLOCK","DCGM_FI_DEV_CLOCKS_EVENT_REASONS"]','warning','检查风扇与机房空调，清理滤网','',NOW(),NOW()),
('NVLink互连故障','74','NCCL通信hang或带宽下降','NVLink物理故障或远端设备失效','NVLink CRC错误上升，Fabric掩码非0','["DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL","DCGM_FI_DEV_FABRIC_HEALTH_MASK"]','critical','重新插拔NVLink；检查NVSwitch','https://docs.nvidia.com/deploy/xid-errors/',NOW(),NOW());
