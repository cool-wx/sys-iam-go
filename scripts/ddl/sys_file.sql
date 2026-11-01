-- sys_file 建表 DDL（Phase 4 文件域；注意表名是 sys_file 非 sys_file_info）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_file，2026-11-01）
-- 列集 16 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键（T-1-07）。
-- file_hash 无唯一索引——判重由应用层 categoryId IS NULL 双分支查询承担（FILE-01，
-- 唯一索引对 NULL 互不相等）；scripts/ddl/*.sql 由 testutil.execDDLDir 按文件名序
-- 自动加载，落目录即生效。

DROP TABLE IF EXISTS `sys_file`;
CREATE TABLE `sys_file` (
  `id` bigint NOT NULL,
  `category_id` bigint DEFAULT NULL,
  `category_name` varchar(100) DEFAULT NULL,
  `file_name` varchar(255) DEFAULT NULL,
  `file_path` varchar(500) DEFAULT NULL,
  `file_type` varchar(50) DEFAULT NULL,
  `file_size` bigint DEFAULT NULL,
  `file_hash` varchar(32) DEFAULT NULL,
  `description` varchar(500) DEFAULT NULL,
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_category_id` (`category_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
