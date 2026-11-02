package datascope

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// int8Ptr 表格驱动便捷构造。
func int8Ptr(v int8) *int8 { return &v }

// TestWidest 档位聚合表格驱动（多角色取宽 + fail-closed 兜底）：全 nil →
// ModeSelf；nil 混有效档取有效档；[2,5] → 5（**非 min**——「min 即最宽」笔误
// 的方向回归锁）；空切片 → ModeSelf。
func TestWidest(t *testing.T) {
	cases := []struct {
		name   string
		scopes []*int8
		want   Mode
	}{
		{"全 nil（存量 NULL 行）→ 最窄档 fail-closed", []*int8{nil, nil}, ModeSelf},
		{"nil 混档位 3 → 取有效档 3", []*int8{nil, int8Ptr(3)}, ModeDeptSubtree},
		{"档位 2 与 5 → 取 max=5（数字越大越宽）", []*int8{int8Ptr(2), int8Ptr(5)}, ModeAll},
		{"空切片（无角色）→ ModeSelf fail-closed", []*int8{}, ModeSelf},
		{"单档位 4 原样", []*int8{int8Ptr(4)}, ModeCustomDepts},
		{"nil 单独 → ModeSelf", []*int8{nil}, ModeSelf},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Widest(tc.scopes))
		})
	}
}

// deptNode 子树展开 fixture 节点构造。
func deptNode(id int64, parentID *int64) po.DeptPO {
	return po.DeptPO{ID: id, ParentID: parentID}
}

func pidOf(v int64) *int64 { return &v }

// TestCollectSubtreeIDs 三层树含根自身：D1←D1a←D1a1 全集收集（含 rootID 自身），
// 独立根 D2 不混入。
func TestCollectSubtreeIDsThreeLayers(t *testing.T) {
	depts := []po.DeptPO{
		deptNode(1, nil),
		deptNode(11, pidOf(1)),
		deptNode(111, pidOf(11)),
		deptNode(2, nil), // 独立根
		deptNode(21, pidOf(2)),
	}
	got := CollectSubtreeIDs(depts, 1)
	assert.ElementsMatch(t, []int64{1, 11, 111}, got, "子树必须含根自身与全部后代，不含 D2 分支")
	got = CollectSubtreeIDs(depts, 111)
	assert.Equal(t, []int64{111}, got, "叶子根自身单元素")
}

// TestCollectSubtreeIDsOrphanAbsent 孤儿节点（parent 指向不存在 id）不入任何子树
// 自然缺席（tree.go 已锁定语义——datascope 只认从 rootID 向下的可达闭包）。
func TestCollectSubtreeIDsOrphanAbsent(t *testing.T) {
	depts := []po.DeptPO{
		deptNode(1, nil),
		deptNode(11, pidOf(1)),
		deptNode(99, pidOf(999)), // 孤儿：999 不存在
	}
	got := CollectSubtreeIDs(depts, 1)
	assert.ElementsMatch(t, []int64{1, 11}, got, "孤儿 99 必须缺席（999 不可达）")
}

// TestCollectSubtreeIDsCycleFinite 父子环（A.parent=B、B.parent=A）不死循环且
// 输出有限集：visited 集防环直证（2s 超时纪律照 tree_test）。
func TestCollectSubtreeIDsCycleFinite(t *testing.T) {
	depts := []po.DeptPO{
		deptNode(10, pidOf(11)), // 环 A
		deptNode(11, pidOf(10)), // 环 B
		deptNode(1, nil),
	}
	done := make(chan []int64, 1)
	go func() { done <- CollectSubtreeIDs(depts, 10) }()
	var got []int64
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CollectSubtreeIDs 在环引用 fixture 上未于 2s 内返回——死循环回归")
	}
	assert.ElementsMatch(t, []int64{10, 11}, got, "环双方均属同一闭包，输出有限集")
}

// newDryRunDB 构造免真连接的 gorm DryRun 会话工厂：mysql dialector +
// SkipInitializeWithVersion（无版本探测查询）+ DisableAutomaticPing（gorm.Open
// 默认 Ping 一次——不关则免容器断言必挂）+ 不可达 DSN（DryRun 只构建 SQL 不执行，
// 绝不触碰连接）。
func newDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "dryrun:dryrun@tcp(127.0.0.1:1)/dryrun",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)
	return db
}

