package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/route"
	"chihqiang/q-iam/svc"

	"github.com/chihqiang/ten-go/core/conf"
	"github.com/chihqiang/ten-go/core/logger"
	"github.com/chihqiang/ten-go/rest"
)

func main() {
	var cfg config.Config
	// UseEnv：加载时展开配置文件中的环境变量引用（如 ${JWT_SECRET}），
	// 可用于覆盖敏感配置（JWT Secret 等），避免硬编码。
	conf.MustLoad("config.yaml", &cfg, conf.UseEnv())
	log := logger.New(cfg.Logger)
	defer log.Sync()
	logger.SetGlobal(log)

	// 装配服务上下文：数据库/Redis/加密/JWT/各业务 Logic 与 Handler 的
	// 创建、注入与生命周期管理统一收敛到 svc.ServiceContext。
	ctx, err := svc.NewServiceContext(cfg)
	if err != nil {
		log.Fatalf("服务初始化失败: %v", err)
	}
	// 优雅关闭：停止审计落库 worker 并排空队列，避免丢失已入队日志；关闭 Redis 连接。
	defer ctx.Close()

	// ten-go rest 的恢复/请求 ID/访问日志默认值只在零值 Config 时启用；
	// 这里由配置文件构造 Config，显式打开这几项内置中间件。
	defaults := rest.DefaultConfig()
	cfg.Middlewares.Recovery = defaults.Middlewares.Recovery
	cfg.Middlewares.RequestID = defaults.Middlewares.RequestID
	logCfg := defaults.Middlewares.Log
	logCfg.ServiceName = cfg.Name
	cfg.Middlewares.Log = logCfg

	allowOrigins := cfg.CORS.AllowOrigins
	if len(allowOrigins) == 0 {
		allowOrigins = []string{"*"}
	}
	server, err := rest.NewServer(cfg.Config, rest.WithLogger(log), rest.WithCORS(allowOrigins...))
	if err != nil {
		log.Fatalf("创建 HTTP 服务失败: %v", err)
	}
	route.Register(server, ctx)
	_ = server.PrintRoutes(os.Stdout)

	// ten-go rest.Server 不内置信号处理，这里自行监听并优雅关闭。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		shutdownTimeout := cfg.ShutdownTimeout
		if shutdownTimeout <= 0 {
			shutdownTimeout = 10 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Errorf("服务优雅关闭失败: %v", err)
		}
	}()

	logger.Infof("服务启动 %s:%d", cfg.Host, cfg.Port)
	if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("服务启动失败: %v", err)
	}
}
