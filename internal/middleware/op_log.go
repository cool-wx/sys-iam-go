// Package middleware 操作日志中间件：
// 声明制挂载（路由注册处显式声明 module/operation）+ body 缓存回放 +
// chan 异步落库 + 优雅关停 drain
// + 8 敏感字段掩码。
//
// 设计要点两处：
//   - chan 异步落库：chan 满丢弃优于请求阻塞
//     （丢弃是设计而非缺陷——纪律：Enqueue 绝不阻塞业务请求）；
//   - 仅显式声明的写操作路由记录（读操作不记）。
//
// 威胁缓解面：掩码先于截断（截断不切出掩码边界新明文）；chan 满
// select-default 丢弃 + worker 落库错误仅 Warn 不反压 + 容量 512 × params 截断
// 60KB 联合封顶内存 ≈30MB（工控机可接受）；Go regexp 为 RE2 线性时间
// 引擎（无回溯指数路径），掩码模式结构性免疫 ReDoS。
package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/model/po"
)

// 异步链参数：容量 512 × entry params 截断后 ≤60KB ≈ 30MB 内存
// 上界（工控机可接受）；批满 64 条或 500ms 到期即批量落库。
const (
	opLogChanSize      = 512
	opLogBatchSize     = 64
	opLogFlushInterval = 500 * time.Millisecond
	// Stop/Flush 落库排空预算：app.Stop 在 15s 关停预算内独立计时（次序
	// 中的 drain 段），通常毫秒级完成。
	opLogFlushTimeout = 5 * time.Second
	// params 按字节截断上限：MySQL TEXT 64KB 边界内（严格模式超长 INSERT 直接
	// 报错会整条丢日志，故先掩码后截断——顺序固定）。
	paramsMaxBytes = 60_000
	// errorMsg 按 rune 截断上限（500 rune）。
	errorMsgMaxRunes = 500
)

// sensitiveRe 掩码正则：捕获组为
// password/oldPassword/newPassword/secretKey/apiKey/token/accessToken/refreshToken
// 八字段名，只匹配带双引号的字符串值形态（数字/布尔/null 值不匹配，不掩码）。
// RE2 线性引擎无回溯 ReDoS 面（结构性免疫依据）。
var sensitiveRe = regexp.MustCompile(`"(password|oldPassword|newPassword|secretKey|apiKey|token|accessToken|refreshToken)"\s*:\s*"[^"]*"`)

// maskReplacement 替换值：键名保留（$1）、值统一 ******。
const maskReplacement = `"${1}":"******"`

// MaskSensitive 掩码 JSON 文本中 8 个敏感字段的字符串值为 ******。非 JSON 输入
// （如空串/multipart 空参数）原样返回。掩码必须先于任何截断调用（prohibition：
// 截断会把文本切出新边界，若先截后掩码则切点处可能出现未掩码明文残余）。
func MaskSensitive(jsonStr string) string {
	return sensitiveRe.ReplaceAllString(jsonStr, maskReplacement)
}

// Sink 批量落库函数签名（可注入依赖）：生产装配传 repo.InsertBatch，测试传
// fake 计数器（Flush drain 语义免容器直测）。
type Sink func(ctx context.Context, entries []po.OperationLogPO) error

// OpLogWriter 操作日志异步写通道：buffered chan + 单 worker 批量落库。
type OpLogWriter struct {
	ch       chan *po.OperationLogPO
	flushCh  chan chan struct{}
	quit     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
	started  atomic.Bool // worker 启动标志——Stop 在未 Start 时免 Flush（无消费者会超时空等）
	sink     Sink
	logger   *slog.Logger
	dropped  atomic.Int64 // chan 满丢弃累计计数（可观测性；丢弃不反压——prohibition）
}

