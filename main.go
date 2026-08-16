package main

import (
	"chihqiang/q-iam/config"
	"chihqiang/q-iam/route"
	"chihqiang/q-iam/svc"

	"github.com/chihqiang/infra-go/conf"
	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/logger"
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

	server := httpx.NewServer(cfg.Server)
	route.Register(server, ctx)
	server.PrintRoutes()

	logger.Infof("服务启动 %s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := server.Start(); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
