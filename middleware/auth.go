package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/jwt"
)

// BearerToken 从 Authorization: Bearer <token> 头提取令牌，无则返回空串。
func BearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// RevokedChecker 访问令牌撤销黑名单检查函数：jti 命中黑名单（已吊销）返回 true。
// 由 Auth 中间件在解析验证令牌后调用，用于使已登出/被吊销会话的 access token 立即失效。
type RevokedChecker func(ctx context.Context, jti string) bool

// Auth 返回 JWT 认证中间件（带可选的访问令牌撤销检查）。
// 从 Authorization: Bearer <token> 头提取令牌并解析验证（签名/过期/access 类型）：
//   - 若提供 revokedChecker，校验 jti 是否在撤销黑名单，命中返回 401；
//   - 将业务声明（排除标准声明与 token_type）注入 context，供下游 ClaimsFromContext 读取。
//
// 注意：标准声明（sub/exp/jti 等）与 token_type 会被过滤，下游判断令牌主体类型
// 须使用业务声明（token_subject_type / app_id，见 logic/token.go），不能依赖 sub。
func Auth(j *jwt.JWT, checkRevoked RevokedChecker) httpx.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token := BearerToken(r)
			if token == "" {
				http.Error(w, "token is missing", http.StatusUnauthorized)
				return
			}

			claims, err := j.ParseAccessToken(token)
			if err != nil {
				msg := "invalid token"
				if errors.Is(err, jwt.ErrExpiredToken) {
					msg = "token expired"
				}
				http.Error(w, msg, http.StatusUnauthorized)
				return
			}

			// 撤销黑名单校验：登出/被吊销会话的 access token 立即失效
			if checkRevoked != nil {
				if jti, _ := claims[jwt.ClaimKeyJWTID].(string); jti != "" && checkRevoked(r.Context(), jti) {
					http.Error(w, "token revoked", http.StatusUnauthorized)
					return
				}
			}

			// 注入业务声明（与 infra-go AuthMiddleware 语义一致：仅业务字段进入 context）
			next(w, r.WithContext(jwt.WithClaims(r.Context(), extractBusinessClaims(claims))))
		}
	}
}

// extractBusinessClaims 过滤标准声明与 token_type，仅保留业务字段。
// 与 infra-go jwt 包内同名函数语义保持一致（该函数未导出，此处复刻）。
func extractBusinessClaims(claims jwt.Claims) jwt.Claims {
	out := make(jwt.Claims, len(claims))
	for k, v := range claims {
		switch k {
		case jwt.ClaimKeyIssuer, jwt.ClaimKeyAudience, jwt.ClaimKeySubject,
			jwt.ClaimKeyExpirationTime, jwt.ClaimKeyIssuedAt, jwt.ClaimKeyNotBefore,
			jwt.ClaimKeyTokenType, jwt.ClaimKeyJWTID:
			continue
		default:
			out[k] = v
		}
	}
	return out
}
