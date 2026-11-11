package middleware

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
	"github.com/cool-wx/sys-iam-go/pkg/errcode"
	"github.com/cool-wx/sys-iam-go/pkg/resp"
)

// newTestWriter 测试专用小容量 writer（直接构造未导出字段；sink 可注入 fake）。
func newTestWriter(t *testing.T, capacity int, sink Sink) *OpLogWriter {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := &OpLogWriter{
		ch:      make(chan *po.OperationLogPO, capacity),
		flushCh: make(chan chan struct{}, 1),
		quit:    make(chan struct{}),
		sink:    sink,
		logger:  logger,
	}
	t.Cleanup(w.Stop)
	return w
}

// TestMaskSensitiveEightFields 8 敏感字段逐字段掩码（与生产正则一致）+
// 混合体普通字段原样。
func TestMaskSensitiveEightFields(t *testing.T) {
	// 逐字段：{"<field>":"x"} → {"<field>":"******"}（键名保留）
	fields := []string{
		"password", "oldPassword", "newPassword", "secretKey",
		"apiKey", "token", "accessToken", "refreshToken",
	}
	for _, f := range fields {
		in := fmt.Sprintf(`{"%s":"x"}`, f)
		out := MaskSensitive(in)
		assert.Equal(t, fmt.Sprintf(`{"%s":"******"}`, f), out, "字段 %s 未按源语义掩码", f)
		assert.NotContains(t, out, `"x"`, "字段 %s 明文值残留", f)
	}

	// 混合体：8 字段 + 普通字段并存——普通字段原样、8 字段全掩
	in := `{"username":"admin","password":"p1","apiKey":"k1","token":"t1",` +
		`"oldPassword":"o1","newPassword":"n1","secretKey":"s1","accessToken":"a1",` +
		`"refreshToken":"r1","remark":"普通值保留"}`
	out := MaskSensitive(in)
	for _, f := range fields {
		assert.Contains(t, out, fmt.Sprintf(`"%s":"******"`, f), "混合体中 %s 未掩码", f)
	}
	assert.Contains(t, out, `"username":"admin"`, "普通字段不得被掩码")
	assert.Contains(t, out, `"remark":"普通值保留"`, "普通字段不得被掩码")
	assert.NotContains(t, out, `"p1"`)
	assert.NotContains(t, out, `"r1"`, "敏感值明文残留")
}

// TestMaskSensitiveNonStringUntouched 非字符串值不掩码（正则只匹配带
// 双引号的字符串值形态——数字/布尔/null 原样保留）。
func TestMaskSensitiveNonStringUntouched(t *testing.T) {
	cases := []string{
		`{"apiKey":123}`,
		`{"token":null}`,
		`{"password":true}`,
		`{"password":["a","b"]}`,
		`{"token":{"nested":"x"}}`,
	}
	for _, in := range cases {
		assert.Equal(t, in, MaskSensitive(in), "非字符串值被错误掩码: %s", in)
	}
}

// TestMaskSensitiveOrderWithTruncation 掩码先于截断的顺序直证（prohibition）：
// 70KB 含敏感字段 JSON → 掩码 → 截 60 000 字节；断言结果长度 ≤60 000 且截断
// 结果内已无明文敏感值（若顺序颠倒，截断边界可能切出未掩码明文残余）。
func TestMaskSensitiveOrderWithTruncation(t *testing.T) {
	big := `{"password":"secret123","data":"` + strings.Repeat("A", 70*1024) + `"}`
	require.Greater(t, len(big), paramsMaxBytes, "构造体必须超 60000 字节")

	masked := MaskSensitive(big)
	require.Contains(t, masked, `"password":"******"`)

	truncated := truncateBytes(masked, paramsMaxBytes)
	assert.LessOrEqual(t, len(truncated), paramsMaxBytes, "截断后超上限")
	assert.Contains(t, truncated, `"password":"******"`, "掩码必须先于截断保留在截断结果内")
	assert.NotContains(t, truncated, "secret123", "明文敏感值泄漏进截断结果")
}

