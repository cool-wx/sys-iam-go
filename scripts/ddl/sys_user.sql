-- sys_user 建表 DDL（Phase 1 验证载体表）
-- 来源：实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_user，2026-10-31）+ builtin 补列
--   （源 PO User.java:62-64 证实 builtin 列存在，导入 DDL 缺失；本地空表补列零风险）
-- 列集 22 列；对空库幂等（IF NOT EXISTS）；不含 DROP、不含外键（T-1-07）。

CREATE TABLE IF NOT EXISTS `sys_user` (
  `id` bigint NOT NULL,
  `username` varchar(50) DEFAULT NULL,
  `password` varchar(100) DEFAULT NULL,
  `real_name` varchar(50) DEFAULT NULL,
  `avatar` varchar(255) DEFAULT NULL,
  `gender` tinyint DEFAULT NULL,
  `phone` varchar(20) DEFAULT NULL,
  `email` varchar(100) DEFAULT NULL,
  `openid` varchar(100) DEFAULT NULL,
  `ext_token` varchar(512) DEFAULT NULL,
  `dept_id` bigint DEFAULT NULL,
  `dept_name` varchar(50) DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `first_login` tinyint(1) DEFAULT NULL,
  `builtin` tinyint(1) DEFAULT NULL,
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`),
  KEY `idx_dept_id` (`dept_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
