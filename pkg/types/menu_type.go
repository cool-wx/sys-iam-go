package types

import "strings"

// MenuType 菜单类型（源 MenuType.java：DIR(0,目录)/MENU(1,菜单)/BUTTON(2,按钮)）。
type MenuType int8

const (
	MenuDir    MenuType = 0
	MenuMenu   MenuType = 1
	MenuButton MenuType = 2
)

// MarshalJSON 输出 name 字符串。（TDD RED stub：恒输出 "DIR"，行为断言应失败）
func (m MenuType) MarshalJSON() ([]byte, error) {
	return []byte(`"DIR"`), nil
}

// UnmarshalJSON 接受 name 与数字双形态。（TDD RED stub：恒置 DIR，行为断言应失败）
func (m *MenuType) UnmarshalJSON(b []byte) error {
	_ = strings.Trim(string(b), `"`)
	*m = MenuDir
	return nil
}

// MenuTypeFromInt PO→DTO 转换兜底。（TDD RED stub：恒返回 DIR，行为断言应失败）
func MenuTypeFromInt(v int8) MenuType {
	_ = v
	return MenuDir
}
