package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/chihqiang/infra-go/httpx"
)

// trustProxyContextKey 客户端 IP 提取策略（是否信任反向代理）。
type trustProxyContextKey struct{}

// TrustProxy 将「是否信任反向代理」注入上下文。
// 必须在所有需要 ClientIP 的中间件/处理器之前执行（挂全局中间件链最外层）。
// 仅当 trust=true 时 ClientIP 才读取 X-Forwarded-For / X-Real-IP，
// 否则一律使用 TCP 直连地址（RemoteAddr），杜绝伪造。
func TrustProxy(trust bool) httpx.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), trustProxyContextKey{}, trust)
			next(w, r.WithContext(ctx))
		}
	}
}

// ClientIP 提取客户端 IP。
//   - 信任反代（security.trust_proxy=true）：优先 X-Forwarded-For（取首个）→ X-Real-IP → RemoteAddr；
//   - 默认（false）：直接使用 RemoteAddr（TCP 直连地址），避免 XFF 伪造。
//
// 供审计日志与刷新令牌签发来源记录复用。
func ClientIP(r *http.Request) string {
	if trust, _ := r.Context().Value(trustProxyContextKey{}).(bool); trust {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return xr
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
