package logic

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/hash"
	"github.com/chihqiang/infra-go/jwt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestAuthLogic 构造基于内存 SQLite 的 AuthLogic 测试环境。
func newTestAuthLogic(t *testing.T) (*AuthLogic, *gorm.DB, *model.Account) {
	t.Helper()

	// 每个用例独立的命名内存库，避免共享 cache=shared 导致用例间数据污染
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Account{}, &model.RefreshToken{}, &model.PasswordHistory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.Config{}
	cfg.JWT.Secret = "test-secret-key"
	cfg.JWT.AccessTokenExpire = time.Hour
	cfg.JWT.RefreshTokenExpire = 24 * time.Hour
	cfg.Security.Register.Enabled = true // 测试场景默认开放注册

	j, err := jwt.New(cfg.JWT)
	if err != nil {
		t.Fatalf("jwt new: %v", err)
	}

	ph, err := hash.BcryptHashDefault("Password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	now := time.Now()
	acct := model.Account{
		AccountName:       "tester",
		DisplayName:       "Tester",
		Password:          ph,
		Status:            true,
		AllowConsole:      true,
		PasswordChangedAt: &now,
	}
	if err := db.Create(&acct).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	return NewAuthLogic(db, j, cfg), db, &acct
}

// countActiveTokens 统计某账号有效（未吊销且未过期）的刷新令牌记录数。
func countActiveTokens(t *testing.T, db *gorm.DB, accountID int64) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.RefreshToken{}).
		Where("account_id = ? AND revoked_at IS NULL AND expires_at > ?", accountID, time.Now()).
		Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// TestRefreshTokenRotation 验证刷新令牌轮换：登录落库 → 刷新轮换 → 重放检测吊销全部。
func TestRefreshTokenRotation(t *testing.T) {
	svc, db, acct := newTestAuthLogic(t)
	ctx := context.Background()

	// 1. 登录：签发令牌对并落库一条记录
	resp, err := svc.Login(ctx, &LoginRequest{AccountName: acct.AccountName, Password: "Password123"}, "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp.RefreshToken == "" {
		t.Fatal("login response missing refresh token")
	}
	if n := countActiveTokens(t, db, acct.ID); n != 1 {
		t.Fatalf("after login, expected 1 active refresh token, got %d", n)
	}
	oldRefresh := resp.RefreshToken

	// 2. 刷新：旧记录被吊销，新记录落库，新 pair 可用
	resp2, err := svc.Refresh(ctx, &RefreshRequest{RefreshToken: oldRefresh}, "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if resp2.RefreshToken == "" {
		t.Fatal("refresh response missing refresh token")
	}
	if resp2.RefreshToken == oldRefresh {
		t.Fatal("refresh token was not rotated")
	}
	if n := countActiveTokens(t, db, acct.ID); n != 1 {
		t.Fatalf("after rotation, expected 1 active refresh token, got %d", n)
	}
	newRefresh := resp2.RefreshToken

	// 3. 重放旧令牌：判定重用 → 吊销该账号全部刷新令牌
	if _, err := svc.Refresh(ctx, &RefreshRequest{RefreshToken: oldRefresh}, "127.0.0.1", "go-test"); err == nil {
		t.Fatal("reusing rotated refresh token should fail")
	}
	if n := countActiveTokens(t, db, acct.ID); n != 0 {
		t.Fatalf("after reuse detection, expected 0 active refresh tokens, got %d", n)
	}

	// 4. 被连带吊销的新令牌也应失效
	if _, err := svc.Refresh(ctx, &RefreshRequest{RefreshToken: newRefresh}, "127.0.0.1", "go-test"); err == nil {
		t.Fatal("refresh token revoked by reuse detection should fail")
	}
}

