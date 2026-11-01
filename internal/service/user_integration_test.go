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

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/model/dto"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/internal/testutil"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// strPtr 返回字符串指针。
func strPtr(s string) *string { return &s }

// TestUpdateAuditWithIdentity ctx 注入 Identity 后 Update：DB 行 update_by_id/
// update_by_name 必须等于该 identity（ENG-02 update 侧"有 identity"分支的行级证明；
// "无 identity"分支的行级断言在 handler 测试 TestUpdateUserZeroValueProtection）。
func TestUpdateAuditWithIdentity(t *testing.T) {
	ctx := context.Background()
	dsn, stopContainer, err := testutil.RunMySQL(ctx)
	require.NoError(t, err, "mysql testcontainer 启动与 DDL 引导失败")

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		TranslateError: true,
	})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
		stopContainer()
	})

	repo := repository.NewUserRepo(db)
	// 02-03 起 NewUserService 扩依赖（db+relation/role/dept repo+sm）；本测试仅走
	// Create/Update 路径不触达 sm（会话管理器），传 nil 即可（session 依赖无 Redis 环境）。
	svc := service.NewUserService(db, repo,
		repository.NewRelationRepo(db), repository.NewRoleRepo(db), repository.NewDeptRepo(db), nil)

	// create（无 identity 上下文——create_by 保持 NULL，不影响本断言目标）。
	// 02-03 起 Create 强制部门校验链（deptId 必填且必须存在）——先 seed 部门再挂载。
	require.NoError(t, db.Exec(
		"INSERT INTO sys_dept (id, dept_name, sort, status, create_time, update_time) VALUES (100, '审计部', 0, 1, NOW(), NOW())",
	).Error)
	deptID := ids.ID(100)
	created, err := svc.Create(context.Background(), &dto.UserCreateReq{
		Username: strPtr("audit-identity"),
		Password: strPtr("pw-123456"),
		RealName: strPtr("审计更新"),
		DeptID:   &deptID,
	})
	require.NoError(t, err, "service.Create 失败")

	// ctx 注入 identity 后 Update
	opCtx := auth.WithIdentity(context.Background(), auth.Identity{UserID: 42, Name: "操作者甲"})
	_, err = svc.Update(opCtx, int64(created.ID), &dto.UpdateUserReq{RealName: strPtr("更新后的名")})
	require.NoError(t, err, "service.Update 失败")

	// DB 行核对：update_by 两列等于注入 identity
	var row struct {
		UpdateByID   *int64
		UpdateByName *string
		RealName     *string
	}
	require.NoError(t, db.Raw(
		"SELECT update_by_id, update_by_name, real_name FROM sys_user WHERE id = ?",
		int64(created.ID),
	).Scan(&row).Error)
	require.NotNil(t, row.UpdateByID, "identity 存在时 update_by_id 必须落库")
	assert.Equal(t, int64(42), *row.UpdateByID)
	require.NotNil(t, row.UpdateByName)
	assert.Equal(t, "操作者甲", *row.UpdateByName)
	require.NotNil(t, row.RealName)
	assert.Equal(t, "更新后的名", *row.RealName)
}
