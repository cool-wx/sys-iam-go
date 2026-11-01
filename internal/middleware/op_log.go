// Package middleware 操作日志中间件（Phase 4 OLOG-01/02，等价源 OperationLogAspect）：
// 声明制挂载（D-05：路由注册处显式声明 module/operation，等价源 @OperationLog
// 注解心智模型）+ body 缓存回放（D-06）+ chan 异步落库（D-07）+ 优雅关停 drain
// （D-08）+ 8 敏感字段掩码（D-09）。
//
// 有意偏离源两处（04-CONTEXT.md D-07/D-09 留档）：
//   - 源为同步落库（finally 中 save）→ 本版 chan 异步：chan 满丢弃优于请求阻塞
//     （丢弃是设计而非缺陷——prohibition：Enqueue 绝不阻塞业务请求）；
//   - 源 AOP 切点含读操作 → 本版仅显式声明的写操作路由记录（D-05 收紧）。
//
// 威胁缓解面：T-4-07 掩码先于截断（截断不切出掩码边界新明文）；T-4-08 chan 满
// select-default 丢弃 + worker 落库错误仅 Warn 不反压 + 容量 512 × params 截断
// 60KB 联合封顶内存 ≈30MB（工控机可接受）；T-4-09 Go regexp 为 RE2 线性时间
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

// 异步链参数（D-07/T-4-08）：容量 512 × entry params 截断后 ≤60KB ≈ 30MB 内存
// 上界（工控机可接受）；批满 64 条或 500ms 到期即批量落库。
const (
	opLogChanSize      = 512
	opLogBatchSize     = 64
	opLogFlushInterval = 500 * time.Millisecond
	// Stop/Flush 落库排空预算：app.Stop 在 15s 关停预算内独立计时（D-08 次序
	// 中的 drain 段），通常毫秒级完成。
	opLogFlushTimeout = 5 * time.Second
	// params 按字节截断上限：MySQL TEXT 64KB 边界内（严格模式超长 INSERT 直接
	// 报错会整条丢日志，故先掩码后截断——顺序 prohibition）。
	paramsMaxBytes = 60_000
	// errorMsg 按 rune 截断上限（源 substring(0, min(500, len)) 语义）。
	errorMsgMaxRunes = 500
)

// sensitiveRe 掩码正则——**逐字照源 OperationLogAspect.java:27-28**：捕获组为
// password/oldPassword/newPassword/secretKey/apiKey/token/accessToken/refreshToken
// 八字段名（D-09 研究修正：REQUIREMENTS「7 字段」为计数笔误，裁定记录以源正则
// 8 字段为准），只匹配带双引号的字符串值形态（数字/布尔/null 值不匹配——源语义）。
// RE2 线性引擎无回溯 ReDoS 面（T-4-09 disposition 依据）。
var sensitiveRe = regexp.MustCompile(`"(password|oldPassword|newPassword|secretKey|apiKey|token|accessToken|refreshToken)"\s*:\s*"[^"]*"`)

// maskReplacement 替换值：键名保留（$1）、值统一 ******（源 ReplaceAllString 语义）。
const maskReplacement = `"${1}":"******"`

// MaskSensitive 掩码 JSON 文本中 8 个敏感字段的字符串值为 ******。非 JSON 输入
//（如空串/multipart 空参数）原样返回。掩码必须先于任何截断调用（prohibition：
// 截断会把文本切出新边界，若先截后掩码则切点处可能出现未掩码明文残余）。
func MaskSensitive(jsonStr string) string {
	return sensitiveRe.ReplaceAllString(jsonStr, maskReplacement)
}

// Sink 批量落库函数签名（可注入依赖）：生产装配传 repo.InsertBatch，测试传
// fake 计数器（Flush drain 语义免容器直测）。
type Sink func(ctx context.Context, entries []po.OperationLogPO) error

// OpLogWriter 操作日志异步写通道（D-07）：buffered chan + 单 worker 批量落库。
type OpLogWriter struct {
	ch       chan *po.OperationLogPO
	flushCh  chan chan struct{}
	quit     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
	started  atomic.Bool  // worker 启动标志——Stop 在未 Start 时免 Flush（无消费者会超时空等）
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

// Enqueue 非阻塞入队：chan 满即丢弃并 Warn 计数（prohibition：绝不阻塞业务请求
// ——丢弃是 D-07 设计而非缺陷；worker 落库错误同样仅 Warn 不反压）。
func (w *OpLogWriter) Enqueue(entry *po.OperationLogPO) {
	select {
	case w.ch <- entry:
	default:
		w.logger.Warn("oplog chan full, entry dropped", "dropped_total", w.dropped.Add(1))
	}
}

// Flush 同步排空（D-08 flush 语义，供 app.Stop 与集成测试两用）：请求 worker
// 「drain chan 残留 + 落当前批 + 等待落库完成」后返回——测试无需 sleep 轮询。
// 保证：调用时**已入 chan** 的 entry 全部落库；Flush 期间并发 Enqueue 的新 entry
// 属竞态窗口（Stop 场景流量已停于 srv.Shutdown 之后，语义严格成立——D-08 次序
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
//（stopOnce），可安全接入 app.Stop 与测试 Cleanup。worker 未启动（如仅入队
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
// 先 drain chan 残留再落批（D-08 flush 语义的实现点）；quit 分支终局 drain 后退出。
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

// Oplog 操作日志中间件（D-05 声明制挂载）：module/operation 由路由注册处显式
// 声明（等价源 @OperationLog 注解元数据）；挂载次序固定在 RequirePerm 之后
// handler 之前（鉴权失败不产生日志——与源 AOP 在授权后切入语义一致）。
//
// body 处理（D-06）：仅非 multipart Content-Type 缓存 body 并 NopCloser 回放
// （handler 可重读）；multipart 跳过缓存、params 记空串——对齐源 params 过滤
// 跳过 MultipartFile，且避免 20MB 上传全量缓存造成内存放大（prohibition）。
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
			// defer 组装（源 finally save 语义）：handler 链 panic 穿透时也记录
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
	// identity 从请求 ctx 取（认证中间件注入；未登录路由如 login 自身为 nil——源语义）
	if id, ok := auth.FromContext(c.Request.Context()); ok {
		uid, name := id.UserID, id.Name
		entry.UserID = &uid
		entry.RealName = &name
	}
	// method 形如 "POST /api/v1/dict-types"（源 request.getMethod()+" "+getRequestURI）
	method := c.Request.Method + " " + c.Request.URL.Path
	entry.Method = &method
	// params：multipart 记空串（源过滤跳过 MultipartFile）；否则掩码先于截断
	params := ""
	if !isMultipart {
		params = truncateBytes(MaskSensitive(string(cached)), paramsMaxBytes)
	}
	entry.Params = &params
	// ip：RemoteAddr 经 SplitHostPort 取 host（失败原样存——源 getRemoteAddr 纯
	// IP 形态对齐，Open Question Q4 拍板源保真不用 ClientIP/XFF）
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
// 最近完整 rune 边界（WR-01）：params 列为 utf8mb4，硬切多字节字符中间会产生
// 无效 UTF-8 尾串，go-sql-driver 绑定后 MySQL strict mode 报 1366 Incorrect
// string value → CreateInBatches 整批 INSERT 失败、同批（最多 64 条）无关日志
// 一并丢弃——回退后恒为合法 UTF-8，落库不再被确定性打爆。调用方必须保证已先
// 掩码（截断先于掩码会切出掩码边界新明文——prohibition 顺序锁定）。
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// truncateRunes 按 rune 截断（errorMsg 500——源 substring 按 char 语义对齐）。
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