// TestTruncateBytesUTF8Safe 直证：切点落在多字节字符中间时回退至完整
// rune 边界——utf8mb4 params 列 INSERT 不再报 1366（strict mode 整批日志丢失，
// 同批无关日志一并受损）。既有测试只覆盖 ASCII 切点（TestMaskSensitiveOrder
// WithTruncation 用 "A" 重复），此处补多字节切点面。
func TestTruncateBytesUTF8Safe(t *testing.T) {
	// 59999 字节 ASCII + "错"（3 字节，总 60002 > 60000）→ 硬切 60000 落在
	// "错"中间，修复前产出无效 UTF-8 尾串；修复后回退至 rune 边界
	in := strings.Repeat("A", paramsMaxBytes-1) + "错"
	require.Len(t, in, paramsMaxBytes+2)
	out := truncateBytes(in, paramsMaxBytes)
	assert.LessOrEqual(t, len(out), paramsMaxBytes, "截断后仍不超上限")
	assert.True(t, utf8.ValidString(out), "切点必须回退到 rune 边界（utf8mb4 列 1366）")
	assert.Equal(t, strings.Repeat("A", paramsMaxBytes-1), out, "完整 ASCII 前缀原样保留")

	// 全多字节超长体：截断结果恒为合法 UTF-8 且为完整 rune 序列
	cn := strings.Repeat("错", 30000) // 90000 字节，恰在 60000 处于 rune 边界
	out2 := truncateBytes(cn, paramsMaxBytes)
	assert.True(t, utf8.ValidString(out2))
	assert.Equal(t, 20000, utf8.RuneCountInString(out2), "恰在边界时保留 20000 个完整 rune")

	// 不超上限原样返回（含多字节）
	assert.Equal(t, "参数校验失败", truncateBytes("参数校验失败", paramsMaxBytes))
}

// TestTruncateBytesInvalidInputLinear 直证：输入本身含非法字节时，截断
// 一次 O(n) 扫描定位首个非法序列起点（不逐字节回退全量重扫）。恶意构造 >60KB
// 非法 body 在请求 goroutine 的 defer 中同步执行，O(n²) 实现实测 ~18.8ms/请求
// （iter-2 基准，正常请求 <10µs，放大 ~2000 倍）；O(n) 实现同场景 µs 级。时间
// 上界取 5ms 宽松阈值：与 O(n²) 相差近两个数量级，机器抖动不致误报。
func TestTruncateBytesInvalidInputLinear(t *testing.T) {
	// 前置 0xFF（iter-2 实测 18.8ms 的构造）：30000A + \xff + 30000B——切点
	// 尾部是合法 ASCII，ValidString 失败由一次 range 定位首个非法字节处截断，
	// 非法字节之后的合法内容一并丢弃（畸形请求原文，语义无损）。
	in := strings.Repeat("A", 30000) + "\xff" + strings.Repeat("B", 30000)
	require.Len(t, in, paramsMaxBytes+1)
	start := time.Now()
	out := truncateBytes(in, paramsMaxBytes)
	elapsed := time.Since(start)
	assert.Equal(t, strings.Repeat("A", 30000), out, "必须截断至首个非法字节前的完整前缀")
	assert.LessOrEqual(t, len(out), paramsMaxBytes)
	assert.True(t, utf8.ValidString(out), "截断结果恒为合法 UTF-8（utf8mb4 列 1366）")
	assert.Less(t, elapsed, 5*time.Millisecond,
		"非法输入截断必须 O(n)（O(n²) 同构造实测 18.8ms）: %s", elapsed)

	// 非法字节落在切点尾部：碎片回退后仍非法 → 同样一次 range 定位。
	in2 := strings.Repeat("A", 59999) + "\xff" + "B"
	require.Len(t, in2, paramsMaxBytes+1)
	out2 := truncateBytes(in2, paramsMaxBytes)
	assert.Equal(t, strings.Repeat("A", 59999), out2, "切点尾部非法字节连同碎片一并截除")
	assert.True(t, utf8.ValidString(out2))

	// 密集非法点（iter-2 实测 0.6ms 构造族）：首字节合法、次字节即非法 →
	// 截至首个非法点，仅存 1 字节。
	in3 := strings.Repeat("A\xff", 30000) + "A"
	require.Len(t, in3, paramsMaxBytes+1)
	out3 := truncateBytes(in3, paramsMaxBytes)
	assert.Equal(t, "A", out3, "首个非法序列起点之前的完整前缀")
	assert.True(t, utf8.ValidString(out3))

	// 合法编码的 U+FFFD（size==3）不误判为解码失败（size==1）——恰在边界的
	// 完整 U+FFFD 不得被当作非法截除。
	in4 := strings.Repeat("A", paramsMaxBytes-3) + "�" + "X"
	require.Len(t, in4, paramsMaxBytes+1)
	out4 := truncateBytes(in4, paramsMaxBytes)
	assert.Equal(t, strings.Repeat("A", paramsMaxBytes-3)+"�", out4,
		"合法编码 U+FFFD 是完整 rune，不得误截")
	assert.True(t, utf8.ValidString(out4))
}

