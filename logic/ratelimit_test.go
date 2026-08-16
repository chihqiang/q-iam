package logic

import (
	"context"
	"testing"

	"github.com/chihqiang/infra-go/ratelimit"
)

// rejectLimiter 一个始终拒绝的 Limiter，用于验证限流器注入接线。
type rejectLimiter struct{}

func (rejectLimiter) Allow() bool { return false }
func (rejectLimiter) AllowContext(ctx context.Context) (bool, error) {
	return false, nil
}

// 编译期断言：内存/Redis 限流器都实现 ratelimit.Limiter 接口，
// 业务侧可自由注入替换（默认内存，配置 Redis 后注入 RedisTokenBucket）。
var (
	_ ratelimit.Limiter = (*ratelimit.TokenBucket)(nil)
	_ ratelimit.Limiter = (*ratelimit.RedisTokenBucket)(nil)
)

// TestAuthLogicLimiterInjection 验证登录限流器可通过接口注入替换（如 Redis 分布式限流）。
func TestAuthLogicLimiterInjection(t *testing.T) {
	svc, _, acct := newTestAuthLogic(t)
	ctx := context.Background()

	// 注入"始终拒绝"的限流器：登录应立即被全局限流拦截
	svc.SetLoginLimiter(rejectLimiter{})
	_, err := svc.Login(ctx, &LoginRequest{AccountName: acct.AccountName, Password: "Password123"}, "127.0.0.1", "go-test")
	if err == nil || err.Error() != "登录请求过于频繁，请稍后再试" {
		t.Fatalf("expected global rate limit, got %v", err)
	}
}

// TestRegisterLimiterInjection 验证注册限流器可通过接口注入替换。
func TestRegisterLimiterInjection(t *testing.T) {
	svc, _, _ := newTestAuthLogic(t)
	ctx := context.Background()

	svc.SetRegisterLimiter(rejectLimiter{})
	_, err := svc.Register(ctx, &RegisterRequest{AccountName: "newuser", Password: "Password123"}, "127.0.0.1", "go-test")
	if err == nil || err.Error() != "注册请求过于频繁，请稍后再试" {
		t.Fatalf("expected register rate limit, got %v", err)
	}
}

// TestOAuthTokenLimiterInjection 验证换 Token 限流器可通过接口注入替换。
func TestOAuthTokenLimiterInjection(t *testing.T) {
	o := newTestOAuthLogic(t)
	ctx := context.Background()

	o.SetTokenLimiter(rejectLimiter{})
	_, err := o.IssueToken(ctx, &TokenRequest{
		GrantType: "client_credentials",
		AppID:     "app-x",
		AppSecret: "secret",
	})
	if err == nil || err.Error() != "请求过于频繁，请稍后再试" {
		t.Fatalf("expected token rate limit, got %v", err)
	}
}
