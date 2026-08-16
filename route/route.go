package route

import (
	"net/http"

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
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/login", Handler: ctx.AuthHandler.Login},
		httpx.WithMiddlewares(auditW("auth", "login")...))
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/register", Handler: ctx.AuthHandler.Register},
		httpx.WithMiddlewares(auditW("auth", "register")...))
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/refresh", Handler: ctx.AuthHandler.Refresh},
		httpx.WithMiddlewares(auditW("auth", "refresh")...))
	// 退出登录：吊销当前会话刷新令牌（公开接口，access token 过期也能退出）
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/logout", Handler: ctx.AuthHandler.Logout},
		httpx.WithMiddlewares(auditW("auth", "logout")...))
	v1.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/auth/token", Handler: ctx.AuthHandler.Token},
		httpx.WithMiddlewares(auditW("auth", "token")...))
	v1.AddRoute(httpx.Route{Method: http.MethodGet, Path: "/oauth/app-info", Handler: ctx.OAuthHandler.AppInfo})

	// 需要认证的路由组
	authMw := middleware.Auth(ctx.JWT)
	loadAccountMw := middleware.LoadAccount(ctx.AuthLogic)
	authed := v1.Group("", authMw, loadAccountMw)
	authed.AddRoute(httpx.Route{Method: http.MethodGet, Path: "/auth/me", Handler: ctx.AuthHandler.Me})
	// 个人中心：当前登录账号修改自己的密码（从登录态取用户，无需权限校验）
	// 审计：声明式挂载，与账号管理写操作一致（无需在业务 handler 里手动记录）。
	authed.AddRoute(httpx.Route{Method: http.MethodPut, Path: "/auth/password", Handler: ctx.AuthHandler.ChangePassword},
		httpx.WithMiddlewares(auditW("account", "change_password")...))
	// OAuth 授权确认（需登录态，无需权限校验）
	// 审计：声明式挂载，与账号/策略等写操作一致，操作人从登录态提取。
	authed.AddRoute(httpx.Route{Method: http.MethodPost, Path: "/oauth/authorize", Handler: ctx.OAuthHandler.Authorize},
		httpx.WithMiddlewares(auditW("oauth", "authorize")...))

	// 仅需 Bearer 认证（不加载账号）：OAuth UserInfo Endpoint 与数据权限接口
	// 应用 token（client_credentials）的 user_id 是应用 ID，不能走 LoadAccount。
	authOnly := v1.Group("", authMw)
	authOnly.AddRoute(httpx.Route{Method: http.MethodGet, Path: "/oauth/userinfo", Handler: ctx.OAuthHandler.UserInfo})
	// 数据权限：返回当前主体（账号/应用）的权限规则 + 数据范围（data_scopes），
	// 供子系统按需拉取；内部按令牌主体类型解析，不走 LoadAccount。
	authOnly.AddRoute(httpx.Route{Method: http.MethodGet, Path: "/auth/data-permissions", Handler: ctx.AuthHandler.DataPermissions})

	// 权限中间件：admin 账号（内置管理员，见 model.AdminAccountName）拥有全部权限。
	// perm(action) —— 动作级校验（校验当前账号是否拥有指定 action 权限）
	adminName := model.AdminAccountName
	perm := func(action string) httpx.Middleware {
		return middleware.Permission(ctx.PermissionLogic, action, adminName)
	}

	// 账号管理（身份管理）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/accounts", Handler: ctx.AccountHandler.List},
		{Method: http.MethodGet, Path: "/accounts/all", Handler: ctx.AccountHandler.AllList},
		{Method: http.MethodGet, Path: "/accounts/{id}", Handler: ctx.AccountHandler.Detail},
	}, httpx.WithMiddleware(perm("iam:account:read")))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/accounts", Handler: ctx.AccountHandler.Create},
		{Method: http.MethodPut, Path: "/accounts/{id}", Handler: ctx.AccountHandler.Update},
		{Method: http.MethodDelete, Path: "/accounts/{id}", Handler: ctx.AccountHandler.Delete},
	}, httpx.WithMiddlewares(auditW("account", "", perm("iam:account:write"))...))
	// 修改密码 / 重置密码（特殊动作，显式声明审计动作）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPut, Path: "/accounts/{id}/password", Handler: ctx.AccountHandler.ChangePassword},
	}, httpx.WithMiddlewares(auditW("account", "change_password", perm("iam:account:write"))...))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPut, Path: "/accounts/{id}/reset-password", Handler: ctx.AccountHandler.ResetPassword},
	}, httpx.WithMiddlewares(auditW("account", "reset_password", perm("iam:account:write"))...))

	// 账号组管理（身份管理）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/groups", Handler: ctx.GroupHandler.List},
		{Method: http.MethodGet, Path: "/groups/all", Handler: ctx.GroupHandler.AllList},
		{Method: http.MethodGet, Path: "/groups/{id}", Handler: ctx.GroupHandler.Detail},
	}, httpx.WithMiddleware(perm("iam:group:read")))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/groups", Handler: ctx.GroupHandler.Create},
		{Method: http.MethodPut, Path: "/groups/{id}", Handler: ctx.GroupHandler.Update},
		{Method: http.MethodDelete, Path: "/groups/{id}", Handler: ctx.GroupHandler.Delete},
	}, httpx.WithMiddlewares(auditW("group", "", perm("iam:group:write"))...))
	// 组成员管理（特殊动作，显式声明审计动作）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/groups/{id}/members", Handler: ctx.GroupHandler.AddMembers},
	}, httpx.WithMiddlewares(auditW("group", "add_member", perm("iam:group:write"))...))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodDelete, Path: "/groups/{id}/members", Handler: ctx.GroupHandler.RemoveMembers},
	}, httpx.WithMiddlewares(auditW("group", "remove_member", perm("iam:group:write"))...))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPut, Path: "/groups/{id}/members", Handler: ctx.GroupHandler.ReplaceMembers},
	}, httpx.WithMiddlewares(auditW("group", "replace_member", perm("iam:group:write"))...))

	// 权限策略管理（权限管理）
	// 角色与策略的多对多关系统一通过 /grants 授权管理（无独立信任策略体系）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/policies", Handler: ctx.PolicyHandler.List},
		{Method: http.MethodGet, Path: "/policies/all", Handler: ctx.PolicyHandler.AllList},
		{Method: http.MethodGet, Path: "/policies/{id}", Handler: ctx.PolicyHandler.Detail},
	}, httpx.WithMiddleware(perm("iam:policy:read")))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/policies", Handler: ctx.PolicyHandler.Create},
		{Method: http.MethodPut, Path: "/policies/{id}", Handler: ctx.PolicyHandler.Update},
		{Method: http.MethodDelete, Path: "/policies/{id}", Handler: ctx.PolicyHandler.Delete},
	}, httpx.WithMiddlewares(auditW("policy", "", perm("iam:policy:write"))...))

	// 授权管理（权限管理）：写操作记录审计，读操作（GET）由审计中间件自动跳过
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/grants", Handler: ctx.GrantHandler.Grant},
		{Method: http.MethodDelete, Path: "/grants", Handler: ctx.GrantHandler.Revoke},
		{Method: http.MethodGet, Path: "/grants/principals/{type}/{id}", Handler: ctx.GrantHandler.ListByPrincipal},
		{Method: http.MethodGet, Path: "/grants/policies/{id}", Handler: ctx.GrantHandler.ListPrincipals},
	}, httpx.WithMiddlewares(auditW("grant", "", perm("iam:grant"))...))

	// 应用管理（集成管理）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/apps", Handler: ctx.AppHandler.List},
		{Method: http.MethodGet, Path: "/apps/all", Handler: ctx.AppHandler.AllList},
		{Method: http.MethodGet, Path: "/apps/{id}", Handler: ctx.AppHandler.Detail},
	}, httpx.WithMiddleware(perm("iam:app:read")))
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/apps", Handler: ctx.AppHandler.Create},
		{Method: http.MethodPut, Path: "/apps/{id}", Handler: ctx.AppHandler.Update},
		{Method: http.MethodDelete, Path: "/apps/{id}", Handler: ctx.AppHandler.Delete},
	}, httpx.WithMiddlewares(auditW("app", "", perm("iam:app:write"))...))
	// 重置密钥（特殊动作，显式声明审计动作）
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/apps/{id}/reset-secret", Handler: ctx.AppHandler.ResetSecret},
	}, httpx.WithMiddlewares(auditW("app", "reset_secret", perm("iam:app:write"))...))

	// 操作审计（安全审计）：仅 admin 可查看
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodGet, Path: "/audit-logs", Handler: ctx.AuditHandler.List},
		{Method: http.MethodGet, Path: "/audit-logs/modules", Handler: ctx.AuditHandler.Modules},
	}, httpx.WithMiddleware(perm("iam:audit:read")))

	// 历史数据清理（系统管理）：清理 days 天以前的数据（默认 30 天），
	// 清理操作本身记审计（module=system, action=cleanup），防误操作可追溯。
	authed.AddRoutes([]httpx.Route{
		{Method: http.MethodPost, Path: "/cleanup/history", Handler: ctx.CleanupHandler.History},
	}, httpx.WithMiddlewares(auditW("system", "cleanup", perm("iam:system:cleanup"))...))

	// 纯 API 服务：不再托管前端控制台（web 已移出独立部署）。
	// 所有未匹配路由统一返回 JSON 404。
	server.SetNotFoundHandler(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeNotFound, "接口不存在")
	})
}