// TestEnqueueNeverBlocks chan 满丢弃不阻塞直证（prohibition）：容量 1 的 writer
// 先填满，再 Enqueue 1000 次——全部走 default 丢弃路径，总耗时可忽略且不 panic。
func TestEnqueueNeverBlocks(t *testing.T) {
	sinkCalls := 0
	w := newTestWriter(t, 1, func(ctx context.Context, entries []po.OperationLogPO) error {
		sinkCalls += len(entries)
		return nil
	})
	op := "op"
	w.Enqueue(&po.OperationLogPO{Operation: &op}) // 填满容量 1
	require.Equal(t, int64(0), w.dropped.Load())

	start := time.Now()
	for i := 0; i < 1000; i++ {
		w.Enqueue(&po.OperationLogPO{Operation: &op})
	}
	elapsed := time.Since(start)
	assert.Equal(t, int64(1000), w.dropped.Load(), "chan 满时应全部丢弃计数")
	assert.Less(t, elapsed, time.Second, "Enqueue 阻塞了业务请求（prohibition 违规）: %s", elapsed)
	assert.Equal(t, 0, sinkCalls, "无 worker 消费时不落库")
}

// TestFlushDrains Flush 同步排空语义直测（集成测试免 sleep 轮询的基础）：
// Enqueue 数条 → Flush(ctx) 返回后 fake sink 已收到全部 entry 且 chan 排空。
func TestFlushDrains(t *testing.T) {
	var got []po.OperationLogPO
	w := newTestWriter(t, opLogChanSize, func(ctx context.Context, entries []po.OperationLogPO) error {
		got = append(got, entries...) // Flush 返回经 chan close happens-before，读安全
		return nil
	})
	w.Start()

	for i := 0; i < 5; i++ {
		op := fmt.Sprintf("op%d", i)
		w.Enqueue(&po.OperationLogPO{Operation: &op})
	}
	require.NoError(t, w.Flush(context.Background()))

	assert.Len(t, got, 5, "Flush 返回后全部 entry 必须已落库")
	for i, e := range got {
		assert.Equal(t, fmt.Sprintf("op%d", i), *e.Operation)
	}
	assert.Equal(t, 0, len(w.ch), "Flush 后 chan 必须已排空")
}

// TestFlushSinkErrorNotPanic 落库失败仅 Warn 不反压（prohibition）：sink 报错时
// Flush 仍正常返回、批清空、不 panic。
func TestFlushSinkErrorNotPanic(t *testing.T) {
	w := newTestWriter(t, opLogChanSize, func(ctx context.Context, entries []po.OperationLogPO) error {
		return fmt.Errorf("db boom")
	})
	w.Start()
	op := "op"
	w.Enqueue(&po.OperationLogPO{Operation: &op})
	assert.NoError(t, w.Flush(context.Background()), "落库失败不得影响 Flush 返回（静默语义）")
	assert.Equal(t, 0, len(w.ch), "失败批必须清空（丢弃，不反压）")
}

