-- sys_menu 建表 DDL（Phase 2 RBAC 主链路）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_menu，2026-10-31）
-- 列集 19 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键（T-1-07）。

DROP TABLE IF EXISTS `sys_menu`;
CREATE TABLE `sys_menu` (
  `id` bigint NOT NULL,
  `parent_id` bigint DEFAULT NULL,
  `parent_name` varchar(50) DEFAULT NULL,
  `name` varchar(50) DEFAULT NULL,
  `type` tinyint DEFAULT NULL,
  `path` varchar(200) DEFAULT NULL,
  `component` varchar(200) DEFAULT NULL,
  `permission` varchar(100) DEFAULT NULL,
  `icon` varchar(100) DEFAULT NULL,
  `sort` int DEFAULT NULL,
  `visible` tinyint(1) DEFAULT '1',
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
