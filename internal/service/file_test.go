package service

import (
	"context"
	"mime/multipart"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/pkg/errcode"
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

// TestUploadRejectsOverlongFilename WR-05 直证：file_name varchar(255) 字符
// 语义——超 255 rune 的文件名上传入口直接 400「文件名过长」，不走 strict mode
// 1406 → 500；中文名按 rune 计数（256 个汉字 = 768 字节超限，而 300 字节
// 100 汉字合法）。校验在所有依赖使用之前，nil 依赖构造即可直测入口钳位。
func TestUploadRejectsOverlongFilename(t *testing.T) {
	svc := NewFileService(nil, nil, nil, nil)

	// 256 字符 → 400/10400「文件名过长」
	overlong := &multipart.FileHeader{Filename: strings.Repeat("a", maxFileNameLen+1)}
	_, err := svc.Upload(context.Background(), overlong, "")
	require.Error(t, err)
	var ec *errcode.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, errcode.CodeInvalidParam, ec.Code)
	assert.Equal(t, "文件名过长", ec.Msg)

	// 256 个汉字（768 字节）同样超限——证明 rune 计数而非字节计数
	cjkOver := &multipart.FileHeader{Filename: strings.Repeat("中", maxFileNameLen+1)}
	_, err = svc.Upload(context.Background(), cjkOver, "")
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, "文件名过长", ec.Msg)

	// 100 个汉字（300 字节 > 255 字节但 100 rune ≤ 255）不触名称钳位——
	// 错误为后续步骤（空 FileHeader Open 失败）而非「文件名过长」
	cjkOK := &multipart.FileHeader{Filename: strings.Repeat("中", 100)}
	_, err = svc.Upload(context.Background(), cjkOK, "")
	require.Error(t, err)
	require.ErrorAs(t, err, &ec)
	assert.NotEqual(t, "文件名过长", ec.Msg, "300 字节 100 rune 中文名不得被字节口径误拒")
}
