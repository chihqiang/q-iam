package middleware

import (
	"net/http"
	"strings"

	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/jwt"
)

// Auth 返回 JWT 认证中间件。
// 从 Authorization: Bearer <token> 头提取令牌并解析验证，将 claims 注入上下文。
func Auth(j *jwt.JWT) httpx.Middleware {
	getToken := func(r *http.Request) string {
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
	return j.AuthMiddleware(getToken)
}