// NewOpLogWriter 构造（默认 sink = db.CreateInBatches 直写 sys_operation_log；
// 生产装配建议 NewOpLogWriterWithSink 注入 repo.InsertBatch 以收口数据访问层）。
func NewOpLogWriter(db *gorm.DB, logger *slog.Logger) *OpLogWriter {
	return NewOpLogWriterWithSink(func(ctx context.Context, entries []po.OperationLogPO) error {
		return db.WithContext(ctx).CreateInBatches(&entries, len(entries)).Error
	}, logger)
}

// NewOpLogWriterWithSink 注入落库函数的构造（测试 fake / repo.InsertBatch 装配）。
func NewOpLogWriterWithSink(sink Sink, logger *slog.Logger) *OpLogWriter {
	if logger == nil {
		logger = slog.Default()
	}
	return &OpLogWriter{
		ch:      make(chan *po.OperationLogPO, opLogChanSize),
		flushCh: make(chan chan struct{}, 1),
		quit:    make(chan struct{}),
		sink:    sink,
		logger:  logger,
	}
}

// Start 启动单 worker goroutine（app.New 装配处调用；重复调用会 panic 于 wg.Add
// after Wait——本类型约定 Start 一次 + Stop 一次的生命周期，app 单实例语义）。
func (w *OpLogWriter) Start() {
	w.started.Store(true)
	w.wg.Add(1)
	go w.worker()
}

// Enqueue 非阻塞入队：chan 满即丢弃并 Warn 计数（纪律：绝不阻塞业务请求
// ——丢弃是设计而非缺陷；worker 落库错误同样仅 Warn 不反压）。
func (w *OpLogWriter) Enqueue(entry *po.OperationLogPO) {
	select {
	case w.ch <- entry:
	default:
		w.logger.Warn("oplog chan full, entry dropped", "dropped_total", w.dropped.Add(1))
	}
}

