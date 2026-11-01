package types_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/types"
)

func TestUsageStatusUnmarshalThreeForms(t *testing.T) {
	for name, raw := range map[string]string{
		"name":   `"ENABLED"`,
		"intstr": `"1"`,
		"number": `1`,
	} {
		var s types.UsageStatus
		require.NoError(t, json.Unmarshal([]byte(raw), &s), "形态 %s", name)
		assert.Equal(t, types.UsageEnabled, s, "形态 %s → UsageEnabled", name)
	}
}

func TestUsageStatusUnmarshalDisabled(t *testing.T) {
	for _, raw := range []string{`"DISABLED"`, `"0"`, `0`} {
		var s types.UsageStatus
		require.NoError(t, json.Unmarshal([]byte(raw), &s))
		assert.Equal(t, types.UsageDisabled, s)
	}
}

func TestUsageStatusUnmarshalUnknownRejects(t *testing.T) {
	// 非法枚举值必须报错（WR-03 收紧）：状态变更写路径静默兜底会把"停用"拼写
	// 错误反转成"启用"——越界数字/未知名/大小写不符/带空白均不再静默 ENABLED
	for _, raw := range []string{`"99"`, `99`, `"WEIRD"`, `"DISABLE"`, `"DISABLED "`, `"ENABLE "`, `"enabled"`} {
		var s types.UsageStatus
		require.Error(t, json.Unmarshal([]byte(raw), &s), "输入 %s 必须报错", raw)
	}
}

func TestUsageStatusUnmarshalNullFallsBack(t *testing.T) {
	// null/空串兜底保留（源 fromValue 读路径语义；指针 DTO 字段遇 JSON null 实际
	// 不进 UnmarshalJSON，由 binding required 拦截——此处锁非指针用法契约）
	for _, raw := range []string{`null`, `""`} {
		var s types.UsageStatus
		require.NoError(t, json.Unmarshal([]byte(raw), &s), "输入 %s 不允许报错", raw)
		assert.Equal(t, types.UsageEnabled, s, "输入 %s 兜底 UsageEnabled", raw)
	}
}

func TestUsageStatusMarshalName(t *testing.T) {
	b, err := json.Marshal(types.UsageEnabled)
	require.NoError(t, err)
	assert.Equal(t, `"ENABLED"`, string(b))

	b, err = json.Marshal(types.UsageDisabled)
	require.NoError(t, err)
	assert.Equal(t, `"DISABLED"`, string(b))
}

func TestUsageStatusFromIntFallback(t *testing.T) {
	assert.Equal(t, types.UsageEnabled, types.UsageStatusFromInt(1))
	assert.Equal(t, types.UsageDisabled, types.UsageStatusFromInt(0))
	assert.Equal(t, types.UsageEnabled, types.UsageStatusFromInt(99), "未知值兜底 ENABLED")
}

func TestGenderUnmarshalThreeForms(t *testing.T) {
	cases := []struct {
		raw  string
		want types.Gender
	}{
		{`"MALE"`, types.GenderMale},
		{`"1"`, types.GenderMale},
		{`1`, types.GenderMale},
		{`"FEMALE"`, types.GenderFemale},
		{`"2"`, types.GenderFemale},
		{`2`, types.GenderFemale},
		{`"UNKNOWN"`, types.GenderUnknown},
		{`"0"`, types.GenderUnknown},
		{`0`, types.GenderUnknown},
	}
	for _, tc := range cases {
		var g types.Gender
		require.NoError(t, json.Unmarshal([]byte(tc.raw), &g))
		assert.Equal(t, tc.want, g, "输入 %s", tc.raw)
	}
}

func TestGenderUnmarshalUnknownFallsBack(t *testing.T) {
	for _, raw := range []string{`"99"`, `99`, `"WEIRD"`, `null`} {
		var g types.Gender
		require.NoError(t, json.Unmarshal([]byte(raw), &g), "输入 %s 不允许报错", raw)
		assert.Equal(t, types.GenderUnknown, g, "输入 %s 兜底 GenderUnknown", raw)
	}
}

func TestGenderMarshalName(t *testing.T) {
	for g, want := range map[types.Gender]string{
		types.GenderUnknown: `"UNKNOWN"`,
		types.GenderMale:    `"MALE"`,
		types.GenderFemale:  `"FEMALE"`,
	} {
		b, err := json.Marshal(g)
		require.NoError(t, err)
		assert.Equal(t, want, string(b))
	}
}

func TestGenderFromIntFallback(t *testing.T) {
	assert.Equal(t, types.GenderMale, types.GenderFromInt(1))
	assert.Equal(t, types.GenderFemale, types.GenderFromInt(2))
	assert.Equal(t, types.GenderUnknown, types.GenderFromInt(0))
	assert.Equal(t, types.GenderUnknown, types.GenderFromInt(99), "未知值兜底 UNKNOWN")
}
