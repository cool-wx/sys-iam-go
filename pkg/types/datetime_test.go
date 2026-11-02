package types_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/types"
)

func TestLocalDateTimeMarshalCSTFormat(t *testing.T) {
	// 固定时刻：2026-10-31 15:04:05 Asia/Shanghai
	fixed := types.LocalDateTime(time.Date(2026, 10, 31, 15, 4, 5, 0, types.CST()))
	b, err := json.Marshal(fixed)
	require.NoError(t, err)
	assert.Equal(t, `"2026-10-31 15:04:05"`, string(b))
	assert.NotContains(t, string(b), "T", "无 ISO T 分隔符")
	assert.NotContains(t, string(b), "Z", "无 UTC 标记")
	assert.NotContains(t, string(b), "+08:00", "无时区偏移")
}

func TestLocalDateTimeMarshalTimezoneIndependent(t *testing.T) {
	// 同一绝对时刻用 UTC 构造：输出恒为 Asia/Shanghai 墙钟（证明时区无关性）
	utcInstant := types.LocalDateTime(time.Date(2026, 10, 31, 7, 4, 5, 0, time.UTC))
	b, err := json.Marshal(utcInstant)
	require.NoError(t, err)
	assert.Equal(t, `"2026-10-31 15:04:05"`, string(b), "UTC 构造的同一时刻输出相同 CST 墙钟")

	// 字符串本身不含 T/Z/偏移
	assert.False(t, strings.ContainsAny(string(b), "TZ"))
}

func TestCSTLocation(t *testing.T) {
	loc := types.CST()
	require.NotNil(t, loc)
	name, offset := time.Now().In(loc).Zone()
	assert.Equal(t, "CST", name, "Asia/Shanghai 标准时区名")
	assert.Equal(t, 8*60*60, offset, "UTC+8 偏移")
}

// TestLocalDateTimeUnmarshalJSON 请求侧反序列化（与 MarshalJSON 对称）：
// 主格式 yyyy-MM-dd HH:mm:ss（源 DATE_TIME_FORMAT）；RFC3339 兜底；空串置零值；
// 非法格式报错不静默。
func TestLocalDateTimeUnmarshalJSON(t *testing.T) {
	var t1 types.LocalDateTime
	require.NoError(t, json.Unmarshal([]byte(`"2027-01-02 03:04:05"`), &t1))
	assert.Equal(t, "2027-01-02 03:04:05", time.Time(t1).Format(types.TimeFormat), "主格式应解析（CST 墙钟）")

	var t2 types.LocalDateTime
	require.NoError(t, json.Unmarshal([]byte(`"2027-01-02T03:04:05Z"`), &t2))
	assert.Equal(t, "2027-01-02 11:04:05", time.Time(t2).In(types.CST()).Format(types.TimeFormat), "RFC3339 兜底应换算 CST")

	var t3 types.LocalDateTime
	require.NoError(t, json.Unmarshal([]byte(`""`), &t3))
	assert.True(t, time.Time(t3).IsZero(), "空串应置零值")

	var t4 types.LocalDateTime
	require.Error(t, json.Unmarshal([]byte(`"not-a-time"`), &t4), "非法格式必须报错")
}