// Flush 同步排空（flush 语义，供 app.Stop 与集成测试两用）：请求 worker
// 「drain chan 残留 + 落当前批 + 等待落库完成」后返回——测试无需 sleep 轮询。
// 保证：调用时**已入 chan** 的 entry 全部落库；Flush 期间并发 Enqueue 的新 entry
// 属竞态窗口（Stop 场景流量已停于 srv.Shutdown 之后，语义严格成立——次序
// 正是为此）。
func (w *OpLogWriter) Flush(ctx context.Context) error {
	done := make(chan struct{})
	select {
	case w.flushCh <- done:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop = Flush（预算内排空）+ 终止 worker（终局再 drain 一遍兜底）。幂等
// （stopOnce），可安全接入 app.Stop 与测试 Cleanup。worker 未启动（如仅入队
// 丢弃路径的单元测试）时跳过 Flush——flushCh 无消费者，同步请求只会空等超时。
func (w *OpLogWriter) Stop() {
	w.stopOnce.Do(func() {
		if w.started.Load() {
			ctx, cancel := context.WithTimeout(context.Background(), opLogFlushTimeout)
			defer cancel()
			_ = w.Flush(ctx)
		}
		close(w.quit)
		w.wg.Wait()
	})
}

// worker 单消费循环：攒批 64 条或 500ms ticker 到期即批量落库；flush 请求分支
// 先 drain chan 残留再落批（flush 语义的实现点）；quit 分支终局 drain 后退出。
func (w *OpLogWriter) worker() {
	defer w.wg.Done()
	ticker := time.NewTicker(opLogFlushInterval)
	defer ticker.Stop()
	batch := make([]po.OperationLogPO, 0, opLogBatchSize)
	for {
		select {
		case entry := <-w.ch:
			batch = append(batch, *entry)
			if len(batch) >= opLogBatchSize {
				w.flushBatch(context.Background(), &batch)
			}
		case done := <-w.flushCh:
			w.drainChan(&batch)
			w.flushBatch(context.Background(), &batch)
			close(done)
		case <-ticker.C:
			w.flushBatch(context.Background(), &batch)
		case <-w.quit:
			w.drainChan(&batch)
			w.flushBatch(context.Background(), &batch)
			return
		}
	}
}

// drainChan 非阻塞排空 chan 残留入批（ Flush/quit 路径用；恒终止——default 收口）。
func (w *OpLogWriter) drainChan(batch *[]po.OperationLogPO) {
	for {
		select {
		case entry := <-w.ch:
			*batch = append(*batch, *entry)
			if len(*batch) >= opLogBatchSize {
				w.flushBatch(context.Background(), batch)
			}
		default:
			return
		}
	}
}

// flushBatch 批量落库：空批直返；失败仅 slog.Warn（不反压不 panic——prohibition）；
// 批无论成败都清空（丢弃失败批，绝不阻塞消费循环）。
func (w *OpLogWriter) flushBatch(ctx context.Context, batch *[]po.OperationLogPO) {
	if len(*batch) == 0 {
		return
	}
	if err := w.sink(ctx, *batch); err != nil {
		w.logger.Warn("oplog flush batch failed, entries dropped", "count", len(*batch), "error", err.Error())
	}
	*batch = (*batch)[:0]
}

// errReader 读即返回固定错误的 reader：Oplog 缓存 body 读到 *http.MaxBytesError
// 时回放它——bindJSON 再次读到同一错误 → 既有 10401 固定文案路径（超限请求行为
// 与未挂 Oplog 时逐字节一致，不因中间件读穿而降级为 10400 参数校验文案）。
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// Oplog 操作日志中间件（声明制挂载）：module/operation 由路由注册处显式
// 声明；挂载次序固定在 RequirePerm 之后
// handler 之前（鉴权失败不产生日志——日志仅记录已授权的写操作）。
//
// body 处理：仅非 multipart Content-Type 缓存 body 并 NopCloser 回放
// （handler 可重读）；multipart 跳过缓存、params 记空串——multipart 内容
// 不进 params，且避免 20MB 上传全量缓存造成内存放大（纪律要求）。
func Oplog(w *OpLogWriter, module, operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		isMultipart := strings.HasPrefix(c.ContentType(), "multipart/")
		var cached []byte
		if !isMultipart && c.Request.Body != nil {
			b, err := io.ReadAll(c.Request.Body)
			cached = b // 出错时为已读部分（MaxBytesError 时走 errReader 回放保语义）
			if err != nil {
				var mbe *http.MaxBytesError
				if errors.As(err, &mbe) {
					c.Request.Body = io.NopCloser(errReader{err: mbe})
				} else {
					c.Request.Body = io.NopCloser(bytes.NewReader(cached))
				}
			} else {
				c.Request.Body = io.NopCloser(bytes.NewReader(cached))
			}
		}
		success := false
		defer func() {
			// defer 组装：handler 链 panic 穿透时也记录
			//（success 恒 false → status=0 + 固定 errorMsg；响应由上游 Recovery 兜底）。
			w.Enqueue(assembleEntry(c, start, module, operation, cached, isMultipart, success))
		}()
		c.Next()
		success = true
	}
}

// assembleEntry c.Next() 之后组装落库 entry（中间件核心组装逻辑抽函数便于直测）。
func assembleEntry(c *gin.Context, start time.Time, module, operation string, cached []byte, isMultipart, success bool) *po.OperationLogPO {
	entry := &po.OperationLogPO{
		Module:    &module,
		Operation: &operation,
	}
	// identity 从请求 ctx 取（认证中间件注入；未登录路由如 login 自身为 nil）
	if id, ok := auth.FromContext(c.Request.Context()); ok {
		uid, name := id.UserID, id.Name
		entry.UserID = &uid
		entry.RealName = &name
	}
	// method 形如 "POST /api/v1/dict-types"（HTTP 方法 + 请求路径拼接）
	method := c.Request.Method + " " + c.Request.URL.Path
	entry.Method = &method
	// params：multipart 记空串（multipart 内容不进 params）；否则掩码先于截断
	params := ""
	if !isMultipart {
		params = truncateBytes(MaskSensitive(string(cached)), paramsMaxBytes)
	}
	entry.Params = &params
	// ip：RemoteAddr 经 SplitHostPort 取 host（失败原样存，仅取纯 IP 部分、
	// 不用 ClientIP/XFF 代理链解析）
	ip := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	entry.IP = &ip
	duration := int(time.Since(start).Milliseconds())
	entry.Duration = &duration
	// status/errorMsg：正常路径按 HTTP 状态判定 + c.Errors 取错误（resp.Fail 透出）；
	// panic 穿透路径 success=false → status=0 + 固定文案
	status := int8(1)
	if !success {
		status = 0
		m := "handler panic"
		entry.ErrorMsg = &m
	} else {
		if c.Writer.Status() >= 400 {
			status = 0
		}
		if len(c.Errors) > 0 {
			if last := c.Errors[len(c.Errors)-1]; last.Err != nil {
				m := truncateRunes(last.Err.Error(), errorMsgMaxRunes)
				entry.ErrorMsg = &m
			}
		}
	}
	entry.Status = &status
	return entry
}

// truncateBytes 按字节截断（params 上限 60000——TEXT 64KB 边界内），切点回退至
// 最近完整 rune 边界：params 列为 utf8mb4，硬切多字节字符中间会产生
// 无效 UTF-8 尾串，go-sql-driver 绑定后 MySQL strict mode 报 1366 Incorrect
// string value → CreateInBatches 整批 INSERT 失败、同批（最多 64 条）无关日志
// 一并丢弃——回退后恒为合法 UTF-8，落库不再被确定性打爆。调用方必须保证已先
// 掩码（截断先于掩码会切出掩码边界新明文——顺序锁定）。
//
// 复杂度纪律：回退判定用 DecodeLastRuneInString 尾部解码（O(UTFMax)
// 常数窗口），绝不做「剥 1 字节重扫全串」——那对输入本身含非法字节的 body 是
// O(n²)（授权用户恶意构造 >60KB 非法 body 在请求 goroutine defer 中同步执行，
// iter-2 实测 ~18.8ms/请求）。全路径至多 2 次 O(n) 扫描（ValidString + 首个
// 非法点定位）：合法输入的切点碎片至多 3 字节，尾部解码不完整即逐字节回退，
// ≤3 次迭代达边界；输入本身非法时一次 range 定位首个非法序列起点直接截断
// （其后的合法内容一并丢弃——params 是请求原文日志，非法字节来自畸形请求，
// 截至首个非法点语义无损）。尾部解码须用「RuneError 且 size==1」判定不完整：
// 合法编码的 U+FFFD（�）解码结果同为 RuneError 但 size==3，不得误剥。
// 注：iter-2 REVIEW 建议的 RuneStart 逐字节回退会把「恰在 rune 边界」的
// continuation 结尾误剥成不完整序列，故改用尾部完整解码判定。
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	// 尾部解码回退至完整 rune 边界：尾部是完整 rune（ASCII / 恰在边界的多字节
	// 结尾 / 合法 U+FFFD）不剥；不完整切点碎片逐字节回退（≤3 次迭代）。
	for len(s) > 0 {
		if r, size := utf8.DecodeLastRuneInString(s); r != utf8.RuneError || size > 1 {
			break
		}
		s = s[:len(s)-1]
	}
	if utf8.ValidString(s) {
		return s
	}
	// 输入本身非法（非切点问题）：一次扫描定位首个非法序列起点，O(n) 截断，
	// 不再逐字节回退全量重扫（CPU 放大面）。
	for i, r := range s {
		if r == utf8.RuneError {
			if _, size := utf8.DecodeRuneInString(s[i:]); size == 1 {
				return s[:i]
			}
		}
	}
	return s
}

// truncateRunes 按 rune 截断（errorMsg 上限 500 rune）。
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
