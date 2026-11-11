// Package app 聚合应用依赖与生命周期：装配（config→GORM→路由）→ Run 阻塞服务 →
// Stop 摘流+排空+关连接（供 main 信号路径与关停测试复用）。
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/cool-wx/sys-iam-go/internal/auth"
	"github.com/cool-wx/sys-iam-go/internal/authz"
	"github.com/cool-wx/sys-iam-go/internal/config"
	"github.com/cool-wx/sys-iam-go/internal/event"
	"github.com/cool-wx/sys-iam-go/internal/handler"
	"github.com/cool-wx/sys-iam-go/internal/health"
	"github.com/cool-wx/sys-iam-go/internal/middleware"
	"github.com/cool-wx/sys-iam-go/internal/repository"
	"github.com/cool-wx/sys-iam-go/internal/service"
	"github.com/cool-wx/sys-iam-go/internal/storage"
)

// 优雅关停排空预算（systemd TimeoutStopSec 须大于此值）。
const shutdownTimeout = 15 * time.Second

// App 应用聚合：engine、db、rdb、health、config、ologW（操作日志异步写通道）。
type App struct {
	cfg    *config.Config
	logger *slog.Logger
	engine *gin.Engine
	db     *gorm.DB
	rdb    *redis.Client
	health *health.State
	srv    *http.Server
	ologW  *middleware.OpLogWriter
}

