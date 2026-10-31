// Package types 枚举契约类型：MarshalJSON 输出 name 字符串；UnmarshalJSON
// 接受 name 与数字双形态；未知值兜底且永不返回 error（源 fromValue 逐字语义，P7）。
package types

import "strings"

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

// UnmarshalJSON 接受 "ENABLED"/"1"/1 与 "DISABLED"/"0"/0；未知值兜底 UsageEnabled
// 且不返回 error（含 null）。
func (s *UsageStatus) UnmarshalJSON(b []byte) error {
	switch str := strings.Trim(string(b), `"`); str {
	case "DISABLED", "0":
		*s = UsageDisabled
	default:
		// "ENABLED"/"1" 及一切未知值（含 null）→ UsageEnabled，对齐源 fromValue 兜底
		*s = UsageEnabled
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
