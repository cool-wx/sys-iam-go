// sys_fault_info 数据访问（照 repository/menu.go 模板；list 分页 + code 索引列
// eq 直传过滤 + 三键判重查询 FAUL-02）。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
)

// FaultInfoRepo sys_fault_info 数据访问。
type FaultInfoRepo struct {
	db *gorm.DB
}

// NewFaultInfoRepo 构造（db 可为 *gorm.DB 或事务 tx——Excel 导入整批事务内
// 以 tx 构造变体复用同一查询逻辑，04-04 Task 2 依赖）。
func NewFaultInfoRepo(db *gorm.DB) *FaultInfoRepo {
	return &FaultInfoRepo{db: db}
}

// Create 插入一行；ctx 经 WithContext 传递，供审计钩子读取 identity。
// 参数化由 GORM 占位符保证，不拼 SQL 字符串（T-1-03）。
func (r *FaultInfoRepo) Create(ctx context.Context, f *po.FaultInfoPO) error {
	return r.db.WithContext(ctx).Create(f).Error
}

// Patch 显式列更新（零值保护核心，ENG-05；CR-01 回读确认语义与 MenuRepo.Patch
// 一致）：只写 cols 中出现的列（白名单由调用方 service 层收敛，T-1-10）；恒带
// Where("id = ?") 防全表更新（T-1-08）。update_time 与审计操作人列在此显式并入
// map——GORM 对 Updates(map) 不写钩子改动的结构体字段（已知语义坑），显式并入
// 保证确定性。绝不在更新路径调用 Save（Pitfall 3）。
func (r *FaultInfoRepo) Patch(ctx context.Context, id int64, cols map[string]any) error {
	if cols == nil {
		cols = map[string]any{}
	}
	now := time.Now()
	cols["update_time"] = now
	if identity, ok := auth.FromContext(ctx); ok {
		cols["update_by_id"] = identity.UserID
		cols["update_by_name"] = identity.Name
	}
	res := r.db.WithContext(ctx).Model(&po.FaultInfoPO{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 0 变更 ≠ 不存在：同秒重复提交相同内容时 SET 值全等 → 0 行变更但行存在，
		// 需 Count 二次确认（CR-01），仅当行确实不存在才返回 ErrNotFound
		var n int64
		if err := r.db.WithContext(ctx).Model(&po.FaultInfoPO{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errcode.ErrNotFound
		}
	}
	return nil
}

// Delete 物理删除（硬删除语义，P4）。与源一致不校验存在性：删除不存在的 id
// 静默成功（源 removeById 语义）。
func (r *FaultInfoRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.FaultInfoPO{}).Error
}

// FindByID 按主键查询；未命中返回 ErrNotFound（10300）。
func (r *FaultInfoRepo) FindByID(ctx context.Context, id int64) (*po.FaultInfoPO, error) {
	var f po.FaultInfoPO
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// FindByThreeKeys FAUL-02 三键判重查询（源 FaultInfoRepositoryImpl:58-63
// `eq(name).eq(bitIndex).eq(location)` 蓝本逐字语义）：三列 eq 直传参数，LIMIT 1。
// 未命中返回 (nil, nil)——"无重复"非错误。
//
// ── Q2 照源裁定（04-04 prohibition 锁定，勿「顺手修复」）──
// location 为 nil 时 database/sql 把指针绑定为 NULL，生成 `location = NULL`
// 恒为 false——带 NULL 位置的行永远判不中重复、反复 INSERT。这是源系统已接受
// 的既有行为（研究期 Open Question Q2 已拍板照源 eq 语义保守移植：本机
// sys_fault_info 0 行无存量影响；04-RESEARCH 重点 1 三键查询 NULL 边界陷阱）。
// **禁止**改成 file 域式 IS NULL 双分支（FindByCategoryAndHash）——那是对源
// 行为的有意偏离；语义锁定由 TestFaultImportLocationNullNoDedup 断言
// （同三键 NULL 位置二次导入 → 两行）。
func (r *FaultInfoRepo) FindByThreeKeys(ctx context.Context, name string, bitIndex *int, location *string) (*po.FaultInfoPO, error) {
	var f po.FaultInfoPO
	err := r.db.WithContext(ctx).Model(&po.FaultInfoPO{}).
		Where("name = ?", name).
		Where("bit_index = ?", bitIndex).
		Where("location = ?", location). // nil *string 绑定 NULL → location = NULL 恒不中（Q2 照源，见上注）
		Limit(1).
		First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// List 分页列表（源 /faultInfo list 语义）：page/size 已由 handler 层
// paging.Parse 钳位（1/15 全局默认——源 PageHelper 同 1/15）；code 非空时
// idx_code 索引列 eq 直传过滤（无 like 模糊——源 list 无特殊条件研究未记录
// 则不发明）；ORDER BY create_time DESC, id DESC（id tiebreaker 同秒稳定序）。
// 无 dataScope（D-03 零应用）。
func (r *FaultInfoRepo) List(ctx context.Context, page, size int, code string) ([]po.FaultInfoPO, int64, error) {
	q := func() *gorm.DB {
		query := r.db.WithContext(ctx).Model(&po.FaultInfoPO{})
		if code != "" {
			query = query.Where("code = ?", code)
		}
		return query
	}
	var total int64
	if err := q().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	if offset < 0 {
		// int 溢出保护（WR-03，双保险，照 UserRepo.List 同构）
		offset = 0
	}
	var faults []po.FaultInfoPO
	err := q().Offset(offset).Limit(size).
		Order("create_time DESC, id DESC").
		Find(&faults).Error
	if err != nil {
		return nil, 0, err
	}
	return faults, total, nil
}
