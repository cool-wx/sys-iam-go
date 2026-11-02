-- sys_fault_info 建表 DDL（故障域）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_fault_info，2026-11-01）
-- 列集 17 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键。
-- bit_index int DEFAULT -1、type tinyint DEFAULT 3(UNKNOWN)；name+bit_index+location
-- 三键**无唯一索引**——upsert 判重由应用层查询承担（照源 eq 语义，location
-- 为 NULL 时判重失效是源已接受行为）；scripts/ddl/*.sql 由
-- testutil.execDDLDir 按文件名序自动加载，落目录即生效。

DROP TABLE IF EXISTS `sys_fault_info`;
CREATE TABLE `sys_fault_info` (
  `id` bigint NOT NULL,
  `code` varchar(50) DEFAULT NULL,
  `name` varchar(100) DEFAULT NULL,
  `bit_index` int DEFAULT '-1',
  `type` tinyint DEFAULT '3',
  `dict_data_id` bigint DEFAULT NULL,
  `detail` varchar(500) DEFAULT NULL,
  `reason` varchar(500) DEFAULT NULL,
  `location` varchar(200) DEFAULT NULL,
  `fix` varchar(500) DEFAULT NULL,
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