// New 装配全部依赖：GORM 连接（绝不 AutoMigrate，schema 沿用既有库）、连接池
// 工控机参数、Redis 会话存储（Ping fail-fast）、健康状态、认证路由。成功返回可
// Run 的 App。
func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Warn),
		SkipDefaultTransaction: true, // service 层显式 Transaction，关隐式事务
		TranslateError:         true, // MySQL 1062 → gorm.ErrDuplicatedKey
		// 不配置 AutoMigrate —— schema 沿用既有库，禁止运行期改表
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.MySQL.ConnMaxLifetime)

	h := health.New(sqlDB)

	// Redis 会话存储（go-redis v9；启动 Ping fail-fast 对齐 app.New 的 err 返回风格
	// ——会话基建不可用时服务绝不能以"全量 401"状态静默启动）
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = rdb.Ping(pingCtx).Err()
	cancel()
	if err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	// Casbin enforcer 装配：复用既有 GORM 连接；初始化与
	// LoadPolicy 失败 fail-fast return nil, error——授权基建不可用时服务绝不能
	// 以"全量 403"状态静默启动（对齐上方 Redis Ping 先例）。enforcer 是
	// requirePerm 中间件与 roleSvc/userSvc 的依赖，必须在 service 构造之前建好。
	enf, err := authz.New(db)
	if err != nil {
		return nil, fmt.Errorf("init casbin enforcer: %w", err)
	}

	// boot 对账（授权落地前提）：casbin_rule 新表初始为空而存量
	// sys_user_role/sys_role_menu 已有数据——启动全量导入 g/p 策略（ReplaceAll
	// 幂等可重入），存量 admin 冷启动即可用；同时修复运行期同步失败残留（重启
	// 自愈）。对账失败 fail-fast 拒绝启动（绝不全量 403 状态静默启动）。
	relationRepo := repository.NewRelationRepo(db)
	reconcileCtx, reconcileCancel := context.WithTimeout(context.Background(), 10*time.Second)
	userRules, err := relationRepo.FindUserRoleCodes(reconcileCtx)
	if err == nil {
		var rolePerms []repository.RolePermRule
		rolePerms, err = relationRepo.FindRolePerms(reconcileCtx)
		if err == nil {
			pRules := make([][]string, 0, len(rolePerms))
			seenP := make(map[string]struct{}, len(rolePerms))
			for _, rp := range rolePerms {
				key := rp.RoleCode + "\x00" + rp.Perm
				if _, dup := seenP[key]; dup {
					continue
				}
				seenP[key] = struct{}{}
				pRules = append(pRules, []string{rp.RoleCode, rp.Perm})
			}
			gRules := make([][]string, 0, len(userRules))
			seenG := make(map[string]struct{}, len(userRules))
			for _, ur := range userRules {
				uidStr := strconv.FormatInt(ur.UserID, 10)
				key := uidStr + "\x00" + ur.RoleCode
				if _, dup := seenG[key]; dup {
					continue
				}
				seenG[key] = struct{}{}
				gRules = append(gRules, []string{uidStr, ur.RoleCode})
			}
			err = enf.ReplaceAll(pRules, gRules)
		}
	}
	reconcileCancel()
	if err != nil {
		return nil, fmt.Errorf("boot reconcile casbin policies: %w", err)
	}

	sessionMgr := auth.NewSessionManager(rdb, cfg.Auth.SessionTimeout)

	// FileStorage 构造：插在 service 构造之前（与 enforcer/
	// Redis Ping fail-fast 同一纪律，存储基建不可用拒绝启动）；driver=local 时构造链
	// 不触碰 minio（延迟形态），driver=s3 时构造内 EnsureBucket fail-fast。
	storageCtx, storageCancel := context.WithTimeout(context.Background(), 10*time.Second)
	fileStorage, err := storage.New(storageCtx, cfg.Storage)
	storageCancel()
	if err != nil {
		return nil, fmt.Errorf("init file storage: %w", err)
	}

	userRepo := repository.NewUserRepo(db)
	apiKeyRepo := repository.NewApiKeyRepo(db)
	menuRepo := repository.NewMenuRepo(db)
	deptRepo := repository.NewDeptRepo(db)
	roleRepo := repository.NewRoleRepo(db)
	dictTypeRepo := repository.NewDictTypeRepo(db)
	dictDataRepo := repository.NewDictDataRepo(db)
	configRepo := repository.NewSysConfigRepo(db)
	shiftRepo := repository.NewShiftRepo(db)
	fileInfoRepo := repository.NewFileInfoRepo(db)
	fileCategoryRepo := repository.NewFileCategoryRepo(db)
	ologRepo := repository.NewOperationLogRepo(db)
	faultInfoRepo := repository.NewFaultInfoRepo(db)
	// OpLogWriter 构造 + worker 启动（异步落库；声明制挂载的 sink 载体）。
	// sink 注入 repo.InsertBatch——批量写收口数据访问层；chan 满丢弃不阻塞业务。
	ologW := middleware.NewOpLogWriterWithSink(ologRepo.InsertBatch, logger)
	ologW.Start()
	// 名称同步事件装配：基建先于消费者（同 ologW
	// 先做后用的次序）——dispatcher 构造 + 全部 6 个同步 handler 注册；5 个名称域 service 经
	// Publisher 接口注入（Dispatcher 实现该接口），发布时序契约见 internal/event。
	dispatcher := event.NewDispatcher()
	event.RegisterAll(dispatcher, db)
	userSvc := service.NewUserService(db, userRepo, relationRepo, roleRepo, deptRepo, sessionMgr, enf, dispatcher)
	authSvc := service.NewAuthService(userRepo, relationRepo, apiKeyRepo, sessionMgr)
	menuSvc := service.NewMenuService(menuRepo, dispatcher)
	deptSvc := service.NewDeptService(deptRepo, dispatcher)
	roleSvc := service.NewRoleService(db, roleRepo, menuRepo, relationRepo, deptRepo, enf, dispatcher)
	apiKeySvc := service.NewApiKeyService(userRepo, apiKeyRepo, sessionMgr)
	dictSvc := service.NewDictService(dictTypeRepo, dictDataRepo, dispatcher)
	configSvc := service.NewConfigService(configRepo)
	shiftSvc := service.NewShiftService(shiftRepo)
	fileSvc := service.NewFileService(db, fileInfoRepo, fileCategoryRepo, fileStorage)
	fileCategorySvc := service.NewFileCategoryService(fileCategoryRepo)
	ologSvc := service.NewOperationLogService(ologRepo)
	faultSvc := service.NewFaultService(db, faultInfoRepo)
	userHandler := handler.NewUserHandler(userSvc)
	authHandler := handler.NewAuthHandler(authSvc)
	menuHandler := handler.NewMenuHandler(menuSvc)
	deptHandler := handler.NewDeptHandler(deptSvc)
	roleHandler := handler.NewRoleHandler(roleSvc)
	apiKeyHandler := handler.NewApiKeyHandler(apiKeySvc)
	dictTypeHandler := handler.NewDictTypeHandler(dictSvc)
	dictDataHandler := handler.NewDictDataHandler(dictSvc)
	configHandler := handler.NewConfigHandler(configSvc)
	shiftHandler := handler.NewShiftHandler(shiftSvc)
	fileHandler := handler.NewFileHandler(fileSvc)
	fileCategoryHandler := handler.NewFileCategoryHandler(fileCategorySvc)
	ologHandler := handler.NewOlogHandler(ologSvc)
	faultHandler := handler.NewFaultHandler(faultSvc)

	engine := newRouter(h, handlers{
		Auth:           authHandler,
		Users:          userHandler,
		Menus:          menuHandler,
		Depts:          deptHandler,
		Roles:          roleHandler,
		ApiKeys:        apiKeyHandler,
		DictTypes:      dictTypeHandler,
		DictDatas:      dictDataHandler,
		Configs:        configHandler,
		Shifts:         shiftHandler,
		Files:          fileHandler,
		FileCategories: fileCategoryHandler,
		Ologs:          ologHandler,
		Faults:         faultHandler,
	}, middleware.Auth(sessionMgr), enf, logger, ologW)

	return &App{
		cfg:    cfg,
		logger: logger,
		engine: engine,
		db:     db,
		rdb:    rdb,
		health: h,
		ologW:  ologW,
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
			Handler: engine,
			// 传输层超时：三个超时各封一段——
			//   - ReadHeaderTimeout=10s 首道闸：封 slowloris 慢头（工控机 fd 有限，
			//     头迟迟不完整即断）；
			//   - ReadTimeout=30s 封整个请求（头+体）的读取总时长——体积上限
			//     （BodyLimit 1MB）只封内存不封时间，若无此界，头在 10s 内合法发完的
			//     客户端可按每秒 1 字节滴漏 body，连接与 goroutine 被无限期占用
			//     （slowloris 变体）。30s 对工控机局域网极宽裕：1MB JSON 在百兆内网
			//     <1s 传完，正常请求远触不到上界；ReadTimeout 只约束读取，不影响
			//     响应写出；
			//   - IdleTimeout=120s 回收 keep-alive 空闲连接。
			// 刻意不设 WriteTimeout：响应无大 payload，保留长响应兼容性。
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
	}, nil
}

