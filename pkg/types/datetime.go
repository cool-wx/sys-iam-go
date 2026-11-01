// Package types 定义 JSON 出网契约类型（时间无时区格式、枚举 name|int 双形态）。
// 这些类型只进 DTO 层——PO 层一律用原生类型，让 GORM 永远看不到自定义类型
// （研究 Pattern 1：根治 Valuer/Scanner 陷阱）。
package types

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// TimeFormat 源 JacksonConfig DATE_TIME_FORMAT 等价：yyyy-MM-dd HH:mm:ss。
const TimeFormat = "2006-01-02 15:04:05"

var (
	cstOnce sync.Once
	cstLoc  *time.Location
)

// CST 返回 Asia/Shanghai 时区（进程内缓存）。
// main 已内嵌 time/tzdata（P2：工控机无 /usr/share/zoneinfo 也可解析）；
// 此处 FixedZone 兜底保证即便 tzdata 缺失也恒为 UTC+8 墙钟，不静默漂移。
func CST() *time.Location {
	cstOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.FixedZone("CST", 8*60*60)
		}
		cstLoc = loc
	})
	return cstLoc
}

// LocalDateTime 时间契约类型：MarshalJSON 恒以 Asia/Shanghai 墙钟输出
// "yyyy-MM-dd HH:mm:ss"（无 T、无时区偏移）；UnmarshalJSON 对称接受同一格式
// （源 Jackson DATE_TIME_FORMAT 反序列化语义）与 RFC3339 宽容兜底（null 字面量
// 语义为"未提供"，由调用方的指针 nil 分支承接，不在此报错）。
type LocalDateTime time.Time

// MarshalJSON 序列化为 CST 墙钟字符串。
func (t LocalDateTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).In(CST()).Format(TimeFormat) + `"`), nil
}

// UnmarshalJSON 反序列化请求侧时间：主格式 TimeFormat（源契约），RFC3339 兜底
// （宽容第三方客户端 ISO8601 传入）。JSON null 由 encoding/json 直接跳过（指针
// 字段保持 nil）；空串视为未提供置零值。b 为含引号的原始字符串字面量，先 Trim。
func (t *LocalDateTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" {
		*t = LocalDateTime(time.Time{})
		return nil
	}
	if parsed, err := time.ParseInLocation(TimeFormat, s, CST()); err == nil {
		*t = LocalDateTime(parsed)
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("时间格式须为 yyyy-MM-dd HH:mm:ss: %w", err)
	}
	*t = LocalDateTime(parsed)
	return nil
}
