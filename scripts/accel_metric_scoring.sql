-- ============================================================
-- accel_metric_scoring — 健康度指标规则表（1:1 对应 Excel）
-- 数据来源：scripts/⭐️健康度评估使用指标 (2).xlsx（新版，24 列；一票否决已补全）
--
-- 设计目标：
--   把 Excel 的列拆成规范化的独立列，评分时直接用列值匹配指标范围，
--   与旧的 accel_metrics / accel_metric_rules（JSON 存规则）独立并存。
--
-- 列 → Excel 表头映射：
--   seq_no              ← 序号
--   official_no         ← 官方编号
--   card_type           ← 卡的类型（GPU / NPU）
--   owner_subject       ← 归属主体（如 A单卡自身）
--   owner_subject_code  ← 归属主体映射码（见 字段映射.pdf）【新增】
--   metric_name         ← 字段名称
--   value_type          ← 指标数值类型
--   value_type     ← 指标数值类型映射码【新增】
--   health_purpose      ← 健康度用途（核心/归因/配置合规/性能容量）
--   health_purpose ← 健康度用途映射码【新增】
--   concept             ← 指标概念
--   dimension           ← 指标维度（暂不做数字映射）
--   unit                ← 单位
--   work_range          ← 工作范围
--   warn_upbound         ← 告警上界（DOUBLE）【新增】
--   upper_bond         ← 指标上界（DOUBLE）
--   lower_bound         ← 指标下界（DOUBLE）
--   warn_lowbound         ← 告警下界（DOUBLE）【新增】
--   normal_rate         ← 正常速率
--   warn_rate          ← 告警速率【新增】
--   normal_rate_unit           ← 速率单位（直接取 Excel 列，原 normal_rate_unit 改名）
--   bool_normal         ← 布尔正常
--   bool_abnormal       ← 布尔异常
--   enum_result         ← 枚举结果
--   is_veto             ← 关键判决（一票否决），1=是 0=否【新增】
--   derate_threshold    ← 降频/关机阈值
--   source_ref          ← 来源依据
--
-- 字段映射（见 字段映射.pdf）：
--   owner_subject_code:  A单卡自身=1 B链路（Host-Device&P2P）=2 C共享基础设施=3
--                        D环境与配置=4 E跨节点网络=5 空=6
--   value_type:     Gauge连续数值=1 Gauge_Rate比率=2 Counter累计计数=3
--                        Counter_Duration累计时长=4 Level_Count水位计数=5 Bool布尔=6
--                        Ordinal枚举=7 Other其他=8
--   health_purpose: 核心=1 归因=2 配置合规=3 性能容量=4
--   is_veto:             是=1 否=0
--   dimension:           暂不做数字映射（PDF 说明后续再定）
--
-- 执行方式（务必用 SET NAMES + SOURCE，否则中文乱码）：
--   docker cp "scripts/accel_metric_scoring.sql" mysql-standalone:/tmp/
--   docker exec mysql-standalone mysql -uroot -proot123 \
--     -e "SET NAMES utf8mb4; SOURCE /tmp/accel_metric_scoring.sql;" gpu_health
-- ============================================================

SET NAMES utf8mb4;

DROP TABLE IF EXISTS `metric_definition`;

