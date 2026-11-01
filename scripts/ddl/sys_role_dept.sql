-- sys_role_dept 建表 DDL（Phase 3 dataScope 五档之档位 4「指定部门」集合存储）
-- 裁决：CONTEXT.md D-10（2026-11-01 自主拍板）方案 A——sys_role_menu 同构关联表
-- （源系统无此概念：sys_role 13 列仅 data_scope、源仓库 grep 零命中，属 PROJECT.md
-- 「增量扩展」例外族，与 casbin_rule 同批；纯新增不动任何既有表，零 ALTER）。
-- 列集 8 列；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键（T-1-07）。
-- 关联表无审计人四列（DDL 实证同族表）：Go PO 不内嵌 BaseWithOperator。

DROP TABLE IF EXISTS `sys_role_dept`;
CREATE TABLE `sys_role_dept` (
  `id` bigint NOT NULL,
  `role_id` bigint DEFAULT NULL,
  `role_name` varchar(50) DEFAULT NULL,
  `dept_id` bigint DEFAULT NULL,
  `dept_name` varchar(50) DEFAULT NULL,
  `create_time` datetime DEFAULT NULL,
  `update_time` datetime DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_role_id` (`role_id`),
  KEY `idx_dept_id` (`dept_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
