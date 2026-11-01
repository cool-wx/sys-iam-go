package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// TestNormalizeContentType CR-02 直证：客户端 Content-Type 不可信，超 file_type
// 列宽（varchar(50)）必须钳位——浏览器上传 Office 文档发送的标准 MIME 65 字符，
// 直存 strict mode 1406 → 上传事务确定性回滚 500。
func TestNormalizeContentType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空（curl 不带 part 头，既有行为不变）", "", ""},
		{"常规 MIME 原样", "application/pdf", "application/pdf"},
		{"xlsx 标准 MIME 65 字符→兜底", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "application/octet-stream"},
		{"docx 标准 MIME 65 字符→兜底", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/octet-stream"},
		{"带参数段取 media type 本体", "application/vnd.ms-excel.sheet.macroEnabled.12; charset=binary", "application/vnd.ms-excel.sheet.macroEnabled.12"},
		{"参数段剥离后不超宽则保留本体", "text/plain; charset=utf-8", "text/plain"},
	}
	for _, c := range cases {
		got := normalizeContentType(c.in)
		assert.Equal(t, c.want, got, "case %q", c.name)
		assert.LessOrEqual(t, utf8.RuneCountInString(got), maxFileTypeLen, "case %q 超列宽", c.name)
		assert.True(t, utf8.ValidString(got), "case %q 产出无效 UTF-8", c.name)
	}

	// 恰好 50/51 字符边界：50 保留、51 兜底
	fifty := strings.Repeat("a", 50)
	assert.Equal(t, fifty, normalizeContentType(fifty))
	assert.Equal(t, "application/octet-stream", normalizeContentType(fifty+"x"))
}
