package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/logic"
	"chihqiang/q-iam/logic/store"
	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/hash"
	"github.com/chihqiang/infra-go/jwt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newLoadAccountTestEnv 构造 LoadAccount 测试环境：
// 内存 SQLite + 一个启用账号（ID=1，模拟内置 admin 的 ID 空间）+ AuthLogic + JWT。
func newLoadAccountTestEnv(t *testing.T) (*jwt.JWT, *logic.AuthLogic, *gorm.DB, *model.Account) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Account{}, &model.KeyStoreItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.Config{}
	cfg.JWT.Secret = "test-secret-key"
	cfg.JWT.AccessTokenExpire = time.Hour
	cfg.JWT.RefreshTokenExpire = 24 * time.Hour

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

	return j, logic.NewAuthLogic(db, j, cfg), db, &acct
}

// chain 组装 Auth → LoadAccount → next 的完整中间件链（与 route.go 的 authed 组一致）。
func chain(t *testing.T, j *jwt.JWT, authSvc *logic.AuthLogic, next http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return Auth(j, nil)(LoadAccount(authSvc)(next))
}

// TestLoadAccount_RejectsAppToken 回归测试：应用令牌（client_credentials）不得被 LoadAccount
// 当作账号加载。
//
// 背景：历史上应用令牌的 user_id 声明曾写入 App 表主键，与账号表自增 ID 空间重叠——
// 若某应用 ID 恰好命中某账号（如内置 admin=1），会被误当成该账号加载，从而绕过权限校验。
// 修复后 LoadAccount 显式拦截 token_subject_type=app 的令牌。
// 本用例故意构造「带 user_id=账号ID 的 app 令牌」（最恶意的旧格式），
// 验证即使如此也一律拦截——一旦未来该拦截逻辑被移除，本测试立即失败。
func TestLoadAccount_RejectsAppToken(t *testing.T) {
	j, authSvc, _, acct := newLoadAccountTestEnv(t)

	// 构造恶意 app 令牌：携带 user_id=账号ID（模拟 ID 重叠攻击形态）
	appToken, err := j.GenerateAccessToken(jwt.Claims{
		logic.ClaimTokenSubjectType: logic.TokenSubjectTypeApp,
		logic.ClaimTokenAppID:       "app-001",
		jwt.ClaimKeyUserID:          float64(acct.ID),
		jwt.ClaimKeyUsername:        "some-app",
	})
	if err != nil {
		t.Fatalf("generate app token: %v", err)
	}

	nextCalled := false
	handler := chain(t, j, authSvc, func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		if AccountFromContext(r.Context()) != nil {
			t.Fatal("app token must not load an account into context")
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+appToken)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("app token should be rejected with 403, got %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("LoadAccount must not call next for app tokens")
	}
}

// TestLoadAccount_AllowsUserToken 用户令牌（登录 / authorization_code）应正常加载账号进入上下文。
// 防止修复过度拦截：确保普通用户令牌不受影响。
func TestLoadAccount_AllowsUserToken(t *testing.T) {
	j, authSvc, _, acct := newLoadAccountTestEnv(t)

	userToken, err := j.GenerateAccessToken(jwt.Claims{
		jwt.ClaimKeyUserID:   float64(acct.ID),
		jwt.ClaimKeyUsername: acct.AccountName,
	})
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	nextCalled := false
	handler := chain(t, j, authSvc, func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		loaded := AccountFromContext(r.Context())
		if loaded == nil {
			t.Fatal("user token should load account into context")
		}
		// 校验加载的账号与令牌声明一致（修复原 acct.ID != acct.ID 死代码）
		if loaded.ID != acct.ID || loaded.AccountName != acct.AccountName {
			t.Fatalf("loaded account mismatch: got id=%d name=%s, want id=%d name=%s",
				loaded.ID, loaded.AccountName, acct.ID, acct.AccountName)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("user token should pass through with 200, got %d", rec.Code)
	}
	if !nextCalled {
		t.Fatal("LoadAccount should call next for user tokens")
	}
}

// TestLoadAccount_RejectsDisabledAccount 被禁用的账号即使携带有效用户令牌也应被拒绝（403）。
func TestLoadAccount_RejectsDisabledAccount(t *testing.T) {
	j, authSvc, db, acct := newLoadAccountTestEnv(t)

	// 禁用账号
	if err := db.Model(&model.Account{}).Where("id = ?", acct.ID).Update("status", false).Error; err != nil {
		t.Fatalf("disable account: %v", err)
	}

	userToken, err := j.GenerateAccessToken(jwt.Claims{
		jwt.ClaimKeyUserID:   float64(acct.ID),
		jwt.ClaimKeyUsername: acct.AccountName,
	})
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	nextCalled := false
	handler := chain(t, j, authSvc, func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled account should be rejected with 403, got %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("LoadAccount must not call next for disabled accounts")
	}
}

// TestLoadAccount_AccountCacheHit 验证账号信息缓存：命中后账号数据来自缓存（DB 变更不立即反映），
// 证明认证路径确实走缓存、不再每次查库。
func TestLoadAccount_AccountCacheHit(t *testing.T) {
	j, authSvc, db, acct := newLoadAccountTestEnv(t)
	// 注入 DBStore 账号缓存（与生产默认一致）
	authSvc.SetAccountCache(store.NewDBStore(db))

	userToken, err := j.GenerateAccessToken(jwt.Claims{
		jwt.ClaimKeyUserID:   float64(acct.ID),
		jwt.ClaimKeyUsername: acct.AccountName,
	})
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	// 第一次请求：加载账号并写入缓存
	var displayNameFromCache string
	handler := chain(t, j, authSvc, func(w http.ResponseWriter, r *http.Request) {
		acct := AccountFromContext(r.Context())
		displayNameFromCache = acct.DisplayName
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	handler(httptest.NewRecorder(), req)
	if displayNameFromCache != "Tester" {
		t.Fatalf("expected display name Tester, got %s", displayNameFromCache)
	}

	// 直接改 DB（绕过业务逻辑，模拟数据变更），缓存未失效
	if err := db.Model(&model.Account{}).Where("id = ?", acct.ID).Update("display_name", "Renamed").Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	// 第二次请求：缓存命中，读到旧值（证明走缓存）
	displayNameFromCache = ""
	handler(httptest.NewRecorder(), req)
	if displayNameFromCache != "Tester" {
		t.Fatalf("cache hit should return cached display name Tester, got %s", displayNameFromCache)
	}

	// 失效缓存后：重新读 DB，拿到新值
	authSvc.InvalidateAccountCache(context.Background(), acct.ID)
	displayNameFromCache = ""
	handler(httptest.NewRecorder(), req)
	if displayNameFromCache != "Renamed" {
		t.Fatalf("after invalidation should read fresh display name Renamed, got %s", displayNameFromCache)
	}
}

// TestLoadAccount_RejectsDisabledAccountAfterCacheInvalidation 验证「禁用账号 + 账号缓存失效」联动：
// 账号被禁用后，即使旧账号信息曾在缓存中，失效后认证中间件能读到禁用状态并拒绝（403）。
func TestLoadAccount_RejectsDisabledAccountAfterCacheInvalidation(t *testing.T) {
	j, authSvc, db, acct := newLoadAccountTestEnv(t)
	authSvc.SetAccountCache(store.NewDBStore(db))

	userToken, err := j.GenerateAccessToken(jwt.Claims{
		jwt.ClaimKeyUserID:   float64(acct.ID),
		jwt.ClaimKeyUsername: acct.AccountName,
	})
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	// 先走一次请求：账号入缓存（启用状态）
	handler := chain(t, j, authSvc, func(w http.ResponseWriter, r *http.Request) {})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	handler(httptest.NewRecorder(), req)

	// 禁用账号 + 失效账号缓存（模拟 AccountLogic.Update 禁用后调用失效）
	if err := db.Model(&model.Account{}).Where("id = ?", acct.ID).Update("status", false).Error; err != nil {
		t.Fatalf("disable: %v", err)
	}
	authSvc.InvalidateAccountCache(context.Background(), acct.ID)

	// 再次请求：应从 DB 读到禁用状态并拒绝
	nextCalled := false
	handler = chain(t, j, authSvc, func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled account should be rejected with 403 after cache invalidation, got %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("LoadAccount must not call next for disabled accounts")
	}
}
