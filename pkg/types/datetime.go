// Package types 定义 JSON 出网契约类型（时间无时区格式、枚举 name|int 双形态）。
// 这些类型只进 DTO 层——PO 层一律用原生类型，让 GORM 永远看不到自定义类型
// （研究 Pattern 1：根治 Valuer/Scanner 陷阱）。
package types

import (
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
// "yyyy-MM-dd HH:mm:ss"（无 T、无时区偏移）。
type LocalDateTime time.Time

// MarshalJSON 序列化为 CST 墙钟字符串。
func (t LocalDateTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).In(CST()).Format(TimeFormat) + `"`), nil
}
