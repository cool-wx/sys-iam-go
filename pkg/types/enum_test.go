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

func TestUsageStatusUnmarshalUnknownFallsBack(t *testing.T) {
	// 未知值兜底 UsageEnabled 且永不返回 error（源 fromValue 逐字语义，P7）
	for _, raw := range []string{`"99"`, `99`, `"WEIRD"`, `null`} {
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
