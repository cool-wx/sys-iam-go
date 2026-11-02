// file.go File 域两个 PO：FileInfoPO（sys_file）与 FileCategoryPO（sys_file_category）。
// 表名实证为 sys_file 非 sys_file_info（04-RESEARCH 重点 1 DDL 直读）——FileInfo
// 是源 Java 实体类名，Go 版按真实表名命名映射。列集与本机 SHOW CREATE TABLE
// 逐列对齐（scripts/ddl/sys_file.sql / sys_file_category.sql 同源）。
package po

import (
	"time"

	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
)

// FileInfoPO sys_file 表映射（16 列）。file_hash 无唯一索引——判重由应用层
// FindByCategoryAndHash 的 categoryId IS NULL 双分支查询承担（唯一索引
// 对 NULL 视为互不相等，未分类判重全靠 IS NULL 分支）；category_name 冗余列由
// 上传链服务端查库写入（不信请求体）。
type FileInfoPO struct {
	BaseWithOperator

	ID           int64   `gorm:"column:id;primaryKey"`
	CategoryID   *int64  `gorm:"column:category_id"`
	CategoryName *string `gorm:"column:category_name"`
	FileName     *string `gorm:"column:file_name"`
	FilePath     *string `gorm:"column:file_path"`
	FileType     *string `gorm:"column:file_type"`
	FileSize     *int64  `gorm:"column:file_size"`
	FileHash     *string `gorm:"column:file_hash"`
	Description  *string `gorm:"column:description"`
}

// TableName 显式表名（Tabler 接口）。
func (FileInfoPO) TableName() string {
	return "sys_file"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间 + 从 ctx identity 填充四操作人列。
// 无 identity 上下文时操作人列保持 NULL（等价源 AutoFillHandler "系统行为无操作人"）。
func (f *FileInfoPO) BeforeCreate(tx *gorm.DB) error {
	if f.ID == 0 {
		f.ID = int64(ids.NewID())
	}
	now := time.Now()
	f.CreateTime = &now
	f.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		f.CreateByID = &id.UserID
		f.CreateByName = &id.Name
		f.UpdateByID = &id.UserID
		f.UpdateByName = &id.Name
	}
	return nil
}

// FileCategoryPO sys_file_category 表映射（15 列）。树列 parent_id（NULL=根；
// 0 亦按根处理，源 buildTree 语义）+ parent_name 冗余列（add/edit 服务端查库写）。
type FileCategoryPO struct {
	BaseWithOperator

	ID           int64   `gorm:"column:id;primaryKey"`
	ParentID     *int64  `gorm:"column:parent_id"`
	ParentName   *string `gorm:"column:parent_name"`
	CategoryName *string `gorm:"column:category_name"`
	CategoryType *int    `gorm:"column:category_type"`
	Sort         *int    `gorm:"column:sort"`
	Icon         *string `gorm:"column:icon"`
	Status       *int8   `gorm:"column:status"`
}

// TableName 显式表名（Tabler 接口）。
func (FileCategoryPO) TableName() string {
	return "sys_file_category"
}

// BeforeCreate 生成雪花 ID（若未预置）+ 填充双时间 + 从 ctx identity 填充四操作人列。
func (fc *FileCategoryPO) BeforeCreate(tx *gorm.DB) error {
	if fc.ID == 0 {
		fc.ID = int64(ids.NewID())
	}
	now := time.Now()
	fc.CreateTime = &now
	fc.UpdateTime = &now
	if id, ok := auth.FromContext(tx.Statement.Context); ok {
		fc.CreateByID = &id.UserID
		fc.CreateByName = &id.Name
		fc.UpdateByID = &id.UserID
		fc.UpdateByName = &id.Name
	}
	return nil
}
