// Package types 枚举契约类型：MarshalJSON 输出 name 字符串；UnmarshalJSON
// 接受 name 与数字双形态。UsageStatus 反序列化已收紧：非空未知值报错
// ——状态变更写路径静默兜底会把"停用"拼写错误反转成"启用"；null/空串兜底保留。
// Gender 未知值仍兜底 GenderUnknown（源 fromValue 逐字语义——无状态反转风险）。
package types

import (
	"fmt"
	"strings"
)

// UsageStatus 启用状态（源 UsageStatus.java：DISABLED(0)/ENABLED(1)）。
type UsageStatus int8

const (
	UsageDisabled UsageStatus = 0
	UsageEnabled  UsageStatus = 1
)

// MarshalJSON 输出 name 字符串。
func (s UsageStatus) MarshalJSON() ([]byte, error) {
	if s == UsageDisabled {
		return []byte(`"DISABLED"`), nil
	}
	return []byte(`"ENABLED"`), nil
}

// UnmarshalJSON 接受 "ENABLED"/"1"/1 与 "DISABLED"/"0"/0；null/空串兜底 UsageEnabled
// （源 fromValue 读路径语义保留）；其余未知值返回 error（收紧原因：反序列化
// 消费方为全部状态变更写 DTO——UpdateStatusReq/ApiKeyUpdateStatusReq/MenuUpdateReq/
// DeptUpdateReq/RoleUpdateReq 五者（行为上同为本收紧的期望语义）
// ——拼写错误如 "DISABLE" 静默兜底会执行启用、被停用用户被静默恢复且旧会话不踢；
// API 契约允许重新设计（CLAUDE.md），无源契约约束。出网契约不受影响：MarshalJSON
// 恒输出合法 name，无读路径反序列化消费方）。
func (s *UsageStatus) UnmarshalJSON(b []byte) error {
	switch str := strings.Trim(string(b), `"`); str {
	case "DISABLED", "0":
		*s = UsageDisabled
	case "ENABLED", "1":
		*s = UsageEnabled
	case "null", "":
		// null/空串兜底保留（源 fromValue 语义）：指针 DTO 字段遇 JSON null 实际
		// 不进本方法（encoding/json 直接置 nil 指针、由 binding required 拦截）
		*s = UsageEnabled
	default:
		// 非法枚举值（含大小写/拼写错误如 "DISABLE"、"DISABLED "）：报错 →
		// 绑定层译 400/10401「参数校验失败」，绝不静默写库
		return fmt.Errorf("非法的状态值: %s", str)
	}
	return nil
}

// UsageStatusFromInt PO→DTO 转换兜底：非 DISABLED 一律 ENABLED（源语义）。
func UsageStatusFromInt(v int8) UsageStatus {
	if v == int8(UsageDisabled) {
		return UsageDisabled
	}
	return UsageEnabled
}

// Gender 性别（源 Gender.java：UNKNOWN(0)/MALE(1)/FEMALE(2)）。
type Gender int8

const (
	GenderUnknown Gender = 0
	GenderMale    Gender = 1
	GenderFemale  Gender = 2
)

// MarshalJSON 输出 name 字符串。
func (g Gender) MarshalJSON() ([]byte, error) {
	switch g {
	case GenderMale:
		return []byte(`"MALE"`), nil
	case GenderFemale:
		return []byte(`"FEMALE"`), nil
	default:
		return []byte(`"UNKNOWN"`), nil
	}
}

// UnmarshalJSON 接受 name 与数字双形态；未知值兜底 GenderUnknown 且不返回 error（含 null）。
func (g *Gender) UnmarshalJSON(b []byte) error {
	switch str := strings.Trim(string(b), `"`); str {
	case "MALE", "1":
		*g = GenderMale
	case "FEMALE", "2":
		*g = GenderFemale
	default:
		// "UNKNOWN"/"0" 及一切未知值（含 null）→ GenderUnknown，对齐源 fromValue 兜底
		*g = GenderUnknown
	}
	return nil
}

// GenderFromInt PO→DTO 转换兜底：非 MALE/FEMALE 一律 UNKNOWN（源语义）。
func GenderFromInt(v int8) Gender {
	switch v {
	case int8(GenderMale):
		return GenderMale
	case int8(GenderFemale):
		return GenderFemale
	default:
		return GenderUnknown
	}
}
