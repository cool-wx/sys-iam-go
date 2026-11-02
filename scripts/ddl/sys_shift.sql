-- sys_shift 建表 DDL（排班域）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_shift，2026-11-01）
-- 列集 13 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键。
-- start_time/end_time 为 MySQL TIME 列——Go 版以 HH:MM:SS 字符串直传（
-- 不加格式解析层）；无唯一索引；scripts/ddl/*.sql 由 testutil.execDDLDir 按文件名序
-- 自动加载，落目录即生效。

DROP TABLE IF EXISTS `sys_shift`;
CREATE TABLE `sys_shift` (
  `id` bigint NOT NULL,
  `shift_code` varchar(50) DEFAULT NULL,
  `shift_name` varchar(50) DEFAULT NULL,
  `start_time` time DEFAULT NULL,
  `end_time` time DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