// TestRefreshTokenRevokedOnPasswordChange 验证改密后吊销该账号全部刷新令牌。
func TestRefreshTokenRevokedOnPasswordChange(t *testing.T) {
	svc, db, acct := newTestAuthLogic(t)
	ctx := context.Background()

	resp, err := svc.Login(ctx, &LoginRequest{AccountName: acct.AccountName, Password: "Password123"}, "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if n := countActiveTokens(t, db, acct.ID); n != 1 {
		t.Fatalf("after login, expected 1 active refresh token, got %d", n)
	}

	// 修改密码（旧密码正确）
	if err := svc.ChangeOwnPassword(ctx, acct.ID, &ChangeOwnPasswordRequest{
		OldPassword: "Password123",
		NewPassword: "NewPassword456",
	}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	// 改密后全部刷新令牌应被吊销
	if n := countActiveTokens(t, db, acct.ID); n != 0 {
		t.Fatalf("after password change, expected 0 active refresh tokens, got %d", n)
	}
	if _, err := svc.Refresh(ctx, &RefreshRequest{RefreshToken: resp.RefreshToken}, "127.0.0.1", "go-test"); err == nil {
		t.Fatal("refresh token should be invalid after password change")
	}
}

// TestRefreshTokenInvalidTokenID 验证查无记录（伪造/陌生令牌）直接拒绝。
func TestRefreshTokenInvalidTokenID(t *testing.T) {
	svc, db, acct := newTestAuthLogic(t)
	ctx := context.Background()

	// 直接用一个未落库的合法签名刷新令牌（伪造 jti 场景需真实签发，这里用登录后手动吊销来模拟陌生记录）
	resp, err := svc.Login(ctx, &LoginRequest{AccountName: acct.AccountName, Password: "Password123"}, "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// 删除表记录，模拟 jti 在服务端已不存在（如库被清空）
	if err := db.Where("account_id = ?", acct.ID).Delete(&model.RefreshToken{}).Error; err != nil {
		t.Fatalf("delete tokens: %v", err)
	}
	if _, err := svc.Refresh(ctx, &RefreshRequest{RefreshToken: resp.RefreshToken}, "127.0.0.1", "go-test"); err == nil {
		t.Fatal("refresh with unknown token id should fail")
	}
}

// TestLogoutRevokesCurrentSession 验证主动退出：吊销当前会话刷新令牌，且幂等。
func TestLogoutRevokesCurrentSession(t *testing.T) {
	svc, db, acct := newTestAuthLogic(t)
	ctx := context.Background()

	// 登录两次：同一账号两个会话，各有一条刷新令牌记录
	resp1, err := svc.Login(ctx, &LoginRequest{AccountName: acct.AccountName, Password: "Password123"}, "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login 1: %v", err)
	}
	resp2, err := svc.Login(ctx, &LoginRequest{AccountName: acct.AccountName, Password: "Password123"}, "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login 2: %v", err)
	}
	if n := countActiveTokens(t, db, acct.ID); n != 2 {
		t.Fatalf("after two logins, expected 2 active refresh tokens, got %d", n)
	}

	// 退出会话 1：仅吊销其刷新令牌
	if err := svc.Logout(ctx, resp1.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if n := countActiveTokens(t, db, acct.ID); n != 1 {
		t.Fatalf("after logout, expected 1 active refresh token, got %d", n)
	}
	// 会话 1 的刷新令牌已失效，无法再刷新
	if _, err := svc.Refresh(ctx, &RefreshRequest{RefreshToken: resp1.RefreshToken}, "127.0.0.1", "go-test"); err == nil {
		t.Fatal("refresh token should be invalid after logout")
	}
	// 会话 2 的刷新令牌仍有效（未被连带吊销）
	if _, err := svc.Refresh(ctx, &RefreshRequest{RefreshToken: resp2.RefreshToken}, "127.0.0.1", "go-test"); err != nil {
		t.Fatalf("session 2 refresh should still work, got %v", err)
	}

	// 幂等：对同一（已吊销）令牌再次退出仍成功
	if err := svc.Logout(ctx, resp1.RefreshToken); err != nil {
		t.Fatalf("logout again should be idempotent, got %v", err)
	}
	// 对无效令牌退出也幂等成功
	if err := svc.Logout(ctx, "invalid-token-string"); err != nil {
		t.Fatalf("logout with invalid token should be idempotent, got %v", err)
	}
}
