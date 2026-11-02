-- sys_role_menu 建表 DDL（RBAC 主链路）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_role_menu，2026-10-31）
-- 列集 8 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键。
-- 关联表无审计人四列（DDL 实证）：Go PO 不内嵌 BaseWithOperator（02-RESEARCH 研究重点 2）。

DROP TABLE IF EXISTS `sys_role_menu`;
CREATE TABLE `sys_role_menu` (
  `id` bigint NOT NULL,
  `role_id` bigint DEFAULT NULL,
  `role_name` varchar(50) DEFAULT NULL,
  `menu_id` bigint DEFAULT NULL,
  `menu_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_role_id` (`role_id`),
  KEY `idx_menu_id` (`menu_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
