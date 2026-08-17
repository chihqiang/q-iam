package route

import (
	"net/http"

	"chihqiang/q-iam/handler"
	"chihqiang/q-iam/middleware"
	"chihqiang/q-iam/model"
	"chihqiang/q-iam/svc"
	"chihqiang/q-iam/ui"

	"github.com/chihqiang/infra-go/httpx"
)

// Register 注册所有路由与全局中间件。
//
// 路由规划（对应 RAM 三大模块，前缀 /api/v1）：
//
//	┌─ 认证 ──────────────────────────────────────────────
//	│  POST /auth/login          登录（账号/应用换取 Token）
//	│  POST /auth/refresh        刷新 Token
//	│  GET  /auth/me             当前登录主体信息
//	├─ 身份管理（Identity）──────────────────────────────
//	│  /accounts    账号 CRUD
//	│  /groups      账号组 CRUD
//	├─ 权限管理（Permission）────────────────────────────
//	│  /policies    权限策略 CRUD
//	│  /grants      授权（策略绑定到 账号/账号组/角色/应用）
//	└─ 集成管理（Integration）───────────────────────────
//	   /apps        应用 CRUD + 密钥重置
func Register(server *httpx.Server, ctx *svc.ServiceContext) {
	// HTTP 处理器层（Handler）是接口适配层，仅依赖业务逻辑层（Logic），
	// 由 route 包在注册路由处创建；ServiceContext 不混入 HTTP 层概念。
	authHandler := handler.NewAuthHandler(ctx.AuthLogic)
	// /auth/me 返回当前账号权限，供前端按权限过滤菜单
	authHandler.SetPermissionLogic(ctx.PermissionLogic)
	oauthHandler := handler.NewOAuthHandler(ctx.OAuthLogic)
	accountHandler := handler.NewAccountHandler(ctx.AccountLogic)
	groupHandler := handler.NewGroupHandler(ctx.GroupLogic)
	policyHandler := handler.NewPolicyHandler(ctx.PolicyLogic)
	grantHandler := handler.NewGrantHandler(ctx.GrantLogic)
	appHandler := handler.NewAppHandler(ctx.AppLogic)
	auditHandler := handler.NewAuditHandler(ctx.AuditLogic)
	cleanupHandler := handler.NewCleanupHandler(ctx.CleanupLogic)

	cfg := ctx.Config
	if cfg.Pprof.Enabled {
		server.AddRoutes(httpx.PprofRoutes(""))
	}

	// 健康检查
	server.AddRoute(httpx.Route{
		Method: http.MethodGet,
		Path:   "/health",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			httpx.OkJSON(w, map[string]string{"status": "ok"})
		},
	})

	// 全局中间件
	server.Use(httpx.WithRequestID())
	server.Use(httpx.WithRecovery())
	server.Use(httpx.WithLogger())

	server.AddRoute(ui.DistDirFS)

	// 客户端 IP 提取策略（是否信任反向代理）必须先于所有需要 ClientIP 的中间件/处理器
	server.Use(middleware.TrustProxy(cfg.Security.TrustProxy))

	allowOrigins := cfg.CORS.AllowOrigins
	if len(allowOrigins) == 0 {
		allowOrigins = []string{"*"}
	}
	server.Use(httpx.WithCors(allowOrigins...))

	v1 := server.Group("/api/v1")

	// auditW 组装「写操作审计」中间件链：
	// 声明审计模块 → 声明审计动作(可选) → 审计中间件 → 权限校验。
	// 顺序很关键：元数据声明必须在 auditMw 之前（外层），审计中间件才能从 context
	// 读到路由声明的模块/动作；auditMw 在权限校验之前，权限校验失败也会被记录。
	// 新增路由只需在此声明 module/action，无需再修改 logic/audit.go。
	auditMw := middleware.Audit(ctx.AuditLogic)
	auditW := func(module, action string, mws ...httpx.Middleware) []httpx.Middleware {
		chain := []httpx.Middleware{middleware.AuditModule(module)}
		if action != "" {
			chain = append(chain, middleware.AuditAction(action))
		}
		chain = append(chain, auditMw)
		chain = append(chain, mws...)
		return chain
	}

	// 认证：登录 / 注册 / 刷新 / 应用换取 Token / OAuth 应用信息查询无需认证。
	// 公开写接口的审计同样走声明式中间件（操作人从请求体提取：account_name / app_id），
	// 与业务 handler 解耦，不再手动 Record。
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/login", Handler: authHandler.Login},
		httpx.WithMiddlewares(auditW("auth", "login")...))
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/register", Handler: authHandler.Register},
		httpx.WithMiddlewares(auditW("auth", "register")...))
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/refresh", Handler: authHandler.Refresh},
		httpx.WithMiddlewares(auditW("auth", "refresh")...))
	// 退出登录：吊销当前会话刷新令牌（公开接口，access token 过期也能退出）
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/logout", Handler: authHandler.Logout},
		httpx.WithMiddlewares(auditW("auth", "logout")...))
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/token", Handler: authHandler.Token},
		httpx.WithMiddlewares(auditW("auth", "token")...))
	v1.AddRoute(httpx.Route{Method: http.MethodGet, Path: "/oauth/app-info", Handler: oauthHandler.AppInfo})

	// 需要认证的路由组
	authMw := middleware.Auth(ctx.JWT)
	loadAccountMw := middleware.LoadAccount(ctx.AuthLogic)
	authed := v1.Group("", authMw, loadAccountMw)
	authed.AddRoute(httpx.Route{Method: http.MethodGet, Path: "/auth/me", Handler: authHandler.Me})
	// 个人中心：当前登录账号修改自己的密码（从登录态取用户，无需权限校验）
	// 审计：声明式挂载，与账号管理写操作一致（无需在业务 handler 里手动记录）。
	authed.AddRoute(httpx.Route{Method: http.MethodPut, Path: "/auth/password", Handler: authHandler.ChangePassword},
		httpx.WithMiddlewares(auditW("account", "change_password")...))
	// OAuth 授权确认（需登录态，无需权限校验）
	// 审计：声明式挂载，与账号/策略等写操作一致，操作人从登录态提取。
	authed.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/oauth/authorize", Handler: oauthHandler.Authorize},
		httpx.WithMiddlewares(auditW("oauth", "authorize")...))

	// 仅需 Bearer 认证（不加载账号）：OAuth UserInfo Endpoint 与数据权限接口
	// 应用 token（client_credentials）的 user_id 是应用 ID，不能走 LoadAccount。
	authOnly := v1.Group("", authMw)
	authOnly.AddRoute(httpx.Route{Method: http.MethodGet, Path: "/oauth/userinfo", Handler: oauthHandler.UserInfo})
	// 数据权限：返回当前主体（账号/应用）的权限规则 + 数据范围（data_scopes），
	// 供子系统按需拉取；内部按令牌主体类型解析，不走 LoadAccount。
	authOnly.AddRoute(httpx.Route{Method: http.MethodGet, Path: "/auth/data-permissions", Handler: authHandler.DataPermissions})

	// 权限中间件：admin 账号（内置管理员，见 model.AdminAccountName）拥有全部权限。
	// perm(action) —— 动作级校验（校验当前账号是否拥有指定 action 权限）
	adminName := model.AdminAccountName
	perm := func(action string) httpx.Middleware {
		return middleware.Permission(ctx.PermissionLogic, action, adminName)
	}

	// 账号管理（身份管理）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/accounts", Handler: accountHandler.List},
		{Method: http.MethodGet, Path: "/accounts/all", Handler: accountHandler.AllList},
		{Method: http.MethodGet, Path: "/accounts/{id}", Handler: accountHandler.Detail},
	}, httpx.WithMiddleware(perm("iam:account:read")))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/accounts", Handler: accountHandler.Create},
		{Method: http.MethodPut, Path: "/accounts/{id}", Handler: accountHandler.Update},
		{Method: http.MethodDelete, Path: "/accounts/{id}", Handler: accountHandler.Delete},
	}, httpx.WithMiddlewares(auditW("account", "", perm("iam:account:write"))...))
	// 修改密码 / 重置密码（特殊动作，显式声明审计动作）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPut, Path: "/accounts/{id}/password", Handler: accountHandler.ChangePassword},
	}, httpx.WithMiddlewares(auditW("account", "change_password", perm("iam:account:write"))...))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPut, Path: "/accounts/{id}/reset-password", Handler: accountHandler.ResetPassword},
	}, httpx.WithMiddlewares(auditW("account", "reset_password", perm("iam:account:write"))...))

	// 账号组管理（身份管理）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/groups", Handler: groupHandler.List},
		{Method: http.MethodGet, Path: "/groups/all", Handler: groupHandler.AllList},
		{Method: http.MethodGet, Path: "/groups/{id}", Handler: groupHandler.Detail},
	}, httpx.WithMiddleware(perm("iam:group:read")))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/groups", Handler: groupHandler.Create},
		{Method: http.MethodPut, Path: "/groups/{id}", Handler: groupHandler.Update},
		{Method: http.MethodDelete, Path: "/groups/{id}", Handler: groupHandler.Delete},
	}, httpx.WithMiddlewares(auditW("group", "", perm("iam:group:write"))...))
	// 组成员管理（特殊动作，显式声明审计动作）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/groups/{id}/members", Handler: groupHandler.AddMembers},
	}, httpx.WithMiddlewares(auditW("group", "add_member", perm("iam:group:write"))...))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodDelete, Path: "/groups/{id}/members", Handler: groupHandler.RemoveMembers},
	}, httpx.WithMiddlewares(auditW("group", "remove_member", perm("iam:group:write"))...))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPut, Path: "/groups/{id}/members", Handler: groupHandler.ReplaceMembers},
	}, httpx.WithMiddlewares(auditW("group", "replace_member", perm("iam:group:write"))...))

	// 权限策略管理（权限管理）
	// 角色与策略的多对多关系统一通过 /grants 授权管理（无独立信任策略体系）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/policies", Handler: policyHandler.List},
		{Method: http.MethodGet, Path: "/policies/all", Handler: policyHandler.AllList},
		{Method: http.MethodGet, Path: "/policies/{id}", Handler: policyHandler.Detail},
	}, httpx.WithMiddleware(perm("iam:policy:read")))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/policies", Handler: policyHandler.Create},
		{Method: http.MethodPut, Path: "/policies/{id}", Handler: policyHandler.Update},
		{Method: http.MethodDelete, Path: "/policies/{id}", Handler: policyHandler.Delete},
	}, httpx.WithMiddlewares(auditW("policy", "", perm("iam:policy:write"))...))

	// 授权管理（权限管理）：写操作记录审计，读操作（GET）由审计中间件自动跳过
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/grants", Handler: grantHandler.Grant},
		{Method: http.MethodDelete, Path: "/grants", Handler: grantHandler.Revoke},
		{Method: http.MethodGet, Path: "/grants/principals/{type}/{id}", Handler: grantHandler.ListByPrincipal},
		{Method: http.MethodGet, Path: "/grants/policies/{id}", Handler: grantHandler.ListPrincipals},
	}, httpx.WithMiddlewares(auditW("grant", "", perm("iam:grant"))...))

	// 应用管理（集成管理）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/apps", Handler: appHandler.List},
		{Method: http.MethodGet, Path: "/apps/all", Handler: appHandler.AllList},
		{Method: http.MethodGet, Path: "/apps/{id}", Handler: appHandler.Detail},
	}, httpx.WithMiddleware(perm("iam:app:read")))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/apps", Handler: appHandler.Create},
		{Method: http.MethodPut, Path: "/apps/{id}", Handler: appHandler.Update},
		{Method: http.MethodDelete, Path: "/apps/{id}", Handler: appHandler.Delete},
	}, httpx.WithMiddlewares(auditW("app", "", perm("iam:app:write"))...))
	// 重置密钥（特殊动作，显式声明审计动作）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/apps/{id}/reset-secret", Handler: appHandler.ResetSecret},
	}, httpx.WithMiddlewares(auditW("app", "reset_secret", perm("iam:app:write"))...))

	// 操作审计（安全审计）：仅 admin 可查看
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/audit-logs", Handler: auditHandler.List},
		{Method: http.MethodGet, Path: "/audit-logs/modules", Handler: auditHandler.Modules},
	}, httpx.WithMiddleware(perm("iam:audit:read")))

	// 历史数据清理（系统管理）：清理 days 天以前的数据（默认 30 天），
	// 清理操作本身记审计（module=system, action=cleanup），防误操作可追溯。
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/cleanup/history", Handler: cleanupHandler.History},
	}, httpx.WithMiddlewares(auditW("system", "cleanup", perm("iam:system:cleanup"))...))

	// 纯 API 服务：不再托管前端控制台（web 已移出独立部署）。
	// 所有未匹配路由统一返回 JSON 404。
	server.SetNotFoundHandler(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeNotFound, "接口不存在")
	})
}
