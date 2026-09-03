-- ============================================================================
-- 知识图谱：节点画布坐标持久化
--
-- 【为什么需要】
-- 不存坐标的话，每次打开页面、每次保存节点/关系之后，都要重新跑力导向布局，
-- 运维刚整理好的图形会瞬间变回一团乱麻。而"加一条关系"恰恰是这个页面
-- 最高频的操作——不解决这个问题，图谱编辑功能实际上不可用。
--
-- 【为什么用 NULL 而不是 0】
-- (0,0) 是合法坐标。用 0 当"未布局"的哨兵值，会让真的被摆在原点的节点
-- 每次都被重新布局。用 NULL 才能干净地区分"从未摆放过"和"摆在原点"。
--
-- 本脚本幂等，可重复执行。
-- 前置：kg_node / kg_edge 已建，且已执行过 kg_fix_nullable.sql。
-- ============================================================================
SET NAMES utf8mb4;

-- ---------- 1. 加坐标列 ----------
-- MySQL 8.0.29+ 支持 ADD COLUMN IF NOT EXISTS；
-- 低版本直接去掉 IF NOT EXISTS，重复执行会报 Duplicate column，忽略即可。
ALTER TABLE `kg_node`
  ADD COLUMN IF NOT EXISTS `pos_x` DOUBLE NULL
    COMMENT '画布 X 坐标，NULL = 从未人工摆放过' AFTER `props`,
  ADD COLUMN IF NOT EXISTS `pos_y` DOUBLE NULL
    COMMENT '画布 Y 坐标，NULL = 从未人工摆放过' AFTER `pos_x`;

-- ---------- 2. 清理半截数据 ----------
-- 只有一个坐标非空是无意义的状态（前端判定条件是两者都非空），
-- 统一置回 NULL，让这类节点重新参与自动布局。
UPDATE `kg_node`
SET `pos_x` = NULL, `pos_y` = NULL
WHERE (`pos_x` IS NULL) <> (`pos_y` IS NULL);

-- ---------- 3. 自检 ----------
SELECT
  COUNT(*)                                         AS 节点总数,
  SUM(`pos_x` IS NOT NULL AND `pos_y` IS NOT NULL) AS 已固定位置,
  SUM(`pos_x` IS NULL OR  `pos_y` IS NULL)         AS 待自动布局
FROM `kg_node`;

-- ---------- 4. 运维用：重置全部布局 ----------
-- 图谱被拖乱了想推倒重来时执行，下次打开页面会重新跑一次力导向布局。
-- 平时不要执行。
-- UPDATE `kg_node` SET `pos_x` = NULL, `pos_y` = NULL;