CREATE TABLE `metric_definition` (
    `id`                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '技术主键',
    `seq_no`              INT             NULL COMMENT '序号（Excel 内序号）',
    `official_num`         VARCHAR(191)    NULL COMMENT '官方编号，如 DCGM_FI_DEV_GPU_TEMP (150)',
    `card_type`           VARCHAR(64)     NULL COMMENT '卡的类型：GPU / NPU',
    `owner_subject`  TINYINT         NULL COMMENT '归属主体映射码：1=A单卡自身 2=B链路 3=C共享基础设施 4=D环境与配置 5=E跨节点网络 6=空',
    `metric_name`         VARCHAR(191)    NOT NULL COMMENT '字段名称（指标唯一标识）',
    `value_type`     TINYINT         NULL COMMENT '指标数值类型映射码：1~8（见字段映射.pdf）',
    `health_purpose` TINYINT         NULL COMMENT '健康度用途映射码：1=核心 2=归因 3=配置合规 4=性能容量',
    `concept`             TEXT            NULL COMMENT '指标概念',
    `dimension`           VARCHAR(64)     NULL COMMENT '指标维度，如 thermal温度散热',
    `unit`                VARCHAR(32)     NULL COMMENT '单位',
    `work_range`          VARCHAR(128)    NULL COMMENT '工作范围，如 0~85',
    `warn_upbound`         DOUBLE          NULL COMMENT '告警上界（数值；非数值置 NULL）',
    `upper_bond`         DOUBLE          NULL COMMENT '指标上界（数值；非数值置 NULL）',
    `lower_bound`         DOUBLE          NULL COMMENT '指标下界（数值；非数值置 NULL）',
    `warn_lowbound`         DOUBLE          NULL COMMENT '告警下界（数值；非数值置 NULL）',
    `normal_rate`         DOUBLE          NULL COMMENT '正常速率（数值；非数值置 NULL）',
    `warn_rate`          DOUBLE          NULL COMMENT '告警速率（数值；非数值置 NULL）',
    `normal_rate_unit`           VARCHAR(32)     NULL COMMENT '速率单位（直接取 Excel 速率单位列，如 μs/s、次/天）',
    `bool_normal`         TEXT            NULL COMMENT '布尔正常（布尔正常值）',
    `bool_abnormal`       TEXT            NULL COMMENT '布尔异常（布尔异常值）',
    `enum_result`         TEXT            NULL COMMENT '枚举结果（枚举类指标的取值含义）',
    `is_veto`             TINYINT         NULL COMMENT '关键判决（一票否决）：1=是 0=否',
    `derate_threshold`    TEXT            NULL COMMENT '降频/关机阈值（关键分档阈值）',
    `source_ref`          TEXT            NULL COMMENT '来源依据（出处）',
    `vender`              VARCHAR(32)     NULL COMMENT '厂商：NVIDIA / 华为昇腾',
    `created_at`          TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at`          TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_metric_name` (`metric_name`),
    KEY `idx_vender` (`vender`),
    KEY `idx_dimension` (`dimension`),
    KEY `idx_owner_subject` (`owner_subject`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='健康度指标规则表';

-- 数据行由 scripts/gen_scoring_seed.py 从 Excel 自动生成，见下方 INSERT。

INSERT INTO `metric_definition`
(`seq_no`,`official_num`,`card_type`,`owner_subject`,`metric_name`,`value_type`,`health_purpose`,`concept`,`dimension`,`unit`,`work_range`,`warn_upbound`,`upper_bond`,`lower_bound`,`warn_lowbound`,`normal_rate`,`warn_rate`,`normal_rate_unit`,`bool_normal`,`bool_abnormal`,`enum_result`,`is_veto`,`derate_threshold`,`source_ref`,`vender`) VALUES
(1,'DCGM_FI_DEV_GPU_TEMP (150)','GPU',1,'DCGM_FI_DEV_GPU_TEMP',1,1,'GPU 核心 (die) 当前温度，温度类主指标','thermal温度散热','℃','20~85',92,85,20,0,NULL,NULL,NULL,NULL,NULL,NULL,1,'降频(软件)=85；降频(硬件)=89；保护关机=92','【实测值】A100-SXM4 / H100-SXM5 nvidia-smi -q -d TEMPERATURE。H100 的 Max Op 为 87，取 85 作统一保守基线','NVIDIA'),
(2,'DCGM_FI_DEV_MEMORY_TEMP (140)','GPU',1,'DCGM_FI_DEV_MEMORY_TEMP',1,1,'HBM 显存颗粒温度','thermal温度散热','℃','20~95',NULL,95,20,0,NULL,NULL,NULL,NULL,NULL,NULL,1,'降频(软件热)=95，显存温度超此值触发 SW_THERMAL (0x20)','【实测值】A100/H100 HBM2e Memory Max Operating Temp = 95℃','NVIDIA'),
(3,'DCGM_FI_DEV_GPU_MAX_OP_TEMP (152)','GPU',1,'DCGM_FI_DEV_GPU_MAX_OP_TEMP',1,2,'GPU 最高工作温度，软件降频触发门限（设备常量）','thermal温度散热','℃','85~85',85,85,85,85,NULL,NULL,NULL,NULL,NULL,NULL,0,'★软件降频起始点 = 85','【实测值】A100-SXM4 = 85。程序优先读本字段实时值，读取失败回退 85','NVIDIA'),
(4,'DCGM_FI_DEV_SLOWDOWN_TEMP (158)','GPU',1,'DCGM_FI_DEV_SLOWDOWN_TEMP',1,2,'硬件降频温度门限，触发后核心时钟降至 1/2 或更低','thermal温度散热','℃','89~89',89,89,89,89,NULL,NULL,NULL,NULL,NULL,NULL,0,'★硬件降频门限 = 89','【实测值】A100-SXM4 / H100-SXM5 均为 89。读取失败回退 89','NVIDIA'),
(5,'DCGM_FI_DEV_SHUTDOWN_TEMP (159)','GPU',1,'DCGM_FI_DEV_SHUTDOWN_TEMP',1,2,'硬件保护关机温度门限，达到后 GPU 断电避免物理损伤','thermal温度散热','℃','92~92',92,92,92,92,NULL,NULL,NULL,NULL,NULL,NULL,1,'★保护关机门限 = 92','【实测值】A100-SXM4 / H100-SXM5 均为 92。读取失败回退 92','NVIDIA'),
(6,'DCGM_FI_DEV_MEM_MAX_OP_TEMP (151)','GPU',1,'DCGM_FI_DEV_MEM_MAX_OP_TEMP',1,2,'显存最高工作温度门限','thermal温度散热','℃','95~95',95,95,95,95,NULL,NULL,NULL,NULL,NULL,NULL,0,'★显存侧软件热降频门限 = 95','【实测值】A100/H100 HBM2e = 95。读取失败回退 95','NVIDIA'),
(7,'DCGM_FI_DEV_THERMAL_VIOLATION (241)','GPU',1,'DCGM_FI_DEV_THERMAL_VIOLATION',4,1,'因温度触发降频的累计时长（自驱动加载起单调递增）','thermal温度散热','μs','0',NULL,NULL,0,NULL,0,0,NULL,NULL,NULL,NULL,0,'单位：μs/s，采样窗口增量 >0 即判定发生热降频。本字段即“已发生热降频”的直接证据，无需另设阈值','【官方规格】DCGM Field IDs','NVIDIA'),
(8,'DCGM_FI_DEV_FAN_SPEED (191)','GPU',1,'DCGM_FI_DEV_FAN_SPEED',2,2,'风扇转速占最大转速百分比（SXM 模组由整机散热，通常不上报）','thermal温度散热','%','0~90',95,90,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'持续 >90% 超过 10 分钟判定散热能力见底
转速 100% 且温度仍 >85 → 散热系统已失效','【行业经验】90% 为持续运行告警线','NVIDIA'),
(9,'DCGM_FI_DEV_NVSWITCH_TEMPERATURE_CURRENT (858)','GPU',3,'DCGM_FI_DEV_NVSWITCH_TEMPERATURE_CURRENT',1,1,'NVSwitch 芯片当前温度','thermal温度散热','℃','20~90',100,90,20,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'降频=90；关机=100','【行业经验】NVIDIA 未公开 NVSwitch 门限，按 DGX 整机运维经验取 90/100，程序优先读字段 859/860','NVIDIA'),
(10,'DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SLOWDOWN (859)','GPU',3,'DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SLOWDOWN',1,2,'NVSwitch 降频温度门限','thermal温度散热','℃','90~90',90,90,90,90,NULL,NULL,NULL,NULL,NULL,NULL,0,'★NVSwitch 降频门限 = 90','【行业经验】读取失败回退 90','NVIDIA'),
(11,'DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SHUTDOWN (860)','GPU',3,'DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SHUTDOWN',1,2,'NVSwitch 关机温度门限','thermal温度散热','℃','100~100',100,100,100,100,NULL,NULL,NULL,NULL,NULL,NULL,0,'★NVSwitch 关机门限 = 100','【行业经验】读取失败回退 100','NVIDIA'),
(12,'DCGM_FI_DEV_POWER_USAGE (155)','GPU',1,'DCGM_FI_DEV_POWER_USAGE',1,2,'整卡实时功耗','power功耗电源','W','20~400',400,400,20,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'★功耗达到 400 触发软件功率封顶 SW_POWER_CAP (0x4)。H100-SXM5 对应值为 700','【官方规格】A100-SXM4-80GB TDP=400W；H100-SXM5=700W；A100-PCIe=300W。空闲 50~80W 为实测值','NVIDIA'),
(13,'DCGM_FI_DEV_ENFORCED_POWER_LIMIT (164)','GPU',1,'DCGM_FI_DEV_ENFORCED_POWER_LIMIT',1,2,'驱动综合所有限制因素后实际强制执行的功率上限','power功耗电源','W','400~400',400,400,400,400,NULL,NULL,NULL,NULL,NULL,NULL,0,'★功率封顶点，实时功耗触及此值即降频','【实测值】A100-SXM4-80GB = 400。读取失败回退 400','NVIDIA'),
(14,'DCGM_FI_DEV_POWER_MGMT_LIMIT (160)','GPU',1,'DCGM_FI_DEV_POWER_MGMT_LIMIT',1,3,'当前生效的功率管理上限（可由运维用 nvidia-smi-pl修改）','power功耗电源','W','100~400',400,400,100,100,NULL,NULL,NULL,NULL,NULL,NULL,0,'下调到 <400 会提前触发功率降频，应单独识别为配置类异常而非硬件故障','【实测值】A100-SXM4-80GB 可设区间 100~400','NVIDIA'),
(15,'DCGM_FI_DEV_POWER_MGMT_LIMIT_MIN (161)','GPU',1,'DCGM_FI_DEV_POWER_MGMT_LIMIT_MIN',1,3,'功率管理上限可设置的最小值','power功耗电源','W','100~100',100,100,100,100,NULL,NULL,NULL,NULL,NULL,NULL,0,NULL,'【实测值】A100-SXM4-80GB = 100W','NVIDIA'),
(16,'DCGM_FI_DEV_POWER_MGMT_LIMIT_MAX (162)','GPU',1,'DCGM_FI_DEV_POWER_MGMT_LIMIT_MAX',1,3,'功率管理上限可设置的最大值','power功耗电源','W','400~400',400,400,400,400,NULL,NULL,NULL,NULL,NULL,NULL,0,NULL,'【实测值】A100-SXM4-80GB = 400W；H100-SXM5 = 700W','NVIDIA'),
(17,'DCGM_FI_DEV_POWER_MGMT_LIMIT_DEF (163)','GPU',1,'DCGM_FI_DEV_POWER_MGMT_LIMIT_DEF',1,3,'出厂默认功率管理上限（TDP 基准）','power功耗电源','W','400~400',400,400,400,400,NULL,NULL,NULL,NULL,NULL,NULL,0,'用于判断当前功率上限是否被人为下调','【实测值】A100-SXM4-80GB = 400W','NVIDIA'),
(18,'DCGM_FI_DEV_POWER_VIOLATION (240)','GPU',1,'DCGM_FI_DEV_POWER_VIOLATION',4,1,'因功耗超限触发降频的累计时长','power功耗电源','μs','0',NULL,NULL,0,NULL,0,0,NULL,NULL,NULL,NULL,0,'单位μs/s，采样窗口增量 >0 即判定发生功率降频。本字段即“已发生功率降频”的直接证据','【官方规格】DCGM Field IDs','NVIDIA'),
(19,'DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION (156)','GPU',1,'DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION',3,4,'自驱动最近一次加载以来的累计能耗（重装/重载驱动后归零）','power功耗电源','mJ','单调递增',NULL,400000,0,NULL,400000,400000,NULL,NULL,NULL,NULL,0,'速率上界 400000 mJ/s（=400W）；速率应等于字段155×1000，偏差 >10% 说明采样异常','【官方规格】DCGM Field IDs；上界由 TDP 400W 换算','NVIDIA'),
(20,'DCGM_FI_DEV_CLOCK_THROTTLE_REASONS (112)','GPU',1,'DCGM_FI_DEV_CLOCK_THROTTLE_REASONS',7,1,'当前降频原因位掩码，判定“是否降频+为何降频”的唯一权威字段','power功耗电源','bitmask','0x0 / 0x1 / 0x2 属正常',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'0x0 无降频；0x1 GPU_IDLE 空闲降频；0x2 CLOCKS_SETTING 人为设定应用时钟；0x4 SW_POWER_CAP 软件功率封顶；0x8 HW_SLOWDOWN 硬件降频；0x10 SYNC_BOOST；0x20 SW_THERMAL 软件热降频；0x40 HW_THERMAL 硬件热降频；0x80 HW_POWER_BRAKE 外部电源刹车；0x100 DISPLAY_CLOCKS',0,'★判定式：(value & 0xFC) != 0 即存在异常降频。0x8/0x40/0x80 高危，0x4/0x20 中危','【官方规格】DCGM Field Constants (dcgm_fields.h)','NVIDIA'),
(21,'DCGM_FI_DEV_PSTATE (190)','GPU',1,'DCGM_FI_DEV_PSTATE',7,2,'性能状态 P-State，0 为最高性能档','power功耗电源','档位','0~0（有负载时）',NULL,15,0,NULL,NULL,NULL,NULL,NULL,NULL,'0 (P0)；有持续负载但值 >0',0,'空闲时 P-State 升高属节能行为，判定前先确认 GPU_UTIL >10%','【官方规格】DCGM Field IDs','NVIDIA'),
(22,'DCGM_FI_DEV_BOARD_LIMIT_VIOLATION (243)','GPU',1,'DCGM_FI_DEV_BOARD_LIMIT_VIOLATION',4,1,'因板级供电/电流限制触发降频的累计时长','power功耗电源','μs','0',NULL,NULL,0,NULL,0,0,NULL,NULL,NULL,NULL,1,'增量 >0 表示板级供电不足导致降频，需排查整机电源','【官方规格】DCGM Field IDs','NVIDIA'),
(23,'DCGM_FI_DEV_FB_TOTAL (250)','GPU',1,'DCGM_FI_DEV_FB_TOTAL',1,1,'帧缓存(显存)总量','memory显存可靠性','MB','81920~81920',81920,81920,81920,81920,NULL,NULL,NULL,NULL,NULL,NULL,0,'低于标称说明显存颗粒被屏蔽，属永久性降级。A100-40GB=40960，H100-80GB=81559','【实测值】A100-SXM4-80GB = 81920 MB','NVIDIA'),
(24,'DCGM_FI_DEV_FB_USED (252)','GPU',1,'DCGM_FI_DEV_FB_USED',1,4,'已使用显存','memory显存可靠性','MB','0~77824',81920,77824,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'77824 = 总量 95%，作为显存耗尽告警线','【行业经验】95% 为通行告警水位','NVIDIA'),
(25,'DCGM_FI_DEV_FB_FREE (251)','GPU',1,'DCGM_FI_DEV_FB_FREE',1,4,'空闲显存','memory显存可靠性','MB','4096~81920',81920,81920,4096,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'低于 4096 MB 判定显存紧张','【行业经验】4 GB 余量为通行告警线','NVIDIA'),
(26,'DCGM_FI_DEV_FB_RESERVED (253)','GPU',1,'DCGM_FI_DEV_FB_RESERVED',1,1,'驱动保留显存（含 ECC 开销、退役页与重映射行占用）','memory显存可靠性','MB','0~2048',NULL,2048,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'A100-80GB 开 ECC 时正常保留 800~1200 MB，超过 2048 说明退役页/重映射行显著增加','【实测值】正常保留 800~1200 MB；【行业经验】2048 为告警线','NVIDIA'),
(27,'DCGM_FI_DEV_FB_USED_PERCENT (254)','GPU',1,'DCGM_FI_DEV_FB_USED_PERCENT',2,4,'显存使用率 = Used/(Total-Reserved)','memory显存可靠性','比例(0~1)','0~0.95',1,0.95,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'0.95 为告警线','【官方规格】取值域 0.0~1.0；【行业经验】0.95 告警','NVIDIA'),
(28,'DCGM_FI_DEV_ECC_CURRENT (300)','GPU',1,'DCGM_FI_DEV_ECC_CURRENT',6,3,'当前生效的 ECC 模式','memory显存可靠性',NULL,'恒为 1',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1','0',NULL,0,'1 = Enabled；0 = Disabled（失去纠错能力，生产环境禁止）','【官方规格】DCGM Field IDs','NVIDIA'),
(29,'DCGM_FI_DEV_ECC_PENDING (301)','GPU',1,'DCGM_FI_DEV_ECC_PENDING',6,3,'下次重启后生效的 ECC 模式','memory显存可靠性',NULL,'恒为 1',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1','0',NULL,0,'1，且等于字段300；!= 字段300（存在待重启生效的 ECC 模式变更）','【官方规格】DCGM Field IDs','NVIDIA'),
(30,'DCGM_FI_DEV_ECC_SBE_VOL_TOTAL (310)','GPU',1,'DCGM_FI_DEV_ECC_SBE_VOL_TOTAL',5,1,'易失(volatile)单比特可纠正 ECC 错误总数，驱动重载/GPU 复位后清零','memory显存可靠性','次','0~24',NULL,24,0,NULL,24,24,'次/天',NULL,NULL,NULL,0,'应≤1次/小时；>24 次/天 判定 HBM 开始退化；>100 次/天 判定需停机检修','【行业经验】1 次/小时为关注线，24 次/天为告警线','NVIDIA'),
(31,'DCGM_FI_DEV_ECC_DBE_VOL_TOTAL (311)','GPU',1,'DCGM_FI_DEV_ECC_DBE_VOL_TOTAL',5,1,'易失双比特不可纠正 ECC 错误总数，通常直接导致任务崩溃','memory显存可靠性','次','0~0',NULL,0,0,NULL,0,0,'次/天','0','>0（任意一次即严重故障，立即隔离该卡）',NULL,1,'次/天；触发 DCGM_FR_DBE_VIOLATION','【官方规格】DCGM dcgm_errors.h','NVIDIA'),
(32,'DCGM_FI_DEV_ECC_SBE_AGG_TOTAL (312)','GPU',1,'DCGM_FI_DEV_ECC_SBE_AGG_TOTAL',3,1,'持久化(aggregate)单比特 ECC 错误总数，跨重启保留','memory显存可靠性','次','0~1000',NULL,1000,0,NULL,0,24,'次',NULL,NULL,NULL,0,'≤24 次/天；累计 >1000 次建议安排 RMA 评估','【行业经验】1000 次累计为业内常用 RMA 评估参考线','NVIDIA'),
(33,'DCGM_FI_DEV_ECC_DBE_AGG_TOTAL (313)','GPU',1,'DCGM_FI_DEV_ECC_DBE_AGG_TOTAL',3,1,'持久化双比特 ECC 错误总数，跨重启保留','memory显存可靠性','次','0~0',NULL,0,0,NULL,0,0,'次',NULL,NULL,NULL,1,'次/天，>0（历史存在不可纠正错误，RMA 候选）','【官方规格】DCGM Field IDs；【行业经验】任意 >0 即 RMA 候选','NVIDIA'),
(34,'DCGM_FI_DEV_ECC_DBE_VOL_DEV (319)','GPU',1,'DCGM_FI_DEV_ECC_DBE_VOL_DEV',5,1,'显存颗粒(device memory)双比特 ECC 错误，用于定位错误发生位置','memory显存可靠性','次','0~0',NULL,0,0,NULL,0,0,'次/天','0','>0',NULL,1,'页/天','【官方规格】DCGM Field IDs','NVIDIA'),
(35,'DCGM_FI_DEV_RETIRED_SBE (390)','GPU',1,'DCGM_FI_DEV_RETIRED_SBE',3,1,'因单比特错误而退役的显存页数（Volta 及更早架构机制）','memory显存可靠性','页','0~60',NULL,60,0,NULL,0,0,'页',NULL,NULL,NULL,0,'页/天，新增即需关注
SBE+DBE 退役页合计达 64 页（页黑名单上限）即需 RMA，60 为预警线','【行业经验】64 页为 NVIDIA 页黑名单容量，业内通行 RMA 判据','NVIDIA'),
(36,'DCGM_FI_DEV_RETIRED_DBE (391)','GPU',1,'DCGM_FI_DEV_RETIRED_DBE',3,1,'因双比特错误而退役的显存页数','memory显存可靠性','页','0~0',NULL,64,0,NULL,0,0,'页',NULL,NULL,NULL,1,'页/天，与 SBE 退役页合计上限 64 页，>0异常','【行业经验】同上','NVIDIA'),
(37,'DCGM_FI_DEV_RETIRED_PENDING (392)','GPU',1,'DCGM_FI_DEV_RETIRED_PENDING',5,1,'待退役页数，需 GPU 复位后才实际生效','memory显存可靠性','页','0~0',NULL,0,0,NULL,0,0,'页/天','0','>0（存在待处理坏页，需安排 GPU reset）',NULL,1,'页/天','【官方规格】DCGM Field IDs / DCGM Diagnostics','NVIDIA'),
(38,'DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS (393)','GPU',1,'DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS',3,1,'因不可纠正错误发生行重映射的行数（Ampere 及以后架构）','memory显存可靠性','行','0~0',NULL,0,0,NULL,0,0,'行',NULL,NULL,NULL,1,'行/天，>0（HBM 存在硬缺陷，RMA 候选）','【行业经验】不可纠正重映射任意 >0 即 RMA 判据','NVIDIA'),
(39,'DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS (394)','GPU',1,'DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS',3,1,'因可纠正错误发生行重映射的行数','memory显存可靠性','行','0~8',NULL,8,0,NULL,0,8,'行',NULL,NULL,NULL,0,'行/天；累计 >8 行进入关注，>60 行建议 RMA每 bank 备用行有限，60 行为预警线','【行业经验】8 行关注 / 60 行 RMA 为业内通行梯度','NVIDIA'),
(40,'DCGM_FI_DEV_ROW_REMAP_FAILURE (395)','GPU',1,'DCGM_FI_DEV_ROW_REMAP_FAILURE',6,1,'行重映射是否失败（备用行耗尽）','memory显存可靠性',NULL,'恒为 0',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'0','1',NULL,1,'0 = 未失败；1 = 重映射失败，显存已无冗余，立即隔离并 RMA','【官方规格】DCGM Field IDs','NVIDIA'),
(41,'DCGM_FI_DEV_ROW_REMAP_PENDING (396)','GPU',1,'DCGM_FI_DEV_ROW_REMAP_PENDING',6,1,'是否存在待生效的行重映射（需 GPU 复位）','memory显存可靠性',NULL,'恒为 0',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'0','1',NULL,0,'0 = 无待处理项；1 = 有待生效重映射，需 GPU reset 后方恢复满配可靠性','【官方规格】DCGM Field IDs','NVIDIA'),
(42,'DCGM_FI_DEV_BAR1_TOTAL (90)','GPU',1,'DCGM_FI_DEV_BAR1_TOTAL',1,2,'BAR1 地址空间总量','memory显存可靠性','MB','65536~65536',65536,65536,65536,65536,NULL,NULL,NULL,NULL,NULL,NULL,0,NULL,'【实测值】A100-SXM4-80GB = 65536 MB','NVIDIA'),
(43,'DCGM_FI_DEV_BAR1_USED (92)','GPU',1,'DCGM_FI_DEV_BAR1_USED',1,2,'BAR1 已用量，耗尽会导致 P2P / GPUDirect RDMA 失败','memory显存可靠性','MB','0~58982',65536,58982,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'58982 = 总量 90%，超过即预警 P2P 可能失败','【行业经验】90% 为通行告警水位','NVIDIA'),
(44,'DCGM_FI_DEV_MEM_COPY_UTIL (204)','GPU',1,'DCGM_FI_DEV_MEM_COPY_UTIL',2,4,'显存读写(拷贝)通道时间占比','memory显存可靠性','%','0~100',100,90,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'持续 >90% 且 SM_ACTIVE <0.3 说明访存瓶颈（性能问题，非硬件故障）','【官方规格】DCGM Field IDs','NVIDIA'),
(45,'DCGM_FI_DEV_PCIE_REPLAY_COUNTER (202)','GPU',2,'DCGM_FI_DEV_PCIE_REPLAY_COUNTER',3,1,'PCIe 链路层重传次数，反映链路信号质量','pcie总线','次','长期不增长',NULL,NULL,0,NULL,0,8,'次/分钟',NULL,NULL,NULL,0,'★≤8 次/分钟；>8 次/分钟 触发 DCGM 健康检查 Warning；>100 次/分钟 判定链路严重劣化','【官方规格】NVIDIA DCGM User Guide：Detected more than 8 PCIe replays per minute','NVIDIA'),
(46,'DCGM_FI_DEV_PCIE_MAX_LINK_GEN (235)','GPU',2,'DCGM_FI_DEV_PCIE_MAX_LINK_GEN',7,2,'PCIe 最大支持代数（设备能力）','pcie总线','代','4~4',NULL,4,4,NULL,NULL,NULL,NULL,NULL,NULL,NULL,0,'A100 = Gen4；H100 = Gen5（值为 5）','【官方规格】A100 支持 PCIe 4.0','NVIDIA'),
(47,'DCGM_FI_DEV_PCIE_LINK_GEN (237)','GPU',2,'DCGM_FI_DEV_PCIE_LINK_GEN',7,1,'PCIe 当前协商代数','pcie总线','代','4~4',NULL,4,4,NULL,NULL,NULL,NULL,NULL,NULL,'4（等于字段235）；< 4（链路降代，带宽按比例损失）',0,'Gen4→Gen3 带宽腰斩，Gen4→Gen1 仅剩 1/8','【官方规格】判据为 字段237 == 字段235','NVIDIA'),
(48,'DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH (236)','GPU',2,'DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH',7,2,'PCIe 最大链路宽度（设备能力）','pcie总线','lane','16~16',NULL,16,16,NULL,NULL,NULL,NULL,NULL,NULL,'数据中心训练卡均为 x16',0,NULL,'【官方规格】数据中心训练卡均为 x16','NVIDIA'),
(49,'DCGM_FI_DEV_PCIE_LINK_WIDTH (238)','GPU',2,'DCGM_FI_DEV_PCIE_LINK_WIDTH',7,1,'PCIe 当前协商链路宽度','pcie总线','lane','16~16',NULL,16,16,NULL,NULL,NULL,NULL,NULL,NULL,'16（等于字段236）、< 16（降宽至 x8/x4/x1）',0,'降宽通常源于插槽接触不良、金手指氧化或 BIOS 配置错误','【官方规格】判据为 字段238 == 字段236','NVIDIA'),
(50,'DCGM_FI_PROF_PCIE_TX_BYTES (1009)','GPU',2,'DCGM_FI_PROF_PCIE_TX_BYTES',3,4,'PCIe 发送字节数（含包头与载荷，GPU→Host 方向）','pcie总线','Byte','0~31500000000',NULL,31500000000,0,NULL,31.5,20,'GB/s',NULL,NULL,NULL,0,'速率上界 31.5 GB/s（Gen4 x16 单向理论值），实测有效带宽通常 24~26 GB/s。
满载时速率长期 <20 GB/s 需排查链路降级','【官方规格】PCIe 4.0 x16 = 16 GT/s × 16 lane × 128/130 ÷ 8 = 31.5 GB/s','NVIDIA'),
(51,'DCGM_FI_PROF_PCIE_RX_BYTES (1010)','GPU',2,'DCGM_FI_PROF_PCIE_RX_BYTES',3,4,'PCIe 接收字节数（Host→GPU 方向）','pcie总线','Byte','0~31500000000',NULL,31500000000,0,NULL,31.5,20,'GB/s',NULL,NULL,NULL,0,'速率上界 31.5 GB/s，实测有效 24~26 GB/s','【官方规格】同上','NVIDIA'),
(52,'DCGM_FI_DEV_PCIE_TX_THROUGHPUT (200)','GPU',2,'DCGM_FI_DEV_PCIE_TX_THROUGHPUT',8,4,'PCIe 发送吞吐（官方标注 Deprecated，新项目改用字段1009）','pcie总线','KB/s','0~30761718',NULL,30761718,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'30761718 KB/s = 31.5 GB/s','【官方规格】DCGM Field IDs 标注 Deprecated','NVIDIA'),
(53,'DCGM_FI_DEV_PCIE_RX_THROUGHPUT (201)','GPU',2,'DCGM_FI_DEV_PCIE_RX_THROUGHPUT',8,4,'PCIe 接收吞吐（官方标注 Deprecated，新项目改用字段1010）','pcie总线','KB/s','0~30761718',NULL,30761718,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,NULL,'【官方规格】DCGM Field IDs 标注 Deprecated','NVIDIA'),
(54,'DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL (409)','GPU',2,'DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL',3,1,'NVLink FLIT(流控单元) CRC 错误总数（全链路汇总）','nvlink片间互连（DCGM）','次','长期不增长',NULL,NULL,0,NULL,0,10,'次/分钟',NULL,NULL,NULL,0,'★<100 次/秒（DCGM 官方阈值）；工程建议收紧到 <10 次/分钟','【官方规格】DCGM dcgm_errors.h：DCGM_FR_NVLINK_CRC_ERROR_THRESHOLD = 100 次/秒','NVIDIA'),
(55,'DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL (419)','GPU',2,'DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL',3,1,'NVLink 数据 CRC 错误总数','nvlink片间互连（DCGM）','次','长期不增长',NULL,NULL,0,NULL,0,10,'次/分钟',NULL,NULL,NULL,0,'★<100 次/秒；工程建议 <10 次/分钟','【官方规格】DCGM dcgm_errors.h','NVIDIA'),
(56,'DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL (429)','GPU',2,'DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL',3,1,'NVLink 链路重传次数总数','nvlink片间互连（DCGM）','次','恒不增长',NULL,NULL,0,NULL,0,0,'次/分钟',NULL,NULL,NULL,0,'0 次/分钟；>10 次/分钟 判定链路劣化','【行业经验】10 次/分钟为通行告警线','NVIDIA'),
(57,'DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL (439)','GPU',2,'DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL',3,1,'NVLink 链路恢复(重训练)次数总数','nvlink片间互连（DCGM）','次','恒不增长',NULL,NULL,0,NULL,0,0,'次/小时',NULL,NULL,NULL,0,'次/小时；任意增量都表示链路曾中断重训，直接影响集合通信','【行业经验】任意 >0 即告警','NVIDIA'),
(58,'DCGM_FI_DEV_GPU_NVLINK_ERRORS (450)','GPU',2,'DCGM_FI_DEV_GPU_NVLINK_ERRORS',3,1,'GPU 侧 NVLink 错误汇总','nvlink片间互连（DCGM）','次','恒不增长',NULL,NULL,0,NULL,0,0,'次/小时',NULL,NULL,NULL,0,'次/小时','【官方规格】DCGM Field IDs','NVIDIA'),
(59,'DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL (449)','GPU',2,'DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL',3,4,'NVLink 全链路累计传输量','nvlink片间互连（DCGM）','KB','0~600000000',NULL,600000000,0,NULL,600,NULL,'GB/s',NULL,NULL,NULL,0,'速率上界 600 GB/s（A100 NVLink3 双向聚合）；H100 NVLink4 为 900 GB/s','【官方规格】A100 12 条 NVLink3 × 50 GB/s = 600 GB/s 双向','NVIDIA'),
(60,'DCGM_FI_PROF_NVLINK_TX_BYTES (1011)','GPU',2,'DCGM_FI_PROF_NVLINK_TX_BYTES',3,4,'NVLink 发送字节数（含包头与载荷）','nvlink片间互连（DCGM）','Byte','0~300000000000',NULL,300000000000,0,NULL,300,NULL,'GB/s',NULL,NULL,NULL,0,'速率上界 300 GB/s（单向）','【官方规格】A100 单向 300 GB/s；H100 单向 450 GB/s','NVIDIA'),
(61,'DCGM_FI_PROF_NVLINK_RX_BYTES (1012)','GPU',2,'DCGM_FI_PROF_NVLINK_RX_BYTES',3,4,'NVLink 接收字节数','nvlink片间互连（DCGM）','Byte','0~300000000000',NULL,300000000000,0,NULL,300,NULL,'GB/s',NULL,NULL,NULL,0,'速率上界 300 GB/s（单向）','【官方规格】同上','NVIDIA'),
(62,'DCGM_FI_DEV_NVSWITCH_LINK_STATUS (870)','GPU',3,'DCGM_FI_DEV_NVSWITCH_LINK_STATUS',7,1,'NVSwitch 上 NVLink 端口状态','nvlink片间互连（DCGM）',NULL,'恒为 2',NULL,3,-1,NULL,NULL,NULL,NULL,NULL,NULL,'2 = ACTIVE；-1 UNKNOWN；0 OFF ；1 SAFE ；3 ERROR',0,NULL,'【官方规格】DCGM Field IDs','NVIDIA'),
(63,'DCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS (782)','GPU',3,'DCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS',3,1,'NVSwitch 端口致命错误计数（端口 0-17）','nvlink片间互连（DCGM）','次','0~0',NULL,0,0,NULL,0,0,'次/小时',NULL,NULL,NULL,1,'次/小时，>0（触发 DCGM_FR_NVSWITCH_FATAL_ERROR，业务需立即迁移）','【官方规格】DCGM dcgm_errors.h','NVIDIA'),
(64,'DCGM_FI_DEV_NVSWITCH_LINK_NON_FATAL_ERRORS (783)','GPU',3,'DCGM_FI_DEV_NVSWITCH_LINK_NON_FATAL_ERRORS',3,1,'NVSwitch 端口非致命错误计数','nvlink片间互连（DCGM）','次','0~10',NULL,10,0,NULL,0,10,'次',NULL,NULL,NULL,0,'次/小时；累计 >10 进入关注，>10（可继续承载业务但需监控）','【行业经验】非致命错误 10 次为关注线','NVIDIA'),
(65,'DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS (856)','GPU',3,'DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS',7,1,'NVSwitch 致命错误事件，字段值即具体 SXid 码','nvlink片间互连（DCGM）','SXid 码','无上报',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'无 SXid 上报；上报任意致命 SXid（触发 DCGM_FR_SXID_ERROR）',1,NULL,'【官方规格】DCGM dcgm_errors.h','NVIDIA'),
(66,'DCGM_FI_DEV_NVSWITCH_RESET_REQUIRED (864)','GPU',3,'DCGM_FI_DEV_NVSWITCH_RESET_REQUIRED',6,1,'NVSwitch 是否需要复位','nvlink片间互连（DCGM）',NULL,'恒为 0',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'0','1',NULL,1,'0 = 无需复位；1 = 需要复位（整机 NVLink 域受影响）','【官方规格】DCGM Field IDs','NVIDIA'),
(67,'DCGM_FI_DEV_XID_ERRORS (230)','GPU',1,'DCGM_FI_DEV_XID_ERRORS',7,1,'驱动上报的 XID 错误码，GPU 稳定性的核心事件源','driver驱动（DCGM）','XID 码','无 XID 上报',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'无 XID 事件；13/31/43/45 属应用侧内存越界，不计入硬件健康度；硬件类高危 XID：48/63/64/94/95（ECC 与行重映射）；61/62/69/74（内部硬件与总线错误）；79（GPU 掉出总线）',1,'XID 79 等价于设备不可用，健康度直接判为故障（0 分）','【官方规格】DCGM Field IDs；dcgm_errors.h (DCGM_FR_XID_ERROR=101)','NVIDIA'),
(68,'DCGM_FI_DRIVER_VERSION (1)','GPU',4,'DCGM_FI_DRIVER_VERSION',8,3,'NVIDIA 驱动版本','driver驱动（DCGM）',NULL,'≥ 535.104.05',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'全集群一致且 ≥ 535.104.05（A100/H100 生产推荐 LTS 分支）','跨节点版本不一致，或低于 535 分支',NULL,0,'低版本驱动在 Hopper 上易触发偶发 XID，建议统一到 535 或 550 LTS','【行业经验】535 为 A100/H100 通行生产基线分支','NVIDIA'),
(69,'DCGM_FI_CUDA_DRIVER_VERSION (5)','GPU',4,'DCGM_FI_CUDA_DRIVER_VERSION',8,3,'CUDA 驱动版本','driver驱动（DCGM）',NULL,'≥ 12020',NULL,12080,12020,NULL,NULL,NULL,NULL,'≥ 12020（CUDA 12.2）且全集群一致','< 12020 或跨节点不一致',NULL,0,NULL,'【行业经验】CUDA 12.2 为当前主流训练框架的通行下限','NVIDIA'),
(70,'DCGM_FI_DEV_INFOROM_CONFIG_VALID (84)','GPU',4,'DCGM_FI_DEV_INFOROM_CONFIG_VALID',6,1,'从 Flash 读取 InfoROM 并校验 checksum 的结果','driver驱动（DCGM）',NULL,'恒为 1',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1','0',NULL,1,'1 = 校验通过；0 = InfoROM 损坏（ECC 历史与配置丢失，DCGM 上报 InfoROM system Warning）','【官方规格】DCGM Field IDs；DCGM User Guide 健康检查示例','NVIDIA'),
(71,'DCGM_FI_DEV_PERSISTENCE_MODE (66)','GPU',4,'DCGM_FI_DEV_PERSISTENCE_MODE',6,3,'驱动持久化模式','driver驱动（DCGM）',NULL,'恒为 1',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1','0',NULL,0,'1 = Enabled；0 = Disabled（首次调用有秒级延迟，且卡状态会复位）','【行业经验】生产集群必开','NVIDIA'),
(72,'DCGM_FI_DEV_MIG_MODE (67)','GPU',4,'DCGM_FI_DEV_MIG_MODE',6,3,'MIG 切分模式开关','driver驱动（DCGM）',NULL,'训练集群恒为 0',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'0','1',NULL,0,'0（训练集群不切分）；1（与训练场景规划不符，会导致调度与监控口径错乱）。推理集群若规划启用 MIG，正常值反转为 1','【行业经验】大规模训练集群默认关闭 MIG','NVIDIA'),
(73,'DCGM_FI_DEV_CC_MODE (74)','GPU',4,'DCGM_FI_DEV_CC_MODE',6,3,'机密计算 / Ampere Protected Memory 状态','driver驱动（DCGM）',NULL,'恒为 0',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'0','1',NULL,0,'0（常规训练集群不启用）；1（开启后性能有损失，需与安全策略核对）','【行业经验】常规训练集群默认关闭','NVIDIA'),
(74,'DCGM_FI_DEV_RELIABILITY_VIOLATION (245)','GPU',1,'DCGM_FI_DEV_RELIABILITY_VIOLATION',4,1,'因可靠性策略限制而降频的累计时长','driver驱动（DCGM）','μs','0',NULL,NULL,0,NULL,0,0,NULL,NULL,NULL,NULL,0,'增量 >0 表示驱动因可靠性原因主动限制性能','【官方规格】DCGM Field IDs','NVIDIA'),
(75,'DCGM_FI_DEV_COUNT (4)','GPU',4,'DCGM_FI_DEV_COUNT',1,1,'节点上被驱动识别到的 GPU 数量','driver驱动（DCGM）','个','8~8',8,8,8,8,NULL,NULL,NULL,NULL,NULL,NULL,1,'4 卡节点则基线改为 4','【行业经验】DGX/HGX 标准训练节点为 8 卡','NVIDIA'),
(76,'DCGM_FI_DEV_GPU_UTIL (203)','GPU',1,'DCGM_FI_DEV_GPU_UTIL',2,2,'GPU 忙碌时间占比（粗粒度，不反映算力饱和度）','compute算力性能','%','0~100',100,90,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'本字段 >90% 但 PROF_SM_ACTIVE <0.3 → 算力空转，按性能问题处理','【官方规格】DCGM Field IDs','NVIDIA'),
(77,'DCGM_FI_DEV_SM_CLOCK (100)','GPU',1,'DCGM_FI_DEV_SM_CLOCK',1,1,'SM 当前频率，判断是否降频的直接观测量','compute算力性能','MHz','1340~1410（满载时）',1410,1340,1340,1340,NULL,NULL,NULL,NULL,NULL,NULL,0,'★满载判据：SM_CLOCK < 1340（即 95% × 1410）且字段112 出现热/功率位 → 判定降频。H100-SXM5 基准为 1980','【实测值】A100-SXM4 最大 SM 频率 = 1410 MHz；【行业经验】95% 为降频判定线','NVIDIA'),
(78,'DCGM_FI_DEV_MAX_SM_CLOCK (113)','GPU',1,'DCGM_FI_DEV_MAX_SM_CLOCK',1,2,'设备支持的最大 SM 频率','compute算力性能','MHz','1410~1410',1410,1410,1410,1410,NULL,NULL,NULL,NULL,NULL,NULL,0,'作为 SM_CLOCK 的分母基准；H100-SXM5 = 1980','【实测值】A100-SXM4 = 1410 MHz','NVIDIA'),
(79,'DCGM_FI_DEV_MEM_CLOCK (101)','GPU',1,'DCGM_FI_DEV_MEM_CLOCK',1,2,'显存当前频率','compute算力性能','MHz','1154~1215（满载时）',1215,1215,1154,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'低于 1154（95% × 1215）判定显存降频','【实测值】A100-SXM4-80GB HBM2e = 1215 MHz','NVIDIA'),
(80,'DCGM_FI_DEV_MAX_MEM_CLOCK (114)','GPU',1,'DCGM_FI_DEV_MAX_MEM_CLOCK',1,2,'设备支持的最大显存频率','compute算力性能','MHz','1215~1215',1215,1215,1215,1215,NULL,NULL,NULL,NULL,NULL,NULL,0,NULL,'【实测值】A100-SXM4 = 1215 MHz；H100-SXM5 = 2619 MHz','NVIDIA'),
(81,'DCGM_FI_DEV_APP_SM_CLOCK (110)','GPU',1,'DCGM_FI_DEV_APP_SM_CLOCK',1,3,'应用锁定的 SM 时钟设定值','compute算力性能','MHz','1410~1410',1410,1410,1410,1410,NULL,NULL,NULL,NULL,NULL,NULL,0,'运维用 nvidia-smi -ac 锁频后此值会低于最大值，属配置类异常','【实测值】A100-SXM4 默认 = 1410 MHz','NVIDIA'),
(82,'DCGM_FI_DEV_APP_MEM_CLOCK (111)','GPU',1,'DCGM_FI_DEV_APP_MEM_CLOCK',1,3,'应用锁定的显存时钟设定值','compute算力性能','MHz','1215~1215',1215,1215,1215,1215,NULL,NULL,NULL,NULL,NULL,NULL,0,NULL,'【实测值】A100-SXM4-80GB = 1215 MHz','NVIDIA'),
(83,'DCGM_FI_PROF_GR_ENGINE_ACTIVE (1001)','GPU',1,'DCGM_FI_PROF_GR_ENGINE_ACTIVE',2,4,'图形/计算引擎处于活跃状态的时间占比','compute算力性能','比例(0~1)','0.5~1.0（训练稳定期）',1,1,0.5,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'训练期低于 0.5 提示存在数据供给或通信瓶颈','【行业经验】训练稳定期 0.5 为关注线','NVIDIA'),
(84,'DCGM_FI_PROF_SM_ACTIVE (1002)','GPU',1,'DCGM_FI_PROF_SM_ACTIVE',2,2,'至少有 1 个 warp 驻留的 SM 周期占比（真实算力占用）','compute算力性能','比例(0~1)','0.6~1.0（训练稳定期）',1,1,0.6,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'★慢卡判据：同集群同负载下，本值低于同批次中位数 10% 即判定慢卡','【行业经验】训练稳定期 0.6 为通行下限','NVIDIA'),
(85,'DCGM_FI_PROF_SM_OCCUPANCY (1003)','GPU',1,'DCGM_FI_PROF_SM_OCCUPANCY',2,4,'SM 上驻留 warp 数占理论最大值的比例','compute算力性能','比例(0~1)','0.3~1.0',1,1,0.3,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'占用率低主要反映 kernel 设计问题，不作为硬件健康度扣分项','【行业经验】0.3 为训练场景常见下限','NVIDIA'),
(86,'DCGM_FI_PROF_PIPE_TENSOR_ACTIVE (1004)','GPU',1,'DCGM_FI_PROF_PIPE_TENSOR_ACTIVE',2,2,'Tensor Core 流水线活跃周期占比（相对峰值可持续周期）','compute算力性能','比例(0~1)','0.3~0.8（混合精度训练）',1,0.8,0.3,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'★算力健康度基线，跨卡同负载横向对比，偏离中位数 10% 即标记慢卡','【行业经验】混合精度训练典型区间 0.3~0.8','NVIDIA'),
(87,'DCGM_FI_PROF_DRAM_ACTIVE (1005)','GPU',1,'DCGM_FI_PROF_DRAM_ACTIVE',2,4,'显存接口收发数据的活跃周期占比','compute算力性能','比例(0~1)','0.2~0.9',1,0.9,0.2,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'>0.9 且 SM_ACTIVE <0.3 → memory-bound，属性能问题','【行业经验】0.9 为访存瓶颈判定线','NVIDIA'),
(88,'DCGM_FI_PROF_PIPE_FP16_ACTIVE (1008)','GPU',1,'DCGM_FI_PROF_PIPE_FP16_ACTIVE',2,4,'FP16 流水线活跃周期占比（不含 HMMA）','compute算力性能','比例(0~1)','0~1.0',1,1,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,NULL,'【官方规格】DCGM Field IDs','NVIDIA'),
(89,'DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION (246)','GPU',1,'DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION',4,1,'未能维持应用设定时钟的累计时长','compute算力性能','μs','0',NULL,NULL,0,NULL,0,0,NULL,NULL,NULL,NULL,0,'增量 >0 = 无法维持应用时钟（降频的旁证）','【官方规格】DCGM Field IDs','NVIDIA'),
(90,'DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION (247)','GPU',1,'DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION',4,1,'未能维持基准时钟的累计时长','compute算力性能','μs','0',NULL,NULL,0,NULL,0,0,NULL,NULL,NULL,NULL,0,'★增量 >0 = 已跌破基准频率，属严重降频，按高危扣分','【官方规格】DCGM Field IDs','NVIDIA'),
(91,'DCGM_FI_DEV_LOW_UTIL_VIOLATION (244)','GPU',1,'DCGM_FI_DEV_LOW_UTIL_VIOLATION',4,4,'因低利用率而限制时钟的累计时长','compute算力性能','μs','0',NULL,NULL,0,NULL,0,0,NULL,NULL,NULL,NULL,0,'GPU_UTIL >10% 时应为 0 μs/s，空闲期增量属正常。属节能行为，非故障，健康度评分中不扣分','【官方规格】DCGM Field IDs','NVIDIA'),
(92,'DCGM_FI_DEV_SYNC_BOOST_VIOLATION (242)','GPU',1,'DCGM_FI_DEV_SYNC_BOOST_VIOLATION',4,2,'Sync Boost 组内被其他卡拖低时钟的累计时长','compute算力性能','μs','0',NULL,NULL,0,NULL,0,0,NULL,NULL,NULL,NULL,0,'增量 >0 表示同组存在慢卡，需定位组内最热或最受限的那张','【官方规格】DCGM Field IDs','NVIDIA'),
(93,'DCGM_FI_DEV_DEC_UTIL（206）','GPU',1,'DCGM_FI_DEV_DEC_UTIL',2,4,'视频解码器(NVENC)利用率','compute算力性能','%','0～100',100,100,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'训练场景通常恒为 0；持续偏高多为编解码型业务，非硬件故障','【官方规格】DCGM Field IDs','NVIDIA'),
(94,'DCGM_FI_DEV_ENC_UTIL（205）','GPU',1,'DCGM_FI_DEV_ENC_UTIL',2,4,'视频编码器(NVENC)利用率','compute算力性能','%','0～100',100,100,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'训练场景通常恒为 0；持续偏高多为编解码型业务，非硬件故障','【官方规格】DCGM Field IDs','NVIDIA'),
(95,'DCGM_FI_DEV_VGPU_LICENSE_STATUS','GPU',4,'DCGM_FI_DEV_VGPU_LICENSE_STATUS',6,3,'vGPU 授权(许可)状态；仅在启用 vGPU/GRID 授权时有效','driver驱动（DCGM）',NULL,'恒为 1（已授权）',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1','0',NULL,0,'1 = 已获得有效授权(Licensed)；0 = 未授权/授权失效(Unlicensed)','【官方规格】DCGM Field IDs','NVIDIA'),
(96,'dcmi_get_device_temperature / 
npu-smi info -t temp/（npu-smi: Temperature(C) / Temp(C)）','NPU',1,'npu_chip_info_temperature',1,1,'昇腾 AI 处理器 die 当前温度','thermal温度散热','℃','20~85',95,85,20,0,NULL,NULL,NULL,NULL,NULL,NULL,1,'降频阈值 = 85（持续 >85℃ 触发频率限制）；软件保护停机线建议取 95','【实测值】空闲 37~48，满载约 60，>85 降频；【行业经验】华为未公开硬件关机温度，95 为运维侧主动停机建议值','华为昇腾'),
(97,'dcmi_get_device_hbm_info / npu-smi info -t memory/（npu-smi: HBM Temperature）','NPU',1,'npu_chip_info_hbm_temperature',1,1,'HBM 显存颗粒温度（区别于芯片整体温度）','thermal温度散热','℃','20~85',95,85,20,0,NULL,NULL,NULL,NULL,NULL,NULL,1,'降频阈值 = 85，HBM 过温同样触发整卡频率限制','【实测值】空闲约 38；【行业经验】HBM2e 颗粒结温上限 95，取 85 作保守告警线','华为昇腾'),
(98,'npu-smi info -t temp -i <id> -c 1/（Chip Name = mcu）','NPU',1,'MCU Temperature(C)',1,2,'板载管理控制单元(MCU)温度','thermal温度散热','℃','20~75',NULL,75,20,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'MCU 温度 >75 提示整卡供电或散热子系统异常','【实测值】正常 30~45；【行业经验】75 为告警线','华为昇腾'),
(99,'npu-smi info -t temp','NPU',1,'LM75A_TE / LM75B_TE',1,2,'板载 LM75 温度传感器读数，用于与 die 温度交叉校验','thermal温度散热','℃','20~80',NULL,80,20,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'与 die 温度差值应稳定在 20℃ 以内
die 温度与 LM75 读数差值 >20℃ → 传感器故障或热耦合失效','【行业经验】20℃ 差值为传感器一致性判据','华为昇腾'),
(100,'产品规格（Atlas 300T Pro / A2 训练卡白皮书）','NPU',4,'工作环境温度（进风口温度）',1,2,'单板允许的工作环境温度范围','thermal温度散热','℃','5~45',NULL,45,5,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'超出 45℃ 会显著提前触发 85℃ 降频；存储温度规格为 -40~70','【官方规格】华为 Atlas 300T Pro 训练卡：工作温度 5℃~45℃，海拔 >900m 需降额','华为昇腾'),
(101,'dcmi_get_device_power_info / npu-smi info -t power/（npu-smi: NPU Real-time Power(W) / Power(W)）','NPU',1,'npu_chip_info_power',1,2,'昇腾 AI 处理器实时功耗（部分形态为整板功耗）','power功耗电源','W','90~400',400,400,90,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'★功耗达到 400 触发功率封顶降频。Atlas 300T Pro PCIe 卡对应值为 300','【官方规格】昇腾 910 处理器 310W，910B OAM 模组 400W，Atlas 300T Pro 整卡最大 300W；【实测值】910B3 空闲 90~93，FP16 满载 231，训练峰值 273','华为昇腾'),
(102,'dcmi_get_device_voltage / npu-smi info -t volt/（npu-smi: Voltage(V)）','NPU',1,'npu_chip_info_voltage',1,2,'芯片核心供电电压','power功耗电源','V','0.76~0.84',NULL,0.84,0.76,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'电压跌出 ±5% 区间常与硬件供电保护降频同时出现','【行业经验】昇腾未公开标称核心电压，按 7nm AI 芯片通行值 0.8V ±5% 取定','华为昇腾'),
(103,'npu-smi info -t power / npu-smi set -t power-limit','NPU',1,'Power Limit（功率上限设定值）',1,3,'功率管理上限，可由运维配置（静态固定值）','power功耗电源','W','400~400',400,400,400,400,NULL,NULL,NULL,NULL,NULL,NULL,0,'★功率封顶点，实时功耗达到该值即启动降频','【实测值】910B OAM 默认 400W；【行业经验】可设下限按 TDP 的 40% 取 150W','华为昇腾'),
(104,'整机 iBMC / 板载电源','NPU',3,'板卡供电电流 / 12V 电压',1,2,'PCIe 槽位与辅助供电的电流电压','power功耗电源','A / V','槽位 ≤5.5A@12V，辅助 ≤18.75A@12V，电压 11.4~12.6',17.8125,18.75,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'12V 跌出 ±5%（11.4~12.6）会触发硬件保护降频甚至掉卡','【官方规格】Atlas 300T Pro：槽位 5.5A@12V + 0.5A@3.3V，辅助电源 18.75A@12V；【行业经验】±5% 为 12V 通行容差','华为昇腾'),
(105,'dcmi_get_device_utilization_rate (input_type=2) / npu-smi info -t usages/（npu-smi: Aicore Usage Rate(%) / AICore(%)）','NPU',1,'npu_chip_info_utilization',2,1,'AI Core 利用率','compute算力性能','%','60~99（训练稳定期）',100,99,60,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'vNPU 算力切分场景下该值可能恒为 0，此时无意义，需改用容器内指标','【实测值】训练稳定期 60~99；【官方规格】华为云 CCE NPU 指标说明','华为昇腾'),
(106,'dcmi_get_device_utilization_rate / npu-smi info -t usages/（npu-smi: Aivector Usage Rate(%)）','NPU',1,'npu_chip_info_vector_utilization',2,4,'Vector(矢量)计算单元利用率','compute算力性能','%','0~100',100,100,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'与 AI Core 利用率差值应 <30 个百分点
与 AI Core 利用率长期背离 >30 个百分点提示算子调度异常','【实测值】通常与 AI Core 同步变化；【行业经验】30 个百分点为背离判据','华为昇腾'),
(107,'npu-smi info -t usages','NPU',1,'Aicpu Usage Rate(%)',2,4,'AI CPU 利用率（处理控制流与非矩阵算子）','compute算力性能','%','0~80',100,80,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'>80 且 AI Core <30 = 算子未下沉到 NPU，属性能问题非硬件故障','【行业经验】80% 为 AI CPU 瓶颈判定线','华为昇腾'),
(108,'npu-smi info -t usages','NPU',1,'Ctrlcpu Usage Rate(%)',2,4,'控制 CPU 利用率（负责设备调度与通信）','compute算力性能','%','0~80',100,80,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'>80 提示设备调度或通信开销过大','【行业经验】80% 为告警线','华为昇腾'),
(109,'dcmi_get_device_frequency (input_type=7) / npu-smi info -t common/（npu-smi: Aicore curFreq(MHZ)）','NPU',1,'npu_chip_info_aicore_current_freq',1,1,'AI Core 当前实际频率，判断是否降频的直接观测量','compute算力性能','MHz','1710~1800（满载时）',NULL,1800,1710,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'★降频判据：满载时 curFreq < 1710（即 95% × 1800）→ 判定降频；再结合温度 >85 归因热降频、功耗触及 400 归因功率降频','【实测值】910B3 空闲 800，满载 1800；【行业经验】95% 为降频判定线','华为昇腾'),
(110,'dcmi_get_device_frequency (input_type=9) / npu-smi info -t common/（npu-smi: Aicore Freq(MHZ)）','NPU',1,'npu_chip_info_aicore_freq',1,2,'AI Core 标称(额定)频率','compute算力性能','MHz','1800~1800',1800,1800,1800,1800,NULL,NULL,NULL,NULL,NULL,NULL,0,'作为 curFreq 的分母基准','【实测值】910B3 = 1800 MHz','华为昇腾'),
(111,'dcmi_get_aicore_info / npu-smi info -t common','NPU',1,'Aicore Count',1,1,'可用 AI Core 数量','compute算力性能','个','24~24',24,24,24,24,NULL,NULL,NULL,NULL,NULL,NULL,1,'核数掉落是硬件永久性降级，健康度直接判为故障。910B4/300T A2 基线为 20','【实测值】910B（910B2/B3）= 24 核；Atlas 300T A2（910B4）= 20 核','华为昇腾'),
(112,'dcmi_get_computing_power_info / ascend-dmi -f','NPU',1,'实测算力 (FP16 TFLOPS)',1,2,'压测实测算力，算力健康度基线','compute算力性能','TFLOPS','288~320',NULL,320,288,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'★288 = 标称 320 的 90%，低于此值判定算力衰减；低于同批次中位数 10% 判定慢卡','【官方规格】昇腾 910B 标称 FP16 320 TFLOPS；【行业经验】90% 为验收下限','华为昇腾'),
(113,'dcmi_get_device_hbm_info / npu-smi info -t usages/（npu-smi: HBM Capacity(MB)）','NPU',1,'npu_chip_info_hbm_total_memory',1,1,'HBM 总容量','memory显存可靠性','MB','65536~65536',65536,65536,65536,65536,NULL,NULL,NULL,NULL,NULL,NULL,0,'关闭 ECC 后容量会升至约 95623 MB，属配置变更非故障。32GB 版基线为 32768','【实测值】910B 64GB 版 = 65536 MB','华为昇腾'),
(114,'dcmi_get_device_hbm_info / npu-smi info/（npu-smi: HBM-Usage(MB)）','NPU',1,'npu_chip_info_hbm_used_memory',1,4,'HBM 已用量','memory显存可靠性','MB','3300~62259',65536,62259,3300,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'空闲态 3300~3400 MB 为驱动保留（约 5%）；62259 = 总量 95%，为耗尽告警线','【实测值】910B3 空闲保留 3379 MB；【行业经验】95% 为告警水位','华为昇腾'),
(115,'dcmi_get_device_hbm_info / npu-smi info -t usages','NPU',1,'HBM Usage Rate(%)',2,4,'HBM 使用率','memory显存可靠性','%','4~90',100,90,4,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'空闲约 4~5%；90% 为告警线','【实测值】空闲 4~5%；【行业经验】90% 告警','华为昇腾'),
(116,'dcmi_get_device_hbm_info / npu-smi info -t usages/（npu-smi: HBM Bandwidth Usage Rate(%)）','NPU',1,'npu_chip_info_hbm_bandwidth_util',2,4,'HBM 带宽利用率','memory显存可靠性','%','0~90',100,90,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'>90 且 AI Core 利用率 <30 = 访存瓶颈，属性能问题非硬件故障','【行业经验】90% 为访存瓶颈判定线','华为昇腾'),
(117,'dcmi_get_device_frequency (input_type=6) / npu-smi info -t memory/（npu-smi: HBM Clock Speed）','NPU',1,'npu_chip_info_hbm_freq',1,2,'HBM 工作频率','memory显存可靠性','MHz','1520~1600',NULL,1600,1520,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'1520 = 95% × 1600，为显存降频判定线','【实测值】910B = 1600 MHz，对应约 1.54 TB/s 带宽','华为昇腾'),
(118,'dcmi_get_device_ecc_info / npu-smi info -t ecc','NPU',1,'HBM Single Bit Error Count',3,1,'HBM 可纠正单比特 ECC 错误累计数','memory显存可靠性','次','0~1000',NULL,1000,0,NULL,0,24,'次/天',NULL,NULL,NULL,0,'≤1 次/小时；>24 次/天 判定 HBM 退化；>1000 次/天 判定需停机检修
累计 >1000 次建议安排 RMA 评估','【实测值】健康服务器该计数恒为 0；【行业经验】1000 次/天为业内通行检修线','华为昇腾'),
(119,'dcmi_get_device_ecc_info / npu-smi info -t ecc','NPU',1,'HBM Double Bit Error Count',3,1,'HBM 不可纠正双比特 ECC 错误累计数','memory显存可靠性','次','0~0',NULL,0,0,NULL,0,0,'次/天',NULL,NULL,NULL,1,'次/天，>0（任意一次即需立即隔离并硬件检查）','【实测值】健康服务器恒为 0；【行业经验】任意 >0 即 RMA 候选','华为昇腾'),
(120,'dcmi_get_device_ecc_info / npu-smi info -t ecc','NPU',1,'Isolated Pages Count（隔离页计数）',3,1,'已被驱动隔离的 HBM 坏页数','memory显存可靠性','页','0~8',NULL,64,0,NULL,0,8,'页',NULL,NULL,NULL,0,'页/天；累计 >8 页进入关注，>64 页建议 RMA。持续增长（HBM 物理退化）','【实测值】健康服务器恒为 0；【行业经验】对齐 NVIDIA 页黑名单 64 页容量取定','华为昇腾'),
(121,'npu-smi info -t ecc-enable / npu-smi set -t ecc-enable','NPU',4,'ECC Enable（HBM ECC 开关）',6,3,'HBM ECC 使能状态。关闭可释放约 30 GB 显存但完全失去纠错能力','memory显存可靠性',NULL,'恒为 1',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1','0',NULL,0,'1 = Enabled；0 = Disabled（生产训练环境禁止）','【官方规格】npu-smi set -t ecc-enable -d 0/1','华为昇腾'),
(122,'dcmi_get_device_memory_info / npu-smi info/（npu-smi: Memory-Usage(MB)）','NPU',1,'npu_chip_info_used_memory',1,4,'DDR 内存已使用量。Atlas A2 训练系列无 DDR 模块，恒为 0','memory显存可靠性','MB','0~0（A2 训练系列）',0,0,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'带 DDR 的形态（如 Atlas 300T Pro，16 GB DDR4）上界改为 16384，告警线 15564（95%）','【官方规格】MindCluster 文档：Atlas A2 训练系列产品没有 DDR 模块，不上报相关指标','华为昇腾'),
(123,'dcmi_get_device_memory_info/（npu-smi: DDR Capacity(MB)）','NPU',1,'npu_chip_info_total_memory',1,4,'DDR 内存总量','memory显存可靠性','MB','0~0（A2 训练系列）',0,0,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'Atlas 300T Pro 形态为 16384 MB','【官方规格】华为云 CCE NPU 指标说明','华为昇腾'),
(124,'npu-smi info / npu-smi info -t usages','NPU',1,'Hugepages-Usage(page) / DDR Hugepages Usage Rate(%)',5,4,'DDR 大页内存占用','memory显存可靠性','page / %','0~0（A2 训练系列）',NULL,0,0,NULL,NULL,NULL,NULL,NULL,NULL,NULL,0,'A2 训练系列无 DDR，恒为 0/0；有 DDR 的形态按 95% 设告警线','【实测值】910B3 输出恒为 0 / 0','华为昇腾'),
(125,'dcmi_get_device_pcie_info_v2 / PCIe sysfs / npu-smi info -t board','NPU',2,'PCIe Max Link Speed',7,2,'设备支持的最大 PCIe 速率（最大链路速度）','pcie总线','GT/s','16~16',NULL,16,16,NULL,NULL,NULL,NULL,NULL,NULL,'16 GT/s = PCIe 4.0',0,'16 GT/s = PCIe 4.0','【官方规格】Atlas 300T A2 / 910B 均为 PCIe 4.0 x16','华为昇腾'),
(126,'dcmi_get_device_pcie_info_v2 / PCIe sysfs','NPU',2,'PCIe Current Link Speed',7,1,'当前协商的 PCIe 速率（当前链路速度）','pcie总线','GT/s','16~16',NULL,16,16,NULL,NULL,NULL,NULL,NULL,NULL,'16（等于 Max Link Speed）；< 16（链路协商降级，如降到 8 GT/s Gen3）',0,'降速直接导致 H2D/D2H 带宽腰斩，需检查插槽、金手指与 BIOS','【官方规格】判据为 Current == Max','华为昇腾'),
(127,'dcmi_get_device_pcie_info_v2 / PCIe sysfs','NPU',2,'PCIe Max Link Width',7,2,'设备支持的最大 lane 数（最大链路宽度）','pcie总线','lane','16~16',NULL,16,16,NULL,NULL,NULL,NULL,NULL,NULL,'PCIe 4.0 x16，双向 64 GB/s',0,NULL,'【官方规格】Atlas 300T A2：PCIe 4.0 x16，双向 64 GB/s','华为昇腾'),
(128,'dcmi_get_device_pcie_info_v2 / PCIe sysfs','NPU',2,'PCIe Current Link Width',7,1,'当前协商的 lane 数（当前链路宽度）','pcie总线','lane','16~16',NULL,16,16,NULL,NULL,NULL,NULL,NULL,NULL,'16（等于 Max Link Width）；< 16（降宽至 x8/x4）',0,'与当前速度共同构成 PCIe 链路健康的核心判据','【官方规格】判据为 Current == Max','华为昇腾'),
(129,'PCIe sysfs / dcmi_get_device_pcie_info_v2','NPU',2,'PCIe 实测带宽（单向）',1,4,'H2D / D2H 实测传输带宽','pcie总线','GB/s','24~31.5',NULL,31.5,24,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'31.5 GB/s 为 Gen4 x16 单向理论值，实测低于 24 GB/s 需排查链路降级','【官方规格】PCIe 4.0 x16 = 31.5 GB/s 单向；【行业经验】24 GB/s 为实测下限','华为昇腾'),
(130,'npu-smi info -t pcie-err','NPU',2,'PCIe TX Error Count / RX Error Count',3,1,'PCIe 收发错误计数','pcie总线','次','0~0',NULL,0,0,NULL,0,0,'次/小时',NULL,NULL,NULL,0,'次/小时，>0异常','【实测值】健康服务器 TX/RX 计数均为 0','华为昇腾'),
(131,'npu-smi info -t pcie-err','NPU',2,'LCRC Error Count',3,1,'PCIe 链路层 CRC 错误计数','pcie总线','次','0~0',NULL,0,0,NULL,0,0,'次/小时',NULL,NULL,NULL,0,'次/小时，>0（链路信号质量劣化）','【实测值】健康服务器为 0','华为昇腾'),
(132,'npu-smi info -t pcie-err','NPU',2,'ECRC Error Count',3,1,'PCIe 端到端 CRC 错误计数','pcie总线','次','0~0',NULL,0,0,NULL,0,0,'次/小时',NULL,NULL,NULL,0,'次/小时，>0异常','【实测值】健康服务器为 0','华为昇腾'),
(133,'npu-smi info -t pcie-err','NPU',2,'Retry Count',3,1,'PCIe 链路重传次数，等价于 DCGM 的 PCIe Replay Counter','pcie总线','次','长期不增长',NULL,NULL,0,NULL,0,8,'次/分钟',NULL,NULL,NULL,0,'★≤8 次/分钟；>8 次/分钟 告警；>100 次/分钟 判定链路严重劣化','【实测值】健康服务器为 0；【行业经验】8 次/分钟阈值口径对齐 NVIDIA DCGM','华为昇腾'),
(134,'npu-smi info -t board','NPU',2,'PCIe Bus Info（BDF）',8,3,'PCIe 总线地址，用于将逻辑卡号映射到物理插槽（字符串）','pcie总线',NULL,'与整机拓扑基线一致',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'与基线拓扑完全一致','BDF 变化或消失（掉卡）',NULL,0,NULL,'【实测值】格式如 0000:C1:00.0','华为昇腾'),
(135,'npu-smi info -t hccs -i <id> -c 0','NPU',2,'hccs health status',7,1,'HCCS（华为缓存一致性互联）链路健康状态','interconnect昇腾互连通信',NULL,'恒为 OK',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'OK；非 OK（链路异常，集合通信会超时）',0,NULL,'【实测值】健康服务器为 OK','华为昇腾'),
(136,'npu-smi info -t hccs -i <id> -c 0','NPU',2,'hccs link lane list',8,1,'每条 HCCS 链路各 lane 的在线状态（1=active）','interconnect昇腾互连通信',NULL,'全部为 1',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1','0','所有 lane 均为 1（如 [1111 1111 ...]）；存在 0（部分 lane 掉线，带宽按比例下降）',0,NULL,'【实测值】910B3 8 卡全互联，全部为 1111','华为昇腾'),
(137,'npu-smi info -t hccs -i <id> -c 0','NPU',2,'hccs lane mode',7,2,'HCCS 组网 lane 模式','interconnect昇腾互连通信','条','4~4',NULL,4,4,NULL,NULL,NULL,NULL,NULL,NULL,'< 4（组网降级）',0,NULL,'【实测值】910B3 = 4','华为昇腾'),
(138,'npu-smi info -t hccs -i <id> -c 0','NPU',2,'hccs link speed',1,2,'单 lane 链路速率','interconnect昇腾互连通信','Gbps','224~224',224,224,224,224,NULL,NULL,NULL,NULL,NULL,NULL,0,'单条 HCCS 链路带宽 = 4 lane × 224 Gbps = 896 Gbps ≈ 112 GB/s（全双工）','【实测值】910B3 = 224 Gbps/lane','华为昇腾'),
(139,'npu-smi info -t hccs -i <id> -c 0','NPU',2,'hccs retry count',3,1,'HCCS 链路重传计数','interconnect昇腾互连通信','次','0~0',NULL,0,0,NULL,0,0,'次/小时',NULL,NULL,NULL,0,'次/小时；>10 次/小时 判定链路劣化；>0（误码率升高）','【实测值】健康服务器全部为 0；【行业经验】10 次/小时为劣化线','华为昇腾'),
(140,'npu-smi info -t hccs -i <id> -c 0','NPU',2,'hccs error count',3,1,'HCCS 链路误码计数','interconnect昇腾互连通信','次','0~0',NULL,0,0,NULL,0,0,'次/小时',NULL,NULL,NULL,0,'次/小时，>0异常','【实测值】健康服务器全部为 0','华为昇腾'),
(141,'npu-smi info -t hccs-bw -i <id> -c 0 -time 100','NPU',2,'HCCS 实时带宽',1,2,'HCCS 链路实测带宽','interconnect昇腾互连通信','GB/s','23.6~112',NULL,112,23.6,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'112 GB/s 为物理上限（全双工）；应用层 P2P 实测约 26.2 GB/s，23.6 = 26.2 的 90%，为 P2P 劣化判定线','【实测值】910B3 P2P 实测 26.2 GB/s（ascend-dmi）；【行业经验】90% 为衰减判定线','华为昇腾'),
(142,'npu-smi info -l（拓扑矩阵）','NPU',2,'卡间互联类型（HCCS/SYS/PHB/PIX/PXB/NA）',7,2,'8×8 卡间互联类型矩阵','interconnect昇腾互连通信',NULL,'全部为 HCCS',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'HCCS（8 卡全互联 full mesh，AllReduce 延迟对称）；SYS；PHB；PIX；PXB（经 PCIe 或跨 NUMA，延迟不对称）；NA（无法判定）',0,'拓扑退化不一定是故障，但会造成集合通信长尾，按性能维度扣分','【实测值】910B3 训练机 8 卡全部 HCCS 直连','华为昇腾'),
(143,'dcmi_get_device_network_health / hccn_tool -i <id> -link -g','NPU',5,'npu_chip_info_network_status',7,1,'卡上 200GE RoCE 网口的链路状态','interconnect昇腾互连通信',NULL,'恒为 1',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1 = UP / Success；0 = DOWN（跨机集合通信直接中断）',0,NULL,'【官方规格】DCMI dcmi_get_device_network_health；华为云 CCE NPU 指标说明','华为昇腾'),
(144,'hccn_tool -i <id> -bandwidth -g','NPU',5,'npu_chip_info_bandwidth_tx / npu_chip_info_bandwidth_rx',8,4,'RoCE 网口实时收发速率（瞬时速率）','interconnect昇腾互连通信','MB/s','0~25000',25000,25000,12500,0,12500,0,NULL,NULL,NULL,NULL,0,'25000 MB/s = 200GE 线速；长期低于 12500 需排查拥塞或链路降级','【官方规格】昇腾 910B 集成 1×200GE RoCE；【行业经验】50% 线速为跨机训练下限','华为昇腾'),
(145,'hccn_tool -i <id> -stat -g','NPU',5,'RoCE 错包 / 重试 / 乱序 / CNP 计数',3,1,'RoCE 传输层统计：错包、重传、乱序包、拥塞通知(CNP)','interconnect昇腾互连通信','次','0~0（错包/乱序）',NULL,0,0,NULL,0,0,'次/小时',NULL,NULL,NULL,0,'错包与乱序 0 次/小时；CNP >100 次/秒 判定网络拥塞；错包/乱序 >0；CNP 持续增长（拥塞，表现为集合通信超时）','【行业经验】RoCE 无损网络下错包应为 0；CNP 100 次/秒为拥塞告警线','华为昇腾'),
(146,'hccn_tool -i <id> -optical -g','NPU',5,'光模块温度',1,2,'光模块工作温度','interconnect昇腾互连通信','℃','0~70',NULL,70,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'超过 70℃ 误码率显著上升并引发链路重训','【行业经验】商用温度等级光模块工作上限 70℃','华为昇腾'),
(147,'hccn_tool -i <id> -optical -g','NPU',5,'光模块收发光功率',1,2,'光模块发送与接收光功率','interconnect昇腾互连通信','dBm','-7~2（发送）；-10~0.5（接收）',NULL,2,-10,NULL,NULL,NULL,NULL,NULL,NULL,NULL,0,'接收光功率低于 -10 dBm 判定光链路衰减超标','【行业经验】200G 光模块通行工作区间','华为昇腾'),
(148,'hccn_tool -i <id> -optical -g','NPU',5,'光模块 LOS / SNR',6,2,'光信号丢失标志与信噪比（bool+连续数值）','interconnect昇腾互连通信','dB','LOS=0；SNR ≥ 15',NULL,15,15,NULL,NULL,NULL,NULL,'0','1','LOS = 0（无光信号丢失）；LOS = 1；或 SNR < 15（信号质量劣化）',0,NULL,'【行业经验】SNR 15 dB 为高速光链路通行下限','华为昇腾'),
(149,'dcmi_get_device_health / npu-smi info -t health/（npu-smi: Health）','NPU',1,'npu_chip_info_health_status',7,1,'昇腾 AI 处理器综合健康状态，健康度评估的一级判据','reliability昇腾可靠性与运行状态',NULL,'恒为 1 / OK',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'1 = OK（npu-exporter 中 1 表示健康）；0 = 不健康。npu-smi 细分：Warning（一般告警，温度/功耗接近阈值或 ECC 增长）；Alarm（重要告警）；Critical（紧急，需立即停任务）；UNKNOWN（设备不存在或未启动）',1,'Alarm / Critical 常由持续高温、电源异常或严重硬件错误触发','【官方规格】华为云 CCE NPU 指标说明（0=不健康，1=健康）','华为昇腾'),
(150,'npu-smi info -t health -i <id>','NPU',1,'健康告警码（error code）',7,1,'Health 非 OK 时上报的具体故障码（故障码）','reliability昇腾可靠性与运行状态',NULL,'无告警码',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'无告警码上报；上报任意告警码（需对照华为《故障处理》手册定位）',0,'健康度评估应把告警码作为可解释性输出，而非只记录 OK / 非 OK','【官方规格】npu-smi info -t health 说明','华为昇腾'),
(151,'npu-exporter','NPU',1,'npu_chip_info_error_code',7,1,'npu-exporter 上报的芯片错误码（故障码）','reliability昇腾可靠性与运行状态',NULL,'0~0',NULL,0,0,NULL,NULL,NULL,NULL,NULL,NULL,'0；≠ 0',0,NULL,'【官方规格】MindCluster NPU Exporter Prometheus Metrics 接口','华为昇腾'),
(152,'npu-smi info -i <id> -t board','NPU',4,'Compatibility',7,3,'驱动与固件版本兼容性自检结果（驱动/固件兼容性）','reliability昇腾可靠性与运行状态',NULL,'恒为 OK',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'OK；非 OK（版本不匹配，易出现 dcmi module initialize failed）',0,NULL,'【实测值】npu-smi info -t board 输出 Compatibility : OK','华为昇腾'),
(153,'npu-smi info -t board','NPU',4,'Software Version',8,3,'昇腾驱动版本（版本字符串）','reliability昇腾可靠性与运行状态',NULL,'≥ 23.0.0 且全集群一致',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'全节点一致且 ≥ 23.0.0（910B 生产通行基线）','跨节点版本不一致，或低于 23.0.0',NULL,0,'内核升级/降级后未重装驱动会直接导致 dcmi module initialize failed（ret -8005）','【实测值】常见生产版本 23.0.5 / 24.1.0.3 / 25.0.rc1；【行业经验】23.0 为 910B 通行基线','华为昇腾'),
(154,'npu-smi info -t board','NPU',4,'Firmware Version',8,3,'板卡固件版本（版本字符串）','reliability昇腾可靠性与运行状态',NULL,'≥ 7.1.0.0 且全集群一致',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'全节点一致且 ≥ 7.1.0.0','跨节点不一致或低于 7.1.0.0',NULL,0,NULL,'【实测值】常见生产版本 7.1.0.7.220 / 7.5.0.5.220','华为昇腾'),
(155,'npu-smi info -t board','NPU',1,'Faulty Chip Count',3,1,'板卡上被判定为故障的芯片数','reliability昇腾可靠性与运行状态','个','0~0',NULL,0,0,NULL,NULL,NULL,NULL,NULL,NULL,NULL,1,'>0（该卡应立即隔离）','【实测值】npu-smi info -t board 输出 Faulty Chip Count : 0','华为昇腾'),
(156,'npu-exporter / npu-smi info -l','NPU',4,'machine_npu_nums',1,1,'节点上被识别到的 NPU 数量','reliability昇腾可靠性与运行状态','个','8~8',8,8,8,8,NULL,NULL,NULL,NULL,NULL,NULL,1,'掉卡常伴随驱动日志 uda_wait_all_phy_startup 超时。4 卡节点基线改为 4。< 8（掉卡）','【实测值】Atlas 800T A2 标准配置 8 卡','华为昇腾'),
(157,'dcmi_get_device_flash_count / dcmi_get_device_flash_info','NPU',1,'Flash 坏块计数',3,1,'板载 Flash 坏块统计','reliability昇腾可靠性与运行状态','块','0~0',NULL,0,0,NULL,0,1,'块/月',NULL,NULL,NULL,1,'块/月，>0（Flash 退化，可能影响固件加载）','【行业经验】Flash 坏块任意 >0 即需关注','华为昇腾'),
(158,'dcmi_get_device_logic_id / dcmi_get_device_phyid_from_logicid','NPU',4,'Chip Logic ID / Chip Physical ID 映射',8,3,'逻辑卡号与物理卡号的映射，用于故障定位','reliability昇腾可靠性与运行状态',NULL,'与装机基线映射完全一致',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'与基线映射一致','映射变化（设备枚举顺序变动或掉卡）',NULL,0,NULL,'【官方规格】DCMI dcmi_get_device_logic_id；npu-smi info -m','华为昇腾'),
(159,'npu-smi info -t board','NPU',4,'VDie ID / Serial Number',8,3,'芯片唯一标识与序列号，作为健康度时序数据的主键','reliability昇腾可靠性与运行状态',NULL,'与资产台账完全一致且恒定',NULL,NULL,NULL,NULL,NULL,NULL,NULL,'与台账一致','发生变化（换卡未同步台账）',NULL,0,NULL,'【实测值】npu-smi info -t board 输出 VDie ID / Serial Number；npu-exporter vdie_id 标签','华为昇腾'),
(160,'npu-smi info -t common（派生）','NPU',6,'降频率 = Aicore curFreq / Aicore Freq',2,1,'当前频率相对标称频率的比值，量化降频程度','auxiliary辅助与效率指标','%','95~100（满载时）',100,100,95,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'★核心派生指标：<95% 且温度 >85 → 热降频；<95% 且功耗 ≥400 → 功率降频；两者皆否 → 排查供电或固件','【行业经验】由 npu-smi info -t common 的 curFreq 与 Freq 计算，95% 为判定线','华为昇腾'),
(161,'npu-smi / npu-exporter（派生）','NPU',6,'能效比 = 实测算力 / 实时功耗',8,4,'单位功耗算力，横向识别效率异常卡（派生比值）','auxiliary辅助与效率指标','TFLOPS/W','0.72~0.80',1,0.8,0.72,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'0.80 = 320 TFLOPS ÷ 400 W 理论值；低于 0.72（理论值 90%）判定效率衰减','【行业经验】由标称算力与 TDP 计算，90% 为衰减线','华为昇腾'),
(162,'npu-smi / iBMC（派生）','NPU',6,'温升 = 芯片温度 − 进风口温度',8,2,'剔除环境影响后的散热能力度量（派生比值）','auxiliary辅助与效率指标','℃','0~40',NULL,40,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'满载温升 >40℃ 判定散热劣化（导热硅脂老化或风道积尘）','【行业经验】满载芯片 60℃ − 环境 25℃ ≈ 35℃，取 40 为告警线','华为昇腾'),
(163,'npu-smi info（进程表）','NPU',1,'Process id / Process name / Process memory(MB)',1,4,'占用 NPU 的进程及其显存占用','auxiliary辅助与效率指标','MB','0~62259',65536,62259,0,0,NULL,NULL,NULL,NULL,NULL,NULL,0,NULL,'【实测值】910B3 空闲态驱动保留 3379 MB','华为昇腾'),
(164,'ascend-dmi --bw','NPU',1,'HBM 实测带宽',1,2,'HBM 实测读写带宽','auxiliary辅助与效率指标','GB/s','1386~1540',NULL,1540,1386,0,NULL,NULL,NULL,NULL,NULL,NULL,0,'1540 GB/s（1.54 TB/s）为 910B 标称值；低于 1386（90%）判定显存性能衰减','【实测值】910B HBM 约 1.54 TB/s；【行业经验】90% 为验收下限','华为昇腾'),
(165,'ascend-dmi --dg -i device,hbm,aicore','NPU',1,'诊断项结果（device / hbm / aicore）',7,2,'主动诊断结果，周期性巡检项','auxiliary辅助与效率指标',NULL,'全部 Pass',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,'Pass；Fail（需按诊断项定位）',0,'aicore / prbs / edp / tdp 四个诊断项互斥，不能与其他项同时执行','【官方规格】ascend-dmi --dg 用法说明','华为昇腾'),
(166,'ascend-dmi 信号质量检测','NPU',5,'HCCS / PCIe / RoCE 物理层误码率',1,2,'物理层信号质量，间歇性通信超时的首选排查项','auxiliary辅助与效率指标','BER','0~1e-12',NULL,1e-12,0,NULL,NULL,NULL,NULL,NULL,NULL,NULL,0,'1e-12 为高速串行链路通行 BER 上限；超过即判定信号质量劣化','【行业经验】PCIe/RoCE 等高速 SerDes 通行 BER 指标 1e-12','华为昇腾');

SELECT COUNT(*) AS total FROM metric_definition;
SELECT vender, COUNT(*) AS cnt FROM metric_definition GROUP BY vender;
SELECT owner_subject, COUNT(*) AS cnt FROM metric_definition GROUP BY owner_subject;
SELECT value_type, COUNT(*) AS cnt FROM metric_definition GROUP BY value_type;
SELECT health_purpose, COUNT(*) AS cnt FROM metric_definition GROUP BY health_purpose;
SELECT is_veto, COUNT(*) AS cnt FROM metric_definition GROUP BY is_veto;
SELECT COUNT(*) AS has_rate_unit FROM metric_definition WHERE normal_rate_unit IS NOT NULL;
