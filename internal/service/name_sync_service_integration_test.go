//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/cool-wx/sys-iam-go/internal/event"
	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/internal/testutil"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// startServiceDB 发布条件单测专用容器（per-test 容器沿 user_integration_test.go
// 直构造形态：一个测试函数一个 MySQL 容器，域内四形态共用、实体间互不冲突；
// TranslateError 对齐 repo 1062→ErrDuplicatedKey 依赖面）。
func startServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()
	dsn, stopContainer, err := testutil.RunMySQL(ctx)
	require.NoError(t, err, "mysql testcontainer 启动与 DDL 引导失败")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		TranslateError: true,
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		stopContainer()
	})
	return db
}

// ---- role 域四形态 ----

// TestNameChangePublishRole role 改名发布条件四形态（T-5-04 缓解单测面，源
// RoleApplicationService.editRole 条件 `roleName != null && !roleName.equals(oldName)`
// 逐字移植的行为锁定）：(a) 改名 → 恰 1 条事件且载荷=DB 旧行+请求新值；(b) 仅改
// remark → 零发布；(c) 新名==旧行名 → 零发布；(d) 目标不存在 → 404 且零发布。
func TestNameChangePublishRole(t *testing.T) {
	ctx := context.Background()
	db := startServiceDB(t)
	pub := &fakePublisher{}
	svc := service.NewRoleService(db, repository.NewRoleRepo(db), repository.NewMenuRepo(db),
		repository.NewRelationRepo(db), repository.NewDeptRepo(db), nil, pub)

	created, err := svc.Create(ctx, &dto.RoleCreateReq{RoleName: strPtr("巡检员"), RoleCode: strPtr("inspector")})
	require.NoError(t, err, "seed 角色失败")
	id := int64(created.ID)
	require.Empty(t, pub.events, "Create 不发布任何事件")

	// (a) 改名请求 → 恰 1 条事件，Type/ID/OldName/NewName 与 DB 旧行及请求一致
	_, err = svc.Update(ctx, id, &dto.RoleUpdateReq{RoleName: strPtr("巡检员乙")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "改名请求必须发布恰 1 条事件")
	assert.Equal(t, event.RoleNameChanged, pub.events[0].Type)
	assert.Equal(t, id, pub.events[0].ID)
	assert.Equal(t, "巡检员", pub.events[0].OldName, "OldName 必须取 DB 旧行值")
	assert.Equal(t, "巡检员乙", pub.events[0].NewName, "NewName 必须取请求新值")

	// (b) 仅改 remark → 零发布（防无条件发布——否则仅改备注也触发三表冗余同步）
	_, err = svc.Update(ctx, id, &dto.RoleUpdateReq{Remark: strPtr("只改备注")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "仅改 remark 不得新增事件")

	// (c) 新名==旧行名 → 零发布（源按值变化检测，同名提交非改名）
	_, err = svc.Update(ctx, id, &dto.RoleUpdateReq{RoleName: strPtr("巡检员乙")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "新名==旧行名不得新增事件")

	// (d) 目标不存在 → 404 域错误且零发布（回读提前拦，Patch 与发布都不触发）
	_, err = svc.Update(ctx, 990001, &dto.RoleUpdateReq{RoleName: strPtr("幽灵角色")})
	require.Error(t, err)
	var ec *errcode.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, errcode.CodeNotFound, ec.Code, "不存在目标必须译 404/10300")
	assert.Contains(t, ec.Msg, "角色不存在")
	require.Len(t, pub.events, 1, "404 路径不得新增事件")
}

// ---- menu 域四形态 ----

// TestNameChangePublishMenu menu 改名发布条件四形态（源 MenuApplicationService.editMenu
// 条件 `menu.getName() != null && !menu.getName().equals(oldName)` 的行为锁定）。
func TestNameChangePublishMenu(t *testing.T) {
	ctx := context.Background()
	db := startServiceDB(t)
	pub := &fakePublisher{}
	svc := service.NewMenuService(repository.NewMenuRepo(db), pub)

	mt := types.MenuDir
	created, err := svc.Create(ctx, &dto.MenuCreateReq{Name: strPtr("系统管理"), Type: &mt, Path: strPtr("/sys")})
	require.NoError(t, err, "seed 菜单失败")
	id := int64(created.ID)
	require.Empty(t, pub.events, "Create 不发布任何事件")

	// (a) 改名请求 → 恰 1 条事件，载荷与 DB 旧行及请求一致
	_, err = svc.Update(ctx, id, &dto.MenuUpdateReq{Name: strPtr("系统中心")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, event.MenuNameChanged, pub.events[0].Type)
	assert.Equal(t, id, pub.events[0].ID)
	assert.Equal(t, "系统管理", pub.events[0].OldName)
	assert.Equal(t, "系统中心", pub.events[0].NewName)

	// (b) 仅改 remark → 零发布
	_, err = svc.Update(ctx, id, &dto.MenuUpdateReq{Remark: strPtr("只改备注")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "仅改 remark 不得新增事件")

	// (c) 新名==旧行名 → 零发布
	_, err = svc.Update(ctx, id, &dto.MenuUpdateReq{Name: strPtr("系统中心")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "新名==旧行名不得新增事件")

	// (d) 目标不存在 → 404 且零发布
	_, err = svc.Update(ctx, 990002, &dto.MenuUpdateReq{Name: strPtr("幽灵菜单")})
	require.Error(t, err)
	var ec *errcode.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, errcode.CodeNotFound, ec.Code)
	assert.Contains(t, ec.Msg, "菜单不存在")
	require.Len(t, pub.events, 1, "404 路径不得新增事件")
}

// ---- dictType 域四形态 ----

// TestNameChangePublishDictType dictType 改名发布条件四形态（源
// DictApplicationService.editDictType 条件 `updated && !Objects.equals(oldName, newName)
// && newName != null` 的行为锁定——updated 前置在 Go 形态下由 Patch 返回 nil 等价成立）。
func TestNameChangePublishDictType(t *testing.T) {
	ctx := context.Background()
	db := startServiceDB(t)
	pub := &fakePublisher{}
	svc := service.NewDictService(repository.NewDictTypeRepo(db), repository.NewDictDataRepo(db), pub)

	created, err := svc.CreateDictType(ctx, &dto.DictTypeCreateReq{DictName: strPtr("故障类型"), DictType: strPtr("fault_kind")})
	require.NoError(t, err, "seed 字典类型失败")
	id := int64(created.ID)
	require.Empty(t, pub.events, "Create 不发布任何事件")

	// (a) 改名请求 → 恰 1 条事件，载荷与 DB 旧行及请求一致
	_, err = svc.UpdateDictType(ctx, id, &dto.DictTypeUpdateReq{DictName: strPtr("故障类别")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, event.DictTypeNameChanged, pub.events[0].Type)
	assert.Equal(t, id, pub.events[0].ID)
	assert.Equal(t, "故障类型", pub.events[0].OldName)
	assert.Equal(t, "故障类别", pub.events[0].NewName)

	// (b) 仅改 remark → 零发布
	_, err = svc.UpdateDictType(ctx, id, &dto.DictTypeUpdateReq{Remark: strPtr("只改备注")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "仅改 remark 不得新增事件")

	// (c) 新名==旧行名 → 零发布
	_, err = svc.UpdateDictType(ctx, id, &dto.DictTypeUpdateReq{DictName: strPtr("故障类别")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "新名==旧行名不得新增事件")

	// (d) 目标不存在 → 404 且零发布
	_, err = svc.UpdateDictType(ctx, 990003, &dto.DictTypeUpdateReq{DictName: strPtr("幽灵类型")})
	require.Error(t, err)
	var ec *errcode.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, errcode.CodeNotFound, ec.Code)
	assert.Contains(t, ec.Msg, "字典类型不存在")
	require.Len(t, pub.events, 1, "404 路径不得新增事件")
}
// ==== Task 2 RED:追加到 internal/service/name_sync_service_integration_test.go 的内容 ====

// ---- user 域四形态（realName 发布条件，05-02 Task 2）----

// TestNameChangePublishUserRealName user 改真名发布条件四形态（源
// UserApplicationService.editUser 条件 `realName != null && !realName.equals(oldRealName)`
// 的行为锁定；与 role/menu/dict 三域测试同构）。
func TestNameChangePublishUserRealName(t *testing.T) {
	ctx := context.Background()
	db := startServiceDB(t)
	pub := &fakePublisher{}
	svc := service.NewUserService(db, repository.NewUserRepo(db), repository.NewRelationRepo(db),
		repository.NewRoleRepo(db), repository.NewDeptRepo(db), nil, nil, pub)

	// Create 强制部门校验链：先 seed 部门再建用户
	require.NoError(t, db.Exec(
		"INSERT INTO sys_dept (id, dept_name, sort, status, create_time, update_time) VALUES (100, '真名部', 0, 1, NOW(), NOW())",
	).Error)
	deptID := ids.ID(100)
	created, err := svc.Create(ctx, &dto.UserCreateReq{
		Username: strPtr("rn-publisher"),
		Password: strPtr("pw-123456"),
		RealName: strPtr("旧真名"),
		DeptID:   &deptID,
	})
	require.NoError(t, err, "seed 用户失败")
	id := int64(created.ID)
	require.Empty(t, pub.events, "Create 不发布任何事件")

	// (a) 改真名请求 → 恰 1 条事件，载荷与 DB 旧行及请求一致
	_, err = svc.Update(ctx, id, &dto.UpdateUserReq{RealName: strPtr("新真名")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "改真名请求必须发布恰 1 条事件")
	assert.Equal(t, event.RealNameChanged, pub.events[0].Type)
	assert.Equal(t, id, pub.events[0].ID)
	assert.Equal(t, "旧真名", pub.events[0].OldName, "OldName 必须取 DB 旧行值")
	assert.Equal(t, "新真名", pub.events[0].NewName, "NewName 必须取请求新值")

	// (b) 仅改 phone → 零发布（防无条件发布——否则仅改手机号也触发 26 条 UPDATE）
	_, err = svc.Update(ctx, id, &dto.UpdateUserReq{Phone: strPtr("13900000000")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "仅改 phone 不得新增事件")

	// (c) 新真名==旧真名 → 零发布
	_, err = svc.Update(ctx, id, &dto.UpdateUserReq{RealName: strPtr("新真名")})
	require.NoError(t, err)
	require.Len(t, pub.events, 1, "新真名==旧真名不得新增事件")

	// (d) 目标不存在 → 404 且零发布（回读提前拦，Patch 与发布都不触发）
	_, err = svc.Update(ctx, 990004, &dto.UpdateUserReq{RealName: strPtr("幽灵真名")})
	require.Error(t, err)
	var ec *errcode.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, errcode.CodeNotFound, ec.Code)
	assert.Contains(t, ec.Msg, "用户不存在")
	require.Len(t, pub.events, 1, "404 路径不得新增事件")
}