// runOplog 驱动真实中间件链：handlers[0]=Oplog、后续为测试自定义下游（写响应/
// resp.Fail），跑完后 Flush 同步落库并返回组装 entry（Flush 返回即 happens-before，
// 读 sink 捕获结果无竞态）。前置 handlers 可选（如 identity 注入中间件）。
func runOplogChain(t *testing.T, w *OpLogWriter, module, operation, method, path, contentType, body string, downstream gin.HandlerFunc, front ...gin.HandlerFunc) []po.OperationLogPO {
	t.Helper()
	var got []po.OperationLogPO
	w.sink = func(ctx context.Context, entries []po.OperationLogPO) error {
		got = append(got, entries...)
		return nil
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	chain := append(append([]gin.HandlerFunc{}, front...), Oplog(w, module, operation))
	if downstream != nil {
		chain = append(chain, downstream)
	}
	router.Handle(method, path, chain...)
	w.Start() // worker 必须在跑——Flush 同步语义依赖 flushCh 消费者

	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.RemoteAddr = "192.168.1.10:54321"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.NoError(t, w.Flush(context.Background()))
	return got
}

// runOplog 单中间件链便捷变体（module/operation 固定 TestController/create）。
func runOplog(t *testing.T, w *OpLogWriter, method, path, contentType, body string, downstream gin.HandlerFunc) []po.OperationLogPO {
	return runOplogChain(t, w, "TestController", "create", method, path, contentType, body, downstream)
}

// TestOplogEntryAssembly 七要素组装直测：module/operation 路由声明值、
// method 拼接形态、params 为掩码后 body JSON 回放、ip 取 host、status=1；且
// handler 可重读 body（回放直证）；未注入 identity 时 UserID/RealName 为 nil。
func TestOplogEntryAssembly(t *testing.T) {
	var downstreamBody string
	got := runOplog(t, newTestWriter(t, opLogChanSize, nil), http.MethodPost,
		"/api/v1/dict-types", "application/json", `{"dictName":"状态","password":"p1"}`,
		func(c *gin.Context) {
			// handler 重读 body（回放直证——「body 回放不破坏 handler」）
			raw, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			downstreamBody = string(raw)
			c.Status(http.StatusOK)
		})

	require.Len(t, got, 1, "落库行缺失")
	e := got[0]
	require.NotNil(t, e.Module)
	assert.Equal(t, "TestController", *e.Module)
	require.NotNil(t, e.Operation)
	assert.Equal(t, "create", *e.Operation)
	require.NotNil(t, e.Method)
	assert.Equal(t, "POST /api/v1/dict-types", *e.Method)
	require.NotNil(t, e.Params)
	assert.Contains(t, *e.Params, `"dictName":"状态"`, "params 必须回放 body JSON")
	assert.Contains(t, *e.Params, `"password":"******"`, "params 敏感值必须已掩码")
	assert.NotContains(t, *e.Params, `"p1"`)
	require.NotNil(t, e.IP)
	assert.Equal(t, "192.168.1.10", *e.IP, "ip 必须取 host（SplitHostPort）")
	require.NotNil(t, e.Status)
	assert.Equal(t, int8(1), *e.Status, "200 请求 status=1")
	assert.Nil(t, e.ErrorMsg, "成功请求 error_msg 为 NULL（DB 列缺省语义）")
	require.NotNil(t, e.Duration)
	assert.GreaterOrEqual(t, *e.Duration, 0)
	assert.Nil(t, e.UserID, "未注入 identity 的 ctx 落 NULL（未登录语义）")
	assert.Nil(t, e.RealName)
	assert.Equal(t, `{"dictName":"状态","password":"p1"}`, downstreamBody, "handler 必须能重读完整 body")
}

// TestOplogIdentityFromContext identity 注入时 UserID/RealName 取会话值
// （认证中间件 WithIdentity 前置注入形态）。
func TestOplogIdentityFromContext(t *testing.T) {
	var got []po.OperationLogPO
	w := newTestWriter(t, opLogChanSize, func(ctx context.Context, entries []po.OperationLogPO) error {
		got = append(got, entries...)
		return nil
	})

	injectIdentity := func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithIdentity(c.Request.Context(), auth.Identity{UserID: 42, Name: "管理员"}))
		c.Next()
	}
	got = runOplogChain(t, w, "UserController", "update", http.MethodPost, "/api/v1/x",
		"application/json", `{}`, func(c *gin.Context) { c.Status(http.StatusOK) }, injectIdentity)

	require.Len(t, got, 1)
	require.NotNil(t, got[0].UserID)
	assert.Equal(t, int64(42), *got[0].UserID)
	require.NotNil(t, got[0].RealName)
	assert.Equal(t, "管理员", *got[0].RealName)
}

// TestOplogFailedRequestStatusZero 失败请求（400 + resp.Fail c.Error 透出）：
// status=0 且 error_msg 为错误消息（失败也落库）。
func TestOplogFailedRequestStatusZero(t *testing.T) {
	got := runOplog(t, newTestWriter(t, opLogChanSize, nil), http.MethodPost,
		"/api/v1/dict-types", "application/json", `{}`,
		func(c *gin.Context) {
			resp.Fail(c, errcode.New(errcode.CodeInvalidParam, "字典类型已存在"))
		})

	require.Len(t, got, 1, "失败请求也必须落库")
	require.NotNil(t, got[0].Status)
	assert.Equal(t, int8(0), *got[0].Status, "400 请求 status=0")
	require.NotNil(t, got[0].ErrorMsg)
	assert.Equal(t, "字典类型已存在", *got[0].ErrorMsg)
}

