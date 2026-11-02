-- sys_operation_log 建表 DDL（操作日志域）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_operation_log，2026-11-01）
-- 列集 14 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键。
-- 注意：无 create_by_id/create_by_name/update_by_id/update_by_name 四审计列
-- （实体 extends BaseAggregate 非 BaseUserEntity）——PO 不内嵌 BaseWithOperator；
-- params/error_msg 为 TEXT 列；scripts/ddl/*.sql 由 testutil.execDDLDir 按文件名序
-- 自动加载，落目录即生效。

DROP TABLE IF EXISTS `sys_operation_log`;
CREATE TABLE `sys_operation_log` (
  `id` bigint NOT NULL,
  `user_id` bigint DEFAULT NULL,
  `real_name` varchar(50) DEFAULT NULL,
  `module` varchar(100) DEFAULT NULL,
  `operation` varchar(200) DEFAULT NULL,
  `method` varchar(200) DEFAULT NULL,
  `params` text,
  `ip` varchar(50) DEFAULT NULL,
  `duration` int DEFAULT NULL,
  `status` tinyint DEFAULT NULL,
  `error_msg` text,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