// buildSQL 在 DryRun 会话上执行 Apply + Find，返回构建出的 WHERE 全句。
func buildSQL(t *testing.T, db *gorm.DB, f Filter, table string) string {
	t.Helper()
	q := f.Apply(db.Session(&gorm.Session{DryRun: true}).Model(&po.UserPO{}), table)
	var users []po.UserPO
	stmt := q.Find(&users)
	require.NoError(t, stmt.Error, "DryRun SQL 构建不应报错")
	return stmt.Statement.SQL.String()
}

// TestFilterApplyClauses 五档 SQL 子句断言：
//   - 档 1 占位符形态（table 限定列 create_by_id = ?）；
//   - 档 2 单元素占位符、档 3/4 IN 子句且占位符数与部门集一致；
//   - 档 5 无 WHERE（原样返回，零条件）；
//   - 未知档（Mode 0 与 99）均落 "1 = 0" 空结果（fail-closed，绝不放行全量）；
//   - Mode 2/3/4 空 DeptIDs 三案例均落 "1 = 0"（case 内首行 len 守卫直证；
//     Mode 2 空集不 panic——守卫在位的运行直证，缺守卫时 DeptIDs[0] 直接越界）。
func TestFilterApplyClauses(t *testing.T) {
	db := newDryRunDB(t)

	// 档 1 本人：table 限定列 + 占位符
	sql := buildSQL(t, db, Filter{Mode: ModeSelf, UserID: 42}, "sys_user")
	assert.Contains(t, sql, "sys_user.create_by_id = ?", "档 1 必须 create_by_id 占位符: %s", sql)
	assert.NotContains(t, sql, "42", "值必须走占位符不内联: %s", sql)

	// 档 2 本部门：单元素占位符
	sql = buildSQL(t, db, Filter{Mode: ModeDept, DeptIDs: []int64{7}}, "sys_user")
	assert.Contains(t, sql, "sys_user.dept_id = ?", "档 2 必须 dept_id 占位符: %s", sql)

	// 档 3 子树 / 档 4 指定部门：IN 子句 + 占位符逐元素
	for _, tc := range []struct {
		name string
		f    Filter
	}{{"档 3", Filter{Mode: ModeDeptSubtree, DeptIDs: []int64{1, 11, 111}}},
		{"档 4", Filter{Mode: ModeCustomDepts, DeptIDs: []int64{2, 21}}}} {
		sql := buildSQL(t, db, tc.f, "sys_user")
		assert.Contains(t, sql, "sys_user.dept_id IN", tc.name+" 必须 IN 子句: %s", sql)
		for _, id := range tc.f.DeptIDs {
			assert.NotContains(t, sql, strconv.FormatInt(id, 10), tc.name+" 值必须走占位符不内联: %s", sql)
		}
	}

	// 档 5 全部：零 WHERE（原样返回）
	sql = buildSQL(t, db, Filter{Mode: ModeAll}, "sys_user")
	assert.NotContains(t, sql, "WHERE", "档 5 不得附加任何条件: %s", sql)

	// 未知档 fail-closed：Mode 零值与域外值 99
	for _, mode := range []Mode{0, 99} {
		sql := buildSQL(t, db, Filter{Mode: mode, UserID: 1, DeptIDs: []int64{1}}, "sys_user")
		assert.Contains(t, sql, "1 = 0", "未知档 "+strconv.Itoa(int(mode))+" 必须落空结果: %s", sql)
	}

	// 空 DeptIDs 三案例 fail-closed（含 Mode 2 空集不 panic——本断言执行完成即直证）
	for _, mode := range []Mode{ModeDept, ModeDeptSubtree, ModeCustomDepts} {
		sql := buildSQL(t, db, Filter{Mode: mode, DeptIDs: nil}, "sys_user")
		assert.True(t, strings.Contains(sql, "1 = 0"), "档 "+strconv.Itoa(int(mode))+" 空 DeptIDs 必须落空结果: %s", sql)
	}

	// table 参数化：换表名时列限定跟随（各域复用契约直证）
	sql = buildSQL(t, db, Filter{Mode: ModeSelf, UserID: 1}, "sys_order")
	assert.Contains(t, sql, "sys_order.create_by_id = ?", "table 参数必须进入列限定: %s", sql)
}
