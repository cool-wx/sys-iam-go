package types_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/types"
)

// FaultType 契约测试（照 Gender/MenuType 既有契约形态）：
// 读侧 name|int 双形态 + 未知值兜底 UNKNOWN（源 FaultTypeEnum.fromValue/fromInfo
// 逐字语义）；ParseFaultType 中文/int 串双形态（FAUL-02 Excel 报警类型列解析依赖）。

func TestFaultTypeUnmarshalThreeForms(t *testing.T) {
	cases := []struct {
		raw  string
		want types.FaultType
	}{
		{`"INFO"`, types.FaultTypeInfo},
		{`"0"`, types.FaultTypeInfo},
		{`0`, types.FaultTypeInfo},
		{`"WARNING"`, types.FaultTypeWarning},
		{`"1"`, types.FaultTypeWarning},
		{`1`, types.FaultTypeWarning},
		{`"FAULT"`, types.FaultTypeFault},
		{`"2"`, types.FaultTypeFault},
		{`2`, types.FaultTypeFault},
		{`"UNKNOWN"`, types.FaultTypeUnknown},
		{`"3"`, types.FaultTypeUnknown},
		{`3`, types.FaultTypeUnknown},
	}
	for _, tc := range cases {
		var ft types.FaultType
		require.NoError(t, json.Unmarshal([]byte(tc.raw), &ft))
		assert.Equal(t, tc.want, ft, "输入 %s", tc.raw)
	}
}

func TestFaultTypeUnmarshalUnknownFallsBackUnknown(t *testing.T) {
	// 未知值兜底 UNKNOWN 且永不返回 error（含 null）——源 fromValue 逐字语义
	for _, raw := range []string{`"99"`, `99`, `"WEIRD"`, `null`, `"消息"`} {
		var ft types.FaultType
		require.NoError(t, json.Unmarshal([]byte(raw), &ft), "输入 %s 不允许报错", raw)
		assert.Equal(t, types.FaultTypeUnknown, ft, "输入 %s 兜底 FaultTypeUnknown", raw)
	}
}

func TestFaultTypeMarshalName(t *testing.T) {
	for ft, want := range map[types.FaultType]string{
		types.FaultTypeInfo:    `"INFO"`,
		types.FaultTypeWarning: `"WARNING"`,
		types.FaultTypeFault:   `"FAULT"`,
		types.FaultTypeUnknown: `"UNKNOWN"`,
	} {
		b, err := json.Marshal(ft)
		require.NoError(t, err)
		assert.Equal(t, want, string(b))
	}
	// 未知 int 值 MarshalJSON 亦兜底 UNKNOWN
	b, err := json.Marshal(types.FaultType(99))
	require.NoError(t, err)
	assert.Equal(t, `"UNKNOWN"`, string(b))
}

func TestParseFaultTypeDualForms(t *testing.T) {
	// int 串四值 + 中文文案四值；未知值兜底 3 不报错（源 fromInfo 语义）
	cases := []struct {
		in   string
		want int8
	}{
		{"0", 0},
		{"1", 1},
		{"2", 2},
		{"3", 3},
		{"消息", 0},
		{"警告", 1},
		{"故障", 2},
		{"未知", 3},
		{"随便什么文案", 3}, // 未知文案兜底
		{"", 3},          // 空串兜底（Excel 空单元格越界取空串）
		{"99", 3},        // 未知 int 串兜底
	}
	for _, tc := range cases {
		got, err := types.ParseFaultType(tc.in)
		require.NoError(t, err, "输入 %q 不允许报错（源兜底语义）", tc.in)
		assert.Equal(t, tc.want, got, "输入 %q", tc.in)
	}
}

func TestFaultTypeNameReverseLookup(t *testing.T) {
	assert.Equal(t, "消息", types.FaultTypeName(0))
	assert.Equal(t, "警告", types.FaultTypeName(1))
	assert.Equal(t, "故障", types.FaultTypeName(2))
	assert.Equal(t, "未知", types.FaultTypeName(3))
	assert.Equal(t, "未知", types.FaultTypeName(99), "未知 int 反查兜底「未知」")
}

func TestFaultTypeFromIntFallback(t *testing.T) {
	assert.Equal(t, types.FaultTypeInfo, types.FaultTypeFromInt(0))
	assert.Equal(t, types.FaultTypeFault, types.FaultTypeFromInt(2))
	assert.Equal(t, types.FaultTypeUnknown, types.FaultTypeFromInt(3))
	assert.Equal(t, types.FaultTypeUnknown, types.FaultTypeFromInt(99), "未知值兜底 UNKNOWN（源 fromValue）")
}
