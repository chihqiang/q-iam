package middleware

import (
	"net/http"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/jwt"
	"github.com/chihqiang/infra-go/logger"
)

// LoadAccount 加载当前账号中间件。
// 从 JWT claims 中提取账号 ID，查询账号并注入上下文，供后续处理器使用。
// 未登录或 claims 中无账号 ID 时直接放行（由业务层自行判断是否需要登录）。
func LoadAccount(authSvc *logic.AuthLogic) httpx.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims := jwt.ClaimsFromContext(r.Context())
			if claims == nil {
				next(w, r)
				return
			}

			// 安全：应用令牌（client_credentials）的主体是应用而非账号。
			// 历史上其 user_id 声明曾写入 App 表主键，与账号表自增 ID 空间重叠——
			// 若某应用的 ID 恰好命中某账号（如内置 admin=1），会被误当成该账号加载，
			// 从而绕过权限校验（甚至命中 admin 同名放行）。
			// 应用令牌只允许访问 authOnly 组（UserInfo/DataPermissions），
			// 一律不得进入需要加载账号的 authed 组。
			if subjectType, _ := claims[logic.ClaimTokenSubjectType].(string); subjectType == logic.TokenSubjectTypeApp {
				httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeForbidden, "应用令牌无此访问权限")
				return
			}

			id, ok := claims[jwt.ClaimKeyUserID].(float64)
			if !ok || id == 0 {
				next(w, r)
				return
			}

			account, err := authSvc.GetAccountByID(r.Context(), int64(id))
			if err != nil {
				logger.ErrorCtx(r.Context(), "load account failed",
					logger.Err(err),
					logger.Int64("account_id", int64(id)))
				httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeUnauthorized, "账号不存在或已被删除")
				return
			}

			// 账号禁用：拒绝访问所有需登录态的接口（业务路由 / 个人中心 / OAuth 授权等）
			if !account.Status {
				logger.WarnCtx(r.Context(), "load account blocked: account disabled",
					logger.Int64("account_id", account.ID),
					logger.String("path", r.URL.Path))
				httpx.WriteHTTPErrorCtx(r.Context(), w, httpx.CodeForbidden, "账号已被禁用")
				return
			}

			ctx := ContextWithAccount(r.Context(), account)
			next(w, r.WithContext(ctx))
		}
	}
}
