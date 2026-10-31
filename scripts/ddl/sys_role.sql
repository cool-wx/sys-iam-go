-- sys_role 建表 DDL（Phase 2 RBAC 主链路）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_role，2026-10-31）
-- 列集 13 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键（T-1-07）。
-- utf8mb4_general_ci：role_code 唯一索引大小写不敏感（DB 决定，Go 侧不处理）。

DROP TABLE IF EXISTS `sys_role`;
CREATE TABLE `sys_role` (
  `id` bigint NOT NULL,
  `role_name` varchar(50) DEFAULT NULL,
  `role_code` varchar(50) DEFAULT NULL,
  `data_scope` tinyint DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `builtin` tinyint(1) DEFAULT '0',
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_role_code` (`role_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
