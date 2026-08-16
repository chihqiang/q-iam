package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"
	"chihqiang/q-iam/middleware"

	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/jwt"
)

// OAuthHandler OAuth 授权 HTTP 处理器。
type OAuthHandler struct {
	svc *logic.OAuthLogic
}

// NewOAuthHandler 创建 OAuth 授权处理器。
// OAuth 授权的审计由路由声明式中间件记录（auditW("oauth", "authorize")），
// handler 无需手动记录，与账号/策略等其他模块保持一致。
func NewOAuthHandler(svc *logic.OAuthLogic) *OAuthHandler {
	return &OAuthHandler{svc: svc}
}

// AppInfo 查询应用信息（公开接口，供授权页展示）。
// GET /oauth/app-info?client_id=xxx
func (h *OAuthHandler) AppInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "缺少 client_id")
		return
	}

	app, err := h.svc.AppInfo(ctx, clientID)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, app)
}

// Authorize 授权确认并签发授权码（需登录态）。
// POST /oauth/authorize
//
//	{
//	  "client_id": "app-xxx",
//	  "redirect_uri": "https://example.com/callback",
//	  "scope": "iam:read",
//	  "state": "xyz"
//	}
//
// 成功返回授权码 code，由前端跳转 redirect_uri?code=xxx&state=xxx。
type authorizeRequest struct {
	ClientID    string `json:"client_id" binding:"required"`
	RedirectURI string `json:"redirect_uri" binding:"required"`
	Scope       string `json:"scope"`
	State       string `json:"state"`
}

func (h *OAuthHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req authorizeRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	// 当前登录账号（由 LoadAccount 中间件注入）
	account := middleware.AccountFromContext(ctx)
	if account == nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeUnauthorized, "未登录")
		return
	}

	app, code, err := h.svc.Authorize(ctx, account, req.ClientID, req.RedirectURI, req.Scope)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}

	httpx.OkJSONCtx(ctx, w, map[string]any{
		"code":         code,
		"app_id":       app.AppID,
		"app_name":     app.Name,
		"redirect_uri": req.RedirectURI,
		"scope":        req.Scope,
		"state":        req.State,
	})
}

// UserInfo 用户信息（OAuth 2.0 UserInfo Endpoint）。
// GET /oauth/userinfo，携带应用换取的 access_token（Bearer 认证）。
// 返回用户信息 + 已授权权限范围 + 应用信息。
func (h *OAuthHandler) UserInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims := jwt.ClaimsFromContext(ctx)
	if claims == nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeUnauthorized, "无效的访问令牌")
		return
	}

	info, err := h.svc.UserInfo(ctx, claims)
	if err != nil {
		// OAuth 协议错误（logic.OAuthError）：返回标准状态码 + {error, error_description}
		// （RFC 6750），兼容标准 OAuth 客户端；其余内部错误返回 500。
		if writeOAuthError(w, err) {
			return
		}
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeInternalError, err.Error())
		return
	}
	httpx.OkJSONCtx(ctx, w, info)
}