// TestOplogErrorMsgTruncatedTo500Runes errorMsg 超 500 rune 截断；
// resp.Fail 是唯一 error 注入点（gin errors 最后一条）。
func TestOplogErrorMsgTruncatedTo500Runes(t *testing.T) {
	long := strings.Repeat("错", 600) // 600 rune > 500
	got := runOplog(t, newTestWriter(t, opLogChanSize, nil), http.MethodPost,
		"/api/v1/x", "application/json", `{}`,
		func(c *gin.Context) {
			resp.Fail(c, errcode.New(errcode.CodeInternal, long))
		})

	require.Len(t, got, 1)
	require.NotNil(t, got[0].ErrorMsg)
	runes := []rune(*got[0].ErrorMsg)
	assert.Len(t, runes, errorMsgMaxRunes, "errorMsg 必须按 rune 截 500")
	assert.True(t, strings.HasPrefix(*got[0].ErrorMsg, strings.Repeat("错", errorMsgMaxRunes)))
}

// TestOplogPanicPathRecorded handler 链 panic 穿透时 entry 仍组装（defer 语义）：status=0 + 固定 errorMsg；panic 继续上抛（不吞——Recovery 兜底）。
func TestOplogPanicPathRecorded(t *testing.T) {
	var got []po.OperationLogPO
	w := newTestWriter(t, opLogChanSize, func(ctx context.Context, entries []po.OperationLogPO) error {
		got = append(got, entries...)
		return nil
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	w.Start() // Flush 同步语义依赖 worker 消费者
	router.POST("/api/v1/x", Oplog(w, "UserController", "create"),
		func(c *gin.Context) { panic("boom") })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.10:54321"
	assert.Panics(t, func() { router.ServeHTTP(httptest.NewRecorder(), req) },
		"Oplog 不得吞 panic（上游 Recovery 兜底契约）")
	require.NoError(t, w.Flush(context.Background()))

	require.Len(t, got, 1, "panic 请求也必须落库（源 finally 语义）")
	require.NotNil(t, got[0].Status)
	assert.Equal(t, int8(0), *got[0].Status)
	require.NotNil(t, got[0].ErrorMsg)
	assert.Equal(t, "handler panic", *got[0].ErrorMsg)
}

// TestOplogMultipartSkipsCache multipart 请求不缓存 body（prohibition：20MB 上传
// 缓存造成内存放大）——params 记空串、body 不被读穿（handler 仍可完整读取）。
func TestOplogMultipartSkipsCache(t *testing.T) {
	got := runOplog(t, newTestWriter(t, opLogChanSize, nil), http.MethodPost,
		"/api/v1/files/upload", "multipart/form-data; boundary=----x",
		"----x\r\nbinary-ish payload\r\n----x--\r\n",
		func(c *gin.Context) {
			// multipart 下 body 保持可读（未被中间件消费缓存）
			raw, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			assert.NotEmpty(t, raw, "multipart body 被中间件读穿缓存（prohibition）")
			c.Status(http.StatusOK)
		})

	require.Len(t, got, 1)
	require.NotNil(t, got[0].Params)
	assert.Empty(t, *got[0].Params, "multipart params 必须记空串（源过滤跳过 MultipartFile）")
}

// TestOplogMaxBytesReplayRoundTrip body 超限时 Oplog 读到 *http.MaxBytesError，
// 回放 errReader 保语义——下游 handler 读 body 仍得到同一 MaxBytesError（既有
// 10401 固定文案路径不因中间件读穿而降级为 10400 参数校验文案）。
func TestOplogMaxBytesReplayRoundTrip(t *testing.T) {
	var got []po.OperationLogPO
	w := newTestWriter(t, opLogChanSize, func(ctx context.Context, entries []po.OperationLogPO) error {
		got = append(got, entries...)
		return nil
	})

	gin.SetMode(gin.TestMode)
	var downstreamErr error
	router := gin.New()
	w.Start() // Flush 同步语义依赖 worker 消费者
	// 前置手动包 MaxBytesReader（模拟全局 BodyLimit 中间件效果，limit 1024）
	router.POST("/api/v1/x",
		func(c *gin.Context) {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
			c.Next()
		},
		Oplog(w, "UserController", "create"),
		func(c *gin.Context) {
			_, err := io.ReadAll(c.Request.Body)
			downstreamErr = err
			c.Status(http.StatusOK)
		})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", strings.NewReader(strings.Repeat("A", 8192)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	router.ServeHTTP(httptest.NewRecorder(), req)
	require.NoError(t, w.Flush(context.Background()))

	var mbe *http.MaxBytesError
	require.ErrorAs(t, downstreamErr, &mbe, "下游必须仍读到 MaxBytesError（10401 语义保真）")
	require.Len(t, got, 1, "超限请求本身也落库（params 为已读部分掩码结果）")
}
