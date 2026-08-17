package middleware

import (
	"net/http"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/logger"
)

// Permission 权限校验中间件（动作级）。
// 校验当前登录账号是否拥有指定 action 的权限。
//
// 超级管理员（model.Account.IsAdmin=true，内置 admin）拥有全部权限，跳过校验。
//
// 用法：
//
//	perm := middleware.Permission(permLogic, "iam:grant")
//	authed.AddRoutes([]httpx.Route{{...}}, httpx.WithMiddleware(perm))
func Permission(permLogic *logic.PermissionLogic, action string) httpx.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			account := AccountFromContext(r.Context())
			if account == nil {
				httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeUnauthorized, "未登录")
				return
			}

			// 超级管理员放行（基于模型 IsAdmin 标志，而非账号名）
			if account.IsAdmin {
				if !account.Status {
					httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeForbidden, "账号已被禁用")
					return
				}
				next(w, r)
				return
			}

			// 账号禁用检查
			if !account.Status {
				httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeForbidden, "账号已被禁用")
				return
			}

			// 控制台访问资格：未授权进入管理控制台的账号（如注册账号）即使被授予策略，
			// 也不允许调用管理接口。前端隐藏菜单仅是体验层，服务端必须强制校验。
			if !account.AllowConsole {
				httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeForbidden, "无管理控制台访问权限")
				return
			}

			// 权限集合：优先复用上下文已加载的（同一请求内多个权限校验点不重复加载），
			// 未命中才从逻辑层加载（内部有 Redis 缓存兜底，无 Redis 时直接查库）。
			ps := PermissionSetFromContext(r.Context())
			if ps == nil {
				var err error
				ps, err = permLogic.LoadPermissionSet(r.Context(), account.ID)
				if err != nil {
					logger.ErrorCtx(r.Context(), "permission load failed",
						logger.Err(err), logger.Int64("account_id", account.ID))
					httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeInternalError, "权限加载失败")
					return
				}
			}

			if !ps.Check(action) {
				logger.WarnCtx(r.Context(), "permission denied",
					logger.Int64("account_id", account.ID),
					logger.String("action", action),
					logger.String("method", r.Method),
					logger.String("path", r.URL.Path))
				httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeForbidden, "无权限访问")
				return
			}

			// 将权限集合注入上下文，供同一请求链路内后续中间件/处理器复用
			next(w, r.WithContext(ContextWithPermissionSet(r.Context(), ps)))
		}
	}
}
