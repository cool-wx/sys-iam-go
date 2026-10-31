package types

import "strings"

// MenuType 菜单类型（源 MenuType.java：DIR(0,目录)/MENU(1,菜单)/BUTTON(2,按钮)）。
type MenuType int8

const (
	MenuDir    MenuType = 0
	MenuMenu   MenuType = 1
	MenuButton MenuType = 2
)

// MarshalJSON 输出 name 字符串。
func (m MenuType) MarshalJSON() ([]byte, error) {
	switch m {
	case MenuDir:
		return []byte(`"DIR"`), nil
	case MenuButton:
		return []byte(`"BUTTON"`), nil
	default:
		return []byte(`"MENU"`), nil
	}
}

// UnmarshalJSON 接受 "DIR"/"0"/0、"MENU"/"1"/1 与 "BUTTON"/"2"/2；未知值兜底
// MenuMenu 且不返回 error（含 null）——源 fromValue 逐字语义（P7）。
func (m *MenuType) UnmarshalJSON(b []byte) error {
	switch str := strings.Trim(string(b), `"`); str {
	case "DIR", "0":
		*m = MenuDir
	case "BUTTON", "2":
		*m = MenuButton
	default:
		// "MENU"/"1" 及一切未知值（含 null）→ MenuMenu，对齐源 fromValue 兜底
		*m = MenuMenu
	}
	return nil
}

// MenuTypeFromInt PO→DTO 转换兜底：非 DIR/BUTTON 一律 MENU（源语义）。
func MenuTypeFromInt(v int8) MenuType {
	switch v {
	case int8(MenuDir):
		return MenuDir
	case int8(MenuButton):
		return MenuButton
	default:
		return MenuMenu
	}
}
