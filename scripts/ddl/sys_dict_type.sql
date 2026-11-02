-- sys_dict_type 建表 DDL（字典域）
-- 来源：本机容器实测真实 DDL（SHOW CREATE TABLE sys_iam.sys_dict_type，2026-11-01）
-- 列集 11 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键。
-- uk_dict_type 唯一索引是判重语义的 DB 保障（1062 → 400/10400 翻译）；
-- scripts/ddl/*.sql 由 testutil.execDDLDir 按文件名序自动加载，落目录即生效。

DROP TABLE IF EXISTS `sys_dict_type`;
CREATE TABLE `sys_dict_type` (
  `id` bigint NOT NULL,
  `dict_name` varchar(100) DEFAULT NULL,
  `dict_type` varchar(100) DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `create_by_id` bigint DEFAULT NULL,
  `create_by_name` varchar(50) DEFAULT NULL,
  `update_by_id` bigint DEFAULT NULL,
  `update_by_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_dict_type` (`dict_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
