package types_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// MenuType 契约测试（照 Gender/UsageStatus 既有契约形态）：
// 读侧 name|int 双形态 + 未知值兜底 MENU（源 MenuType.fromValue 逐字语义，P7）。

func TestMenuTypeUnmarshalThreeForms(t *testing.T) {
	cases := []struct {
		raw  string
		want types.MenuType
	}{
		{`"DIR"`, types.MenuDir},
		{`"0"`, types.MenuDir},
		{`0`, types.MenuDir},
		{`"MENU"`, types.MenuMenu},
		{`"1"`, types.MenuMenu},
		{`1`, types.MenuMenu},
		{`"BUTTON"`, types.MenuButton},
		{`"2"`, types.MenuButton},
		{`2`, types.MenuButton},
	}
	for _, tc := range cases {
		var m types.MenuType
		require.NoError(t, json.Unmarshal([]byte(tc.raw), &m))
		assert.Equal(t, tc.want, m, "输入 %s", tc.raw)
	}
}

func TestMenuTypeUnmarshalUnknownFallsBackToMenu(t *testing.T) {
	// 未知值兜底 MENU 且永不返回 error（含 null）——验收点：
	// 未知 int 值 JSON 反序列化兜底为 MENU 不 500
	for _, raw := range []string{`"99"`, `99`, `"WEIRD"`, `null`} {
		var m types.MenuType
		require.NoError(t, json.Unmarshal([]byte(raw), &m), "输入 %s 不允许报错", raw)
		assert.Equal(t, types.MenuMenu, m, "输入 %s 兜底 MenuMenu", raw)
	}
}

func TestMenuTypeMarshalName(t *testing.T) {
	for m, want := range map[types.MenuType]string{
		types.MenuDir:    `"DIR"`,
		types.MenuMenu:   `"MENU"`,
		types.MenuButton: `"BUTTON"`,
	} {
		b, err := json.Marshal(m)
		require.NoError(t, err)
		assert.Equal(t, want, string(b))
	}
}

func TestMenuTypeFromIntFallback(t *testing.T) {
	assert.Equal(t, types.MenuDir, types.MenuTypeFromInt(0))
	assert.Equal(t, types.MenuMenu, types.MenuTypeFromInt(1))
	assert.Equal(t, types.MenuButton, types.MenuTypeFromInt(2))
	assert.Equal(t, types.MenuMenu, types.MenuTypeFromInt(99), "未知值兜底 MENU（源 fromValue）")
}
