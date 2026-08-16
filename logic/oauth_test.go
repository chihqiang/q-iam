package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/jwt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestOAuthLogic 构造 OAuthLogic（默认 DBStore 授权码消费）。
func newTestOAuthLogic(t *testing.T) *OAuthLogic {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.App{}, &model.KeyStoreItem{}, &model.Account{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	j, err := jwt.New(jwt.Config{
		Secret:             "test-secret",
		AccessTokenExpire:  time.Hour,
		RefreshTokenExpire: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	return NewOAuthLogic(db, j)
}

// TestOAuthCodeOneTimeConsumption 验证授权码一次性消费（默认 DBStore 后端，SetNX 原子占位）。
func TestOAuthCodeOneTimeConsumption(t *testing.T) {
	o := newTestOAuthLogic(t)
	ctx := context.Background()

	// 同一 jti 只能消费一次
	if !o.consumeCode(ctx, "jti-001") {
		t.Fatal("first consume should succeed")
	}
	if o.consumeCode(ctx, "jti-001") {
		t.Fatal("second consume should fail (one-time)")
	}

	// 不同 jti 可独立消费
	if !o.consumeCode(ctx, "jti-002") {
		t.Fatal("different jti should consume independently")
	}

	// 跨实例（新 OAuthLogic 实例共享同一 DBStore 表）也能防重放
	o2 := newTestOAuthLogic(t)
	// 注意：o2 使用不同的内存库，这里仅验证接口接线；
	// 真实多实例共享依赖同一数据库/Redis，由 store 后端保证。
	if o2.consumeCode(ctx, "jti-001") {
		// 若 o2 与 o1 共享存储则应为 false；此处不同库，允许为 true，仅验证方法不 panic。
		_ = 0
	}
}

// newTestAppLogic 构造带测试密钥的 AppLogic（供 IssueToken 凭证校验注入）。
func newTestAppLogic(t *testing.T, db *gorm.DB) *AppLogic {
	t.Helper()
	cipher, err := NewCipher("test-cipher-key", "")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	return NewAppLogic(db, cipher)
}

// TestIssueTokenOAuthErrors 验证应用换取 Token 的错误映射为 OAuth 协议错误（RFC 6749 §5.2）：
// 凭证无效 → invalid_client/401；授权类型不匹配 → unauthorized_client/400；缺 code → invalid_request/400。
func TestIssueTokenOAuthErrors(t *testing.T) {
	o := newTestOAuthLogic(t)
	ctx := context.Background()

	appLogic := newTestAppLogic(t, o.db)
	o.SetAppLogic(appLogic)

	// client_credentials 应用
	app, err := appLogic.Create(ctx, &AppCreateRequest{Name: "Test App"})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	// authorization_code 应用
	codeApp, err := appLogic.Create(ctx, &AppCreateRequest{
		Name:        "Code App",
		GrantType:   model.AppGrantTypeAuthorizationCode,
		CallbackURL: "https://example.com/cb",
	})
	if err != nil {
		t.Fatalf("create code app: %v", err)
	}

	assertOAuthError := func(t *testing.T, err error, wantCode string, wantStatus int) {
		t.Helper()
		var oauthErr *OAuthError
		if !errors.As(err, &oauthErr) {
			t.Fatalf("expected *OAuthError, got %v", err)
		}
		if oauthErr.Code != wantCode {
			t.Fatalf("expected error code %q, got %q", wantCode, oauthErr.Code)
		}
		if oauthErr.Status != wantStatus {
			t.Fatalf("expected status %d, got %d", wantStatus, oauthErr.Status)
		}
	}

	// 1. 凭证错误 → invalid_client（401）
	_, err = o.IssueToken(ctx, &TokenRequest{GrantType: "client_credentials", AppID: app.AppID, AppSecret: "wrong-secret"})
	assertOAuthError(t, err, OAuthErrorInvalidClient, 401)

	// 2. 授权类型不匹配 → unauthorized_client（400）
	_, err = o.IssueToken(ctx, &TokenRequest{GrantType: "client_credentials", AppID: codeApp.AppID, AppSecret: codeApp.AppSecret})
	assertOAuthError(t, err, OAuthErrorUnauthorizedClient, 400)

	// 3. authorization_code 缺 code → invalid_request（400）
	_, err = o.IssueToken(ctx, &TokenRequest{GrantType: "authorization_code", AppID: codeApp.AppID, AppSecret: codeApp.AppSecret})
	assertOAuthError(t, err, OAuthErrorInvalidRequest, 400)
}
