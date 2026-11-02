-- casbin_rule 建表 DDL（原生策略表，gorm-adapter v3.35.0 持久化层）
-- 来源：03-RESEARCH probe 实测版（列集对齐 gorm-adapter v3.35.0 CasbinRule struct，adapter.go:48-61）
-- 列集 8 列（id 自增 + ptype + v0..v5）；幂等（DROP TABLE IF EXISTS + CREATE TABLE）；不含外键。
-- 唯一索引必须 DDL 自带：TurnOffAutoMigrate 后 adapter 不建表也不建索引（adapter.go:410-417 实证）。
-- id 自增是全库唯一例外（adapter 内部管理；业务代码零直写本表——仅经 enforcer API）。

DROP TABLE IF EXISTS `casbin_rule`;
CREATE TABLE `casbin_rule` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `ptype` varchar(100) DEFAULT NULL,
  `v0` varchar(100) DEFAULT NULL,
  `v1` varchar(100) DEFAULT NULL,
  `v2` varchar(100) DEFAULT NULL,
  `v3` varchar(100) DEFAULT NULL,
  `v4` varchar(100) DEFAULT NULL,
  `v5` varchar(100) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_casbin_rule` (`ptype`,`v0`,`v1`,`v2`,`v3`,`v4`,`v5`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
