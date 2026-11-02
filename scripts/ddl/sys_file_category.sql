-- sys_file_category 建表 DDL（文件分类域）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_file_category，2026-11-01）
-- 列集 15 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键。
-- 树形结构（parent_id/parent_name 冗余列）；scripts/ddl/*.sql 由
-- testutil.execDDLDir 按文件名序自动加载，落目录即生效。

DROP TABLE IF EXISTS `sys_file_category`;
CREATE TABLE `sys_file_category` (
  `id` bigint NOT NULL,
  `parent_id` bigint DEFAULT NULL,
  `parent_name` varchar(50) DEFAULT NULL,
  `category_name` varchar(100) DEFAULT NULL,
  `category_type` int DEFAULT NULL,
  `sort` int DEFAULT NULL,
  `icon` varchar(100) DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_parent_id` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
