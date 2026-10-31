-- sys_api_key 建表 DDL（Phase 2 RBAC 主链路）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_api_key，2026-10-31）
-- 列集 13 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键（T-1-07）。
-- api_key 明文存储为源语义（T-2-06 accept）：getByApiKey 精确匹配，明文仅 add 响应一次性返回。

DROP TABLE IF EXISTS `sys_api_key`;
CREATE TABLE `sys_api_key` (
  `id` bigint NOT NULL,
  `api_key` varchar(100) DEFAULT NULL,
  `username` varchar(50) DEFAULT NULL,
  `user_id` bigint DEFAULT NULL,
  `expire_time` datetime DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_api_key` (`api_key`),
  KEY `idx_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
