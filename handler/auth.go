package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"
	"chihqiang/q-iam/middleware"

	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/jwt"
)

// AuthHandler 认证相关 HTTP 处理器。
type AuthHandler struct {
	svc       *logic.AuthLogic
	permLogic *logic.PermissionLogic // /auth/me 返回权限供前端菜单过滤
}

// NewAuthHandler 创建认证处理器。
// 公开接口（登录/注册/刷新/换 Token）与个人中心改密码的审计
// 已全部由路由声明式中间件记录，handler 无需持有审计逻辑。
func NewAuthHandler(svc *logic.AuthLogic) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// SetPermissionLogic 注入权限逻辑（/auth/me 附加权限列表）。
func (h *AuthHandler) SetPermissionLogic(permLogic *logic.PermissionLogic) {
	h.permLogic = permLogic
}

// Login 登录。
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.LoginRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	resp, err := h.svc.Login(ctx, &req, middleware.ClientIP(r), r.UserAgent())
	// 登录审计由路由中间件声明式记录（route.go: auditW("auth", "login")），操作人从请求体提取。
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}

	httpx.OkJSONCtx(ctx, w, resp)
}

// Token 应用凭证换取访问令牌。
func (h *AuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.TokenRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	resp, err := h.svc.Token(ctx, &req)
	// 换 Token 审计由路由中间件声明式记录（route.go: auditW("auth", "token")），操作人从请求体提取。
	if err != nil {
		// OAuth 协议错误（logic.OAuthError）：返回标准状态码 + {error, error_description}
		// （RFC 6749 §5.2），兼容标准 OAuth 客户端；其余内部错误返回 500。
		if writeOAuthError(w, err) {
			return
		}
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeInternalError, err.Error())
		return
	}

	httpx.OkJSONCtx(ctx, w, resp)
}

// Refresh 刷新令牌。
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.RefreshRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	resp, err := h.svc.Refresh(ctx, &req, middleware.ClientIP(r), r.UserAgent())
	// 刷新令牌审计由路由中间件声明式记录（route.go: auditW("auth", "refresh")）。
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}

	httpx.OkJSONCtx(ctx, w, resp)
}

// Logout 主动退出（吊销当前会话的刷新令牌 + 访问令牌）。
// 公开接口：只需携带 refresh_token，access token 已过期时也能正常退出。
// 同时从 Authorization header 提取 access token 加入撤销黑名单，
// 使已登出会话的访问令牌立即失效（不再依赖自然过期）。
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.RefreshRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	// 吊销当前会话的 access token（幂等：无效/过期/无 jti 静默忽略）
	if tok := middleware.BearerToken(r); tok != "" {
		h.svc.RevokeAccessToken(ctx, tok)
	}

	if err := h.svc.Logout(ctx, req.RefreshToken); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}

	httpx.OkJSONCtx(ctx, w, map[string]string{"message": "退出成功"})
}

// Register 注册账号（公开接口）。
// 注册即启用并自动登录（返回 token）；受 security.register.enabled 开关控制。
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.RegisterRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	resp, err := h.svc.Register(ctx, &req, middleware.ClientIP(r), r.UserAgent())
	// 注册审计由路由中间件声明式记录（route.go: auditW("auth", "register")），操作人从请求体提取。
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}

	httpx.OkJSONCtx(ctx, w, resp)
}

// Me 获取当前登录账号信息。
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	account := middleware.AccountFromContext(ctx)
	if account == nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeUnauthorized, "未登录")
		return
	}

	profile, err := h.svc.GetProfile(ctx, account.ID)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}

	// 附加当前账号的权限（供前端按权限过滤侧边栏菜单）
	if h.permLogic != nil {
		perms, err := h.permLogic.LoadPermissionStatements(ctx, account.ID)
		if err != nil {
			httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
			return
		}
		profile.Permissions = perms
	}

	httpx.OkJSONCtx(ctx, w, profile)
}

// DataPermissions 获取当前主体（账号/应用）的权限规则与数据范围。
// 供子系统按需拉取：解析 Bearer 令牌主体类型，返回权限 + data_scopes + 主体所属组。
func (h *AuthHandler) DataPermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims := jwt.ClaimsFromContext(ctx)
	if claims == nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeUnauthorized, "未认证")
		return
	}

	resp, err := h.svc.DataPermissions(ctx, claims)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, resp)
}

// ChangePassword 当前登录账号修改自己的密码（个人中心）。
// 从登录态取账号 ID，无需 iam:account:write 权限。
// 审计由路由中间件声明式记录（route.go: auditW("account", "change_password")），
// 与账号管理写操作保持一致，业务 handler 不再手动写审计。
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	account := middleware.AccountFromContext(ctx)
	if account == nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeUnauthorized, "未登录")
		return
	}

	var req logic.ChangeOwnPasswordRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	if err := h.svc.ChangeOwnPassword(ctx, account.ID, &req); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}

	httpx.OkJSONCtx(ctx, w, map[string]string{"message": "密码修改成功"})
}