// Run 阻塞监听服务；http.ErrServerClosed 视为正常返回 nil。
func (a *App) Run() error {
	a.logger.Info("server listening", "port", a.cfg.Server.Port)
	if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop 优雅关停：先摘流（readyz 立即 503）→ 15s 排空在途请求 → drain 操作日志
// chan 残留 → 关 Redis/MySQL 连接。
func (a *App) Stop() {
	a.health.SetNotReady()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := a.srv.Shutdown(ctx); err != nil {
		a.logger.Error("http shutdown", "error", err.Error())
	}
	// Oplog flush（次序契约）：必须在 srv.Shutdown 之后（流量已停，无并发
	// Enqueue，drain 语义严格成立）、sqlDB.Close 之前（chan 残留日志才能落库）——
	// 先排空在途 HTTP 再 drain 日志，最后才关 DB。
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer flushCancel()
	if err := a.ologW.Flush(flushCtx); err != nil {
		a.logger.Warn("oplog flush on shutdown", "error", err.Error())
	}
	a.ologW.Stop()
	if sqlDB, err := a.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if err := a.rdb.Close(); err != nil {
		a.logger.Error("redis close", "error", err.Error())
	}
	a.logger.Info("shutdown complete")
}

// Engine 暴露 gin engine（供集成测试 httptest 复用真实装配，不经网络监听）。
func (a *App) Engine() *gin.Engine {
	return a.engine
}

// FlushOpLogs 同步排空操作日志 chan 并等待落库完成（flush 语义）。生产路径
// 由 Stop 在 srv.Shutdown 之后调用；导出仅供集成测试做同步断言（免 sleep 轮询）。
func (a *App) FlushOpLogs(ctx context.Context) error {
	return a.ologW.Flush(ctx)
}
