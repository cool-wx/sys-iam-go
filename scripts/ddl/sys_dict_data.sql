-- sys_dict_data 建表 DDL（字典域）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_dict_data，2026-11-01）
-- 列集 13 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键。
-- dict_type_name 为服务端冗余列（add/edit 校验类型存在后写入）；
-- 无唯一索引——dict_data 重复插入不触发 1062；scripts/ddl/*.sql 由
-- testutil.execDDLDir 按文件名序自动加载，落目录即生效。

DROP TABLE IF EXISTS `sys_dict_data`;
CREATE TABLE `sys_dict_data` (
  `id` bigint NOT NULL,
  `dict_type_id` bigint DEFAULT NULL,
  `dict_type_name` varchar(100) DEFAULT NULL,
  `label` varchar(100) DEFAULT NULL,
  `value` varchar(100) DEFAULT NULL,
  `sort` int DEFAULT NULL,
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_dict_type_id` (`dict_type_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
