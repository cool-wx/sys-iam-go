package types

import "strings"

// FaultType 故障报警类型（源 FaultTypeEnum.java：INFO(0,消息)/WARNING(1,警告)/
// FAULT(2,故障)/UNKNOWN(3,未知)）。fromValue/fromInfo 双入口兜底均为 UNKNOWN——
// 读侧兼容既有先例（ENG-04，同 Gender 兜底形态：无状态反转风险，静默兜底安全）。
type FaultType int8

const (
	FaultTypeInfo    FaultType = 0
	FaultTypeWarning FaultType = 1
	FaultTypeFault   FaultType = 2
	FaultTypeUnknown FaultType = 3
)

// faultTypeNames MarshalJSON 出网 name（英文枚举名，照 MenuType "DIR" 惯例）。
var faultTypeNames = map[FaultType]string{
	FaultTypeInfo:    "INFO",
	FaultTypeWarning: "WARNING",
	FaultTypeFault:   "FAULT",
}

// faultTypeCNLabels 中文文案（源 FaultTypeEnum 构造参数 info：消息/警告/故障/未知），
// Excel 报警类型列解析（ParseFaultType）与反查（FaultTypeName）使用。
var faultTypeCNLabels = map[FaultType]string{
	FaultTypeInfo:    "消息",
	FaultTypeWarning: "警告",
	FaultTypeFault:   "故障",
	FaultTypeUnknown: "未知",
}

// MarshalJSON 输出 name 字符串；未知 int 值兜底 "UNKNOWN"（源 fromValue）。
func (t FaultType) MarshalJSON() ([]byte, error) {
	if name, ok := faultTypeNames[t]; ok {
		return []byte(`"` + name + `"`), nil
	}
	return []byte(`"UNKNOWN"`), nil
}

// UnmarshalJSON 接受 "INFO"/"0"/0、"WARNING"/"1"/1、"FAULT"/"2"/2 与
// "UNKNOWN"/"3"/3；其余未知值兜底 FaultTypeUnknown 且不返回 error（含 null）——
// 源 fromValue 逐字语义（读侧兼容先例，同 Gender：故障类型无状态反转风险）。
func (t *FaultType) UnmarshalJSON(b []byte) error {
	switch strings.Trim(string(b), `"`) {
	case "INFO", "0":
		*t = FaultTypeInfo
	case "WARNING", "1":
		*t = FaultTypeWarning
	case "FAULT", "2":
		*t = FaultTypeFault
	default:
		// "UNKNOWN"/"3" 及一切未知值（含 null）→ FaultTypeUnknown
		*t = FaultTypeUnknown
	}
	return nil
}

// ParseFaultType Excel 报警类型列解析（源 fromValue/fromInfo 双形态）：int 串
// （"0".."3"）与中文文案（消息/警告/故障/未知）均解析；未知值兜底 UNKNOWN(3)
// 不报错（源兜底语义，FAUL-02 导入依赖——脏文案导入不得炸批）。
func ParseFaultType(s string) (int8, error) {
	switch strings.TrimSpace(s) {
	case "0", "消息":
		return int8(FaultTypeInfo), nil
	case "1", "警告":
		return int8(FaultTypeWarning), nil
	case "2", "故障":
		return int8(FaultTypeFault), nil
	default:
		// "3"/"未知" 及一切未知文案 → UNKNOWN(3)，不报错
		return int8(FaultTypeUnknown), nil
	}
}

// FaultTypeName int8 值反查中文文案（消息/警告/故障/未知）；未知值兜底「未知」。
func FaultTypeName(v int8) string {
	if label, ok := faultTypeCNLabels[FaultType(v)]; ok {
		return label
	}
	return faultTypeCNLabels[FaultTypeUnknown]
}

// FaultTypeFromInt PO→DTO 转换兜底：未知 int 一律 UNKNOWN（源 fromValue 语义）。
func FaultTypeFromInt(v int8) FaultType {
	if v >= int8(FaultTypeInfo) && v <= int8(FaultTypeFault) {
		return FaultType(v)
	}
	return FaultTypeUnknown
}
