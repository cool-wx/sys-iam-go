// Package datascope 提供 dataScope 五档行过滤通用件（PERM-03，D-09/D-12）：
// service 层解析档位（identity→deptID→角色档位聚合→子树/自定义部门集合），repo
// 层经 Filter.Apply 注入 WHERE（GORM 闭包组合，不侵入既有查询代码——D-09）。
// internal 定位（依赖 po 内部类型）；table 参数化设计使 Phase 4 各域可直接复用
// （无 user 域特化耦合——调用方传自己的表名常量）。
//
// 档位值域（D-10，沿源 sys_role.data_scope）——**数字越大越宽**：
//
//	1 本人（create_by_id = uid）→ 2 本部门（dept_id = user.deptId）→
//	3 本部门及以下（dept_id IN 子树 ID 集）→ 4 指定部门（dept_id IN
//	sys_role_dept 集合）→ 5 全部（无过滤）。
//
// 多角色档位聚合取 max(data_scope)（D-11 取宽）；无有效角色 → ModeSelf 最窄档
// fail-closed（T-3-07：绝不默认全量）。注意：03-PATTERNS.md 第 8 节「取 min 数值
// 即最宽档（数字越小权限越宽）」为 pattern-mapper 笔误，与本包值域定义直接矛盾，
// 经 03-02 规划裁决记录否决——以 CONTEXT D-10/D-11 为准。
//
// 安全纪律（T-3-06/T-3-07）：过滤列名硬编码（table 参数为代码常量调用，仅
// create_by_id/dept_id 两列白名单），全部值占位符参数化，无任何字符串拼接 SQL 面
// （T-1-03）；未知档与空部门集一律落 "1 = 0" 空结果，绝不 fail-open。
package datascope

import (
	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// Mode 数据权限档位（源 sys_role.data_scope 值域 1..5，D-10；数字越大越宽）。
type Mode int8

const (
	// ModeSelf 本人档：仅 create_by_id = 当前用户的行（最窄档，fail-closed 兜底值）。
	ModeSelf Mode = 1
	// ModeDept 本部门档：dept_id = 用户所属部门（单元素 DeptIDs）。
	ModeDept Mode = 2
	// ModeDeptSubtree 本部门及以下档：dept_id IN 子树 ID 集（CollectSubtreeIDs 展开）。
	ModeDeptSubtree Mode = 3
	// ModeCustomDepts 指定部门档：dept_id IN sys_role_dept 配置集合（可多元素）。
	ModeCustomDepts Mode = 4
	// ModeAll 全部档：无过滤（最宽档）。
	ModeAll Mode = 5
)

// Filter 是五档解析产物：service 层先解析（档位聚合 + 部门集装配），repo 层
// Apply 注入 WHERE。零值 Filter（Mode=0）经 Apply 落 default 分支 "1 = 0"——
// 未解析即未授权（fail-closed，身份缺失/查询者行缺失场景的防御形态）。
type Filter struct {
	Mode    Mode
	UserID  int64   // 档位 1 用（本人 uid）
	DeptIDs []int64 // 档位 2 单元素 / 3、4 多元素
}

// Apply 将档位过滤注入查询（GORM 闭包组合形态，D-09）：调用方在 repo 的
// Count/Find 共用闭包内统一 Apply，保证 total 与行集出自同一过滤闭包（P15 防
// total 漂移）。table 为调用方表名**代码常量**（如 "sys_user"）——列名白名单
// 由本包硬编码（仅 create_by_id/dept_id），值全占位符参数化（T-3-06/T-1-03）。
//
// 五档分派 + fail-closed 三兜底（T-3-07）：ModeAll 原样返回（不加条件）；未知档
// （Mode 零值/域外值）落 default "1 = 0"；ModeDept/ModeDeptSubtree/ModeCustomDepts
// 三 case 首行各自显式 len 守卫——空部门集（查询者无部门/无档 4 配置）落 "1 = 0"
// 空结果。守卫必须在 case 内首行：ModeDept 的 DeptIDs[0] 直接索引在空切片上
// panic，守卫位置是正确性要求非风格。
func (f Filter) Apply(db *gorm.DB, table string) *gorm.DB {
	switch f.Mode {
	case ModeAll:
		return db // 档 5 全部：无过滤
	case ModeSelf:
		return db.Where(table+".create_by_id = ?", f.UserID)
	case ModeDept:
		if len(f.DeptIDs) == 0 {
			return db.Where("1 = 0")
		}
		return db.Where(table+".dept_id = ?", f.DeptIDs[0])
	case ModeDeptSubtree:
		if len(f.DeptIDs) == 0 {
			return db.Where("1 = 0")
		}
		return db.Where(table+".dept_id IN ?", f.DeptIDs)
	case ModeCustomDepts:
		if len(f.DeptIDs) == 0 {
			return db.Where("1 = 0")
		}
		return db.Where(table+".dept_id IN ?", f.DeptIDs)
	default:
		// 未知档（Mode 零值/域外值如 0、99）：安全默认拒绝——绝不放行为全量
		return db.Where("1 = 0")
	}
}

// Widest 档位聚合（D-11 多角色取宽）：跳过 nil 档位（NULL data_scope 的角色不
// 贡献档位——存量行为 NULL 时不因一个 NULL 行放大权限）；无任何有效档位 →
// ModeSelf（最窄档 fail-closed：无角色/全 NULL 的已授权调用者只看得到本人创建
// 的行，绝不默认全量，T-3-07）；否则取数值最大档（数字越大越宽，D-10 值域）。
// 域外值（<1 或 >5）按 max 语义原样参与——经 Apply default 分支落 "1 = 0"
// fail-closed，与档位枚举双保险兜底（T-3-09）。
func Widest(scopes []*int8) Mode {
	widest := ModeSelf
	has := false
	for _, s := range scopes {
		if s == nil {
			continue
		}
		m := Mode(*s)
		if !has || m > widest {
			widest = m
			has = true
		}
	}
	if !has {
		return ModeSelf
	}
	return widest
}

// CollectSubtreeIDs 内存子树展开（档位 3「本部门及以下」，D-10）：depts 全量
// （DeptRepo.List 按 sort ASC, id ASC 供给）按 parentID 索引后从 rootID BFS 收集
// ——含 rootID 自身。dept 表工控机百行级，一次全量查询 + 内存展开优于递归 SQL
// （03-RESEARCH 研究重点 5 原料节）。孤儿节点（parent 指向不存在 id）不入任何
// 子树自然缺席（tree.go 已锁定语义）；visited 集防环——父子环输出有限集不死循环
// （环免疫沿 tree.go 注释纪律，T-2-09）。
func CollectSubtreeIDs(depts []po.DeptPO, rootID int64) []int64 {
	childrenOf := make(map[int64][]int64, len(depts))
	for i := range depts {
		d := &depts[i]
		if d.ParentID == nil || *d.ParentID == 0 {
			continue // 根节点不是任何节点的子节点（0 亦按根处理，源语义）
		}
		childrenOf[*d.ParentID] = append(childrenOf[*d.ParentID], d.ID)
	}
	out := make([]int64, 0, len(depts))
	visited := make(map[int64]struct{}, len(depts))
	visited[rootID] = struct{}{}
	queue := []int64{rootID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		out = append(out, cur)
		for _, child := range childrenOf[cur] {
			if _, seen := visited[child]; seen {
				continue
			}
			visited[child] = struct{}{}
			queue = append(queue, child)
		}
	}
	return out
}
