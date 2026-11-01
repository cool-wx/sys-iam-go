-- sys_config 建表 DDL（Phase 4 系统配置域）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_config，2026-11-01）
-- 列集 11 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键（T-1-07）。
-- uk_config_key 唯一索引是 CONF-01 判重语义的 DB 保障（1062 → 400/10400 翻译）；
-- scripts/ddl/*.sql 由 testutil.execDDLDir 按文件名序自动加载，落目录即生效。

DROP TABLE IF EXISTS `sys_config`;
CREATE TABLE `sys_config` (
  `id` bigint NOT NULL,
  `config_key` varchar(100) DEFAULT NULL,
  `config_value` varchar(500) DEFAULT NULL,
  `config_type` varchar(50) DEFAULT NULL,
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_config_key` (`config_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
