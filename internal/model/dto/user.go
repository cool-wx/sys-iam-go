// Package dto 定义 HTTP 请求/响应契约对象（与 PO 严格分离，禁 GORM struct 直接出网）。
package dto

import (
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/ids"
	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// UserCreateReq 创建用户请求。指针字段 nil=未提供；required 键缺失由 binding 校验拦截。
// 不得定义任何审计字段键（ENG-02 禁止：审计字段只能由服务端钩子从 ctx identity 与
// 系统时钟填充，防审计伪造）。
type UserCreateReq struct {
	Username *string       `json:"username" binding:"required"`
	Password *string       `json:"password" binding:"required"`
	RealName *string       `json:"realName" binding:"required"`
	Phone    *string       `json:"phone"`
	Email    *string       `json:"email"`
	Gender   *types.Gender `json:"gender"`
	DeptID   *ids.ID       `json:"deptId"`
	DeptName *string       `json:"deptName"`
}

// UpdateUserReq 更新用户请求：全指针字段，nil=未提供（不写库），空串=显式清空。
// 不得定义任何审计字段键（ENG-02 禁令：update_by/update_time 只由服务端填充，
// 客户端不可伪造）。可更新列集合收敛于本结构键集——请求体任意额外 JSON 键被
// 忽略且永远不会进 UPDATE 语句（T-1-10 白名单）。
type UpdateUserReq struct {
	RealName *string       `json:"realName"`
	Phone    *string       `json:"phone"`
	Email    *string       `json:"email"`
	Avatar   *string       `json:"avatar"`
	Gender   *types.Gender `json:"gender"`
	DeptID   *ids.ID       `json:"deptId"`
	DeptName *string       `json:"deptName"`
	Builtin  *bool         `json:"builtin"`
	Remark   *string       `json:"remark"`
}

// UserResp 用户响应契约。
// 不定义 password/extToken 键（源 @JsonProperty WRITE_ONLY 语义，永不序列化出网）；
// 全部键无 omitempty——未赋值字段以 null 键输出而非键缺失（源 JacksonConfig 契约）；
// roles 装配由 service 层两查询完成（USER-05 防 N+1），ToUserResp 初始化为空切片
// ——空角色输出 [] 非 null（对齐 PageData 契约）。
type UserResp struct {
	ID           ids.ID               `json:"id"`
	Username     *string              `json:"username"`
	RealName     *string              `json:"realName"`
	Avatar       *string              `json:"avatar"`
	Phone        *string              `json:"phone"`
	Email        *string              `json:"email"`
	Openid       *string              `json:"openid"`
	DeptID       *ids.ID              `json:"deptId"`
	DeptName     *string              `json:"deptName"`
	Gender       *types.Gender        `json:"gender"`
	Status       *types.UsageStatus   `json:"status"`
	FirstLogin   *bool                `json:"firstLogin"`
	Builtin      *bool                `json:"builtin"`
	Roles        []UserRoleRef        `json:"roles"`
	CreateByID   *ids.ID              `json:"createById"`
	CreateByName *string              `json:"createByName"`
	UpdateByID   *ids.ID              `json:"updateById"`
	UpdateByName *string              `json:"updateByName"`
	CreateTime   *types.LocalDateTime `json:"createTime"`
	UpdateTime   *types.LocalDateTime `json:"updateTime"`
	Remark       *string              `json:"remark"`
}

// UserRoleRef 用户关联角色引用（UserVo.roles 元素，源 UserVo.java:24-48 字段集）。
type UserRoleRef struct {
	RoleID   ids.ID `json:"roleId"`
	RoleName string `json:"roleName"`
}

// AssignRolesReq 分配用户角色请求：roleIds 全量替换语义（USER-04/D-12）——
// 空数组=全回收（合法语义）。
type AssignRolesReq struct {
	RoleIDs []ids.ID `json:"roleIds"`
}

// UpdateStatusReq 更新用户状态请求：DISABLED 停用 / ENABLED 启用（停用即踢存量
// 会话——D-09 安全增强）。required 拦截缺失键。
type UpdateStatusReq struct {
	Status *types.UsageStatus `json:"status" binding:"required"`
}

// ToUserResp PO→DTO 手写转换（枚举经 FromInt 兜底；int64 ID→契约 ID 类型）。
// Roles 初始化为空切片（服务层两查询装配后覆写）——无角色用户输出 [] 非 null。
func ToUserResp(u *po.UserPO) *UserResp {
	r := &UserResp{
		ID:           ids.ID(u.ID),
		Username:     u.Username,
		RealName:     u.RealName,
		Avatar:       u.Avatar,
		Phone:        u.Phone,
		Email:        u.Email,
		Openid:       u.Openid,
		DeptName:     u.DeptName,
		FirstLogin:   u.FirstLogin,
		Builtin:      u.Builtin,
		Roles:        []UserRoleRef{},
		CreateByName: u.CreateByName,
		UpdateByName: u.UpdateByName,
		Remark:       u.Remark,
	}
	if u.DeptID != nil {
		id := ids.ID(*u.DeptID)
		r.DeptID = &id
	}
	if u.CreateByID != nil {
		id := ids.ID(*u.CreateByID)
		r.CreateByID = &id
	}
	if u.UpdateByID != nil {
		id := ids.ID(*u.UpdateByID)
		r.UpdateByID = &id
	}
	if u.Gender != nil {
		g := types.GenderFromInt(*u.Gender)
		r.Gender = &g
	}
	if u.Status != nil {
		s := types.UsageStatusFromInt(*u.Status)
		r.Status = &s
	}
	if u.CreateTime != nil {
		t := types.LocalDateTime(*u.CreateTime)
		r.CreateTime = &t
	}
	if u.UpdateTime != nil {
		t := types.LocalDateTime(*u.UpdateTime)
		r.UpdateTime = &t
	}
	return r
}
