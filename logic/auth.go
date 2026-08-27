package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/cache"
	"github.com/chihqiang/infra-go/hash"
	"github.com/chihqiang/infra-go/jwt"
	"github.com/chihqiang/infra-go/logger"
	"github.com/chihqiang/infra-go/ratelimit"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuthLogic 认证逻辑：账号登录、刷新令牌、当前账号信息、登录失败锁定。
// 应用换取 Token（OAuth client_credentials / authorization_code）的签发逻辑归口 OAuthLogic，
// 这里仅作为入口转发（见 Token）。
type AuthLogic struct {
	db         *gorm.DB
	j          *jwt.JWT
	cfg        config.Config
	oauthLogic *OAuthLogic

	// accountStore 账号信息缓存后端（注入 infra-go cache.Cache 接口，nil 表示不缓存）。
	// 供 GetAccountByID（认证中间件加载账号）使用：账号信息变更不频繁，短 TTL 缓存
	// 可显著减少认证路径的 DB 查询。账号变更点（禁用/删除/改密）须主动失效。
	accountStore cache.Cache

	// blacklistStore 访问令牌撤销黑名单后端（注入 infra-go cache.Cache 接口，nil 表示不启用）。
	// 登出时把当前 access token 的 jti 写入黑名单（TTL=令牌剩余有效期），
	// Auth 中间件校验黑名单，使已登出会话的访问令牌立即失效（不再依赖自然过期）。
	blacklistStore cache.Cache

	// loginLimiter 登录全局限流（ratelimit.Limiter 接口，默认内存令牌桶；
	// 配置 Redis 后由 main 注入 RedisTokenBucket，实现多节点共享），
	// 防密码撞库；账号级失败锁定在 Login 内单独处理。
	loginLimiter ratelimit.Limiter
	// registerLimiter 注册全局限流（同 loginLimiter，可注入 Redis 实现），防批量注册滥用。
	registerLimiter ratelimit.Limiter
	// refreshLimiter 刷新令牌全局限流（默认内存令牌桶，可注入 Redis 实现），
	// 防批量无效刷新请求消耗资源（每次刷新都涉及 JWT 解析 + 数据库事务）。
	refreshLimiter ratelimit.Limiter

	// dummyHash 假密码哈希：账号不存在时用于执行一次 Bcrypt 比较，
	// 使「账号不存在」与「密码错误」的响应耗时一致，防止时序侧信道枚举有效账号名。
	dummyHash string
}

// NewAuthLogic 创建认证逻辑。
func NewAuthLogic(db *gorm.DB, j *jwt.JWT, cfg config.Config) *AuthLogic {
	dummyHash, _ := hash.BcryptHashDefault("q-iam-timing-balance-dummy")
	return &AuthLogic{
		db:        db,
		j:         j,
		cfg:       cfg,
		dummyHash: dummyHash,
		// 登录：每秒补充 2 个令牌、突发 10（单机默认内存实现）
		loginLimiter: ratelimit.NewTokenBucket(2, 10),
		// 注册：每 10s 补充 1 个令牌、突发 5，显著遏制批量注册
		registerLimiter: ratelimit.NewTokenBucket(0.1, 5),
		// 刷新：每秒补充 5 个令牌、突发 20（低于换 Token 的 5/20，防批量刷新滥用）
		refreshLimiter: ratelimit.NewTokenBucket(5, 20),
	}
}

// SetLoginLimiter 注入登录全局限流器（配置 Redis 时传 ratelimit.RedisTokenBucket，
// 实现多节点共享；不传则保持内存实现）。
func (s *AuthLogic) SetLoginLimiter(l ratelimit.Limiter) {
	s.loginLimiter = l
}

// SetRegisterLimiter 注入注册全局限流器（配置 Redis 时传 ratelimit.RedisTokenBucket）。
func (s *AuthLogic) SetRegisterLimiter(l ratelimit.Limiter) {
	s.registerLimiter = l
}

// SetRefreshLimiter 注入刷新令牌全局限流器（配置 Redis 时传 ratelimit.RedisTokenBucket）。
func (s *AuthLogic) SetRefreshLimiter(l ratelimit.Limiter) {
	s.refreshLimiter = l
}

// SetOAuthLogic 注入 OAuth 授权逻辑（应用换取 Token 入口转发需要）。
func (s *AuthLogic) SetOAuthLogic(o *OAuthLogic) {
	s.oauthLogic = o
}

// SetAccountCache 注入账号信息缓存后端（实现 infra-go cache.Cache 接口，如 MemCache / RedisCache）。
// nil 表示不缓存。账号缓存短 TTL（accountCacheTTL），账号变更通过 InvalidateAccountCache 主动失效。
func (s *AuthLogic) SetAccountCache(st cache.Cache) {
	s.accountStore = st
}

// SetBlacklistStore 注入访问令牌撤销黑名单后端（复用 infra-go cache.Cache：无 Redis 用 MemCache，
// 配置 Redis 后为 RedisCache）。nil 表示不启用访问令牌撤销。
func (s *AuthLogic) SetBlacklistStore(st cache.Cache) {
	s.blacklistStore = st
}

// accessTokenRevokedKey 访问令牌撤销黑名单键（按 jti 唯一标识）。
func accessTokenRevokedKey(jti string) string {
	return "oauth:access:revoked:" + jti
}

// RevokeAccessToken 将访问令牌加入撤销黑名单（登出时吊销当前会话的 access token）。
// 解析令牌提取 jti，TTL 设为令牌剩余有效期，过期自动释放（黑名单不长期累积）；
// 令牌无效/已过期/缺少 jti 或未启用黑名单时静默忽略（幂等）。
func (s *AuthLogic) RevokeAccessToken(ctx context.Context, accessToken string) {
	if s.blacklistStore == nil || accessToken == "" {
		return
	}
	claims, err := s.j.ParseAccessToken(accessToken)
	if err != nil {
		return
	}
	jti, _ := claims[jwt.ClaimKeyJWTID].(string)
	if jti == "" {
		return
	}
	exp, _ := claims[jwt.ClaimKeyExpirationTime].(float64)
	ttl := time.Until(time.Unix(int64(exp), 0))
	if ttl <= 0 {
		return
	}
	// 黑名单键按 jti 唯一，SetEx 覆盖写即可（无需 SetNX 原子占位），TTL=令牌剩余有效期自动释放。
	if err := s.blacklistStore.SetEx(ctx, accessTokenRevokedKey(jti), "1", ttl); err != nil {
		logger.WarnCtx(ctx, "revoke access token failed", logger.Err(err))
	}
}

// IsAccessTokenRevoked 判断访问令牌 jti 是否已被吊销（Auth 中间件校验黑名单）。
func (s *AuthLogic) IsAccessTokenRevoked(ctx context.Context, jti string) bool {
	if s.blacklistStore == nil || jti == "" {
		return false
	}
	v, err := s.blacklistStore.Get(ctx, accessTokenRevokedKey(jti))
	if err != nil {
		// 未命中（cache.ErrNotFound）或存储故障：视为未吊销
		return false
	}
	str, ok := cacheGetString(v, nil)
	return ok && str != ""
}

// accountCacheTTL 账号信息缓存有效期。账号变更会主动失效，TTL 作为兜底。
const accountCacheTTL = 60 * time.Second

// accountCacheKeyFor 生成账号信息缓存键。
func accountCacheKeyFor(accountID int64) string {
	return fmt.Sprintf("acct:%d", accountID)
}

// InvalidateAccountCache 使某账号的信息缓存失效（账号变更后调用）。
// 用于禁用/删除/改密等场景：避免缓存的旧账号信息（如已禁用账号）被认证中间件继续放行。
func (s *AuthLogic) InvalidateAccountCache(ctx context.Context, accountID int64) {
	if s.accountStore == nil {
		return
	}
	if err := s.accountStore.Delete(ctx, accountCacheKeyFor(accountID)); err != nil {
		logger.WarnCtx(ctx, "invalidate account cache failed",
			logger.Err(err), logger.Int64("account_id", accountID))
	}
}

// DataPermissions 返回当前主体的权限规则与数据范围（转发到 OAuthLogic）。
// 供子系统按需拉取当前登录账号 / 应用的权限与数据范围。
func (s *AuthLogic) DataPermissions(ctx context.Context, claims jwt.Claims) (*DataPermissionsResponse, error) {
	if s.oauthLogic == nil {
		return nil, errors.New("数据权限接口未启用")
	}
	return s.oauthLogic.DataPermissions(ctx, claims)
}

// LoginRequest 登录请求。
type LoginRequest struct {
	AccountName string `json:"account_name" binding:"required"`
	Password    string `json:"password" binding:"required"`
}

// LoginResponse 登录响应。
type LoginResponse struct {
	ID          int64  `json:"id"`
	AccountName string `json:"account_name"`
	DisplayName string `json:"display_name"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	// ExpiresIn 访问令牌有效秒数（OAuth 标准语义，前端据此 + 当前时间推算过期时间戳）。
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	// AllowConsole 是否允许进入管理控制台（前端据此决定登录后跳转）。
	AllowConsole bool `json:"allow_console"`
	// PasswordExpired 密码是否已超过有效期（password_policy.expire_days），
	// 为 true 时前端应引导用户立即修改密码。
	PasswordExpired bool `json:"password_expired,omitempty"`
}

// accessTokenTTLSeconds 返回访问令牌有效秒数（统一令牌 TTL 换算入口）。
func (s *AuthLogic) accessTokenTTLSeconds() int64 {
	return int64(s.j.Config().AccessTokenExpire.Seconds())
}

// Login 账号密码登录。
// clientIP / userAgent 记录到刷新令牌表（签发来源，供安全追溯）。
func (s *AuthLogic) Login(ctx context.Context, req *LoginRequest, clientIP, userAgent string) (*LoginResponse, error) {
	// 全局限流：防撞库（账号级失败锁定在下方单独处理）
	if !s.loginLimiter.Allow() {
		return nil, errors.New("登录请求过于频繁，请稍后再试")
	}

	var account model.Account
	if err := s.db.WithContext(ctx).Where("account_name = ?", req.AccountName).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.WarnCtx(ctx, "auth login failed: account not found", logger.String("account_name", req.AccountName))
			// 时序侧信道防护：账号不存在也执行一次 Bcrypt 比较，
			// 与「密码错误」的响应耗时保持一致，防止通过耗时差异枚举有效账号名
			_ = hash.BcryptCompare(s.dummyHash, req.Password)
			return nil, errors.New("账号或密码错误")
		}
		return nil, err
	}

	// 登录锁定检查
	if account.LockedUntil != nil && account.LockedUntil.After(time.Now()) {
		logger.WarnCtx(ctx, "auth login failed: account locked",
			logger.Int64("account_id", account.ID),
			logger.Time("locked_until", *account.LockedUntil))
		return nil, errors.New("账号已锁定，请稍后再试")
	}

	if !account.Status {
		logger.WarnCtx(ctx, "auth login failed: account disabled", logger.Int64("account_id", account.ID))
		return nil, errors.New("账号已被禁用")
	}

	if err := hash.BcryptCompare(account.Password, req.Password); err != nil {
		if locked := s.recordLoginFailure(ctx, &account); locked {
			logger.WarnCtx(ctx, "auth login failed: locked after too many failures", logger.Int64("account_id", account.ID))
			return nil, errors.New("登录失败次数过多，账号已锁定")
		}
		logger.WarnCtx(ctx, "auth login failed: wrong password", logger.Int64("account_id", account.ID))
		return nil, errors.New("账号或密码错误")
	}

	// 登录成功：清除失败计数、更新最后登录时间
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&account).Updates(map[string]any{
		"login_fail_count": 0,
		"locked_until":     nil,
		"last_login_at":    now,
	}).Error; err != nil {
		logger.ErrorCtx(ctx, "auth login update failed", logger.Err(err))
	}

	claims := jwt.Claims{
		jwt.ClaimKeyUserID:   account.ID,
		jwt.ClaimKeyUsername: account.AccountName,
	}

	tokenPair, err := s.issueTokenPair(ctx, s.db, claims, account.ID, clientIP, userAgent)
	if err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "auth login ok",
		logger.Int64("account_id", account.ID),
		logger.String("account_name", account.AccountName))
	return &LoginResponse{
		ID:           account.ID,
		AccountName:  account.AccountName,
		DisplayName:  account.DisplayName,
		AccessToken:  tokenPair.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.accessTokenTTLSeconds(),
		RefreshToken: tokenPair.RefreshToken,
		AllowConsole: account.AllowConsole,
		// 登录成功（密码正确）时仍标记是否已过期，前端据此引导强制修改
		PasswordExpired: IsPasswordExpired(account.PasswordChangedAt, s.cfg.Security.PasswordPolicy.ExpireDays, now),
	}, nil
}

// issueTokenPair 签发令牌对（登录/注册/刷新统一入口）。
//
// access token 携带业务声明（不含 jti）；refresh token 额外携带唯一 jti，
// 并落库一条刷新令牌记录（q_iam_refresh_tokens），供后续轮换/吊销使用。
// db 可传事务（刷新时与旧记录消费同一事务，保证签发与吊销一致）。
func (s *AuthLogic) issueTokenPair(ctx context.Context, db *gorm.DB, claims jwt.Claims, accountID int64, clientIP, userAgent string) (*jwt.TokenPair, error) {
	now := time.Now()

	// access 与 refresh 各带独立 jti：
	//  - refresh 的 jti 是刷新令牌表主键（token_id），用于轮换/吊销；
	//  - access 的 jti 用于访问令牌撤销黑名单：登出时吊销当前 access token，
	//    Auth 中间件校验黑名单使已登出会话的访问令牌立即失效。
	accessJTI := uuid.NewString()
	refreshJTI := uuid.NewString()

	// 深拷贝 claims，access / refresh 各自携带独立 jti
	accessClaims := make(jwt.Claims, len(claims)+1)
	for k, v := range claims {
		accessClaims[k] = v
	}
	accessClaims[jwt.ClaimKeyJWTID] = accessJTI

	refreshClaims := make(jwt.Claims, len(claims)+1)
	for k, v := range claims {
		refreshClaims[k] = v
	}
	refreshClaims[jwt.ClaimKeyJWTID] = refreshJTI

	accessToken, err := s.j.GenerateAccessToken(accessClaims)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.j.GenerateRefreshToken(refreshClaims)
	if err != nil {
		return nil, err
	}

	// 落库刷新令牌记录（表存储，供轮换/吊销）
	rt := model.RefreshToken{
		TokenID:   refreshJTI,
		AccountID: accountID,
		ExpiresAt: now.Add(s.j.Config().RefreshTokenExpire),
		ClientIP:  clientIP,
		UserAgent: userAgent,
	}
	if err := db.WithContext(ctx).Create(&rt).Error; err != nil {
		return nil, err
	}

	return &jwt.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    now.Add(s.j.Config().AccessTokenExpire).Unix(),
	}, nil
}

// revokeAccountTokens 吊销某账号全部有效刷新令牌，并标记吊销原因。
// db 可传事务；用于改密/禁用/删除账号/重用检测等场景，使已签发会话全部失效。
func revokeAccountTokens(ctx context.Context, db *gorm.DB, accountID int64, reason string) error {
	revokedAt := time.Now()
	return db.WithContext(ctx).Model(&model.RefreshToken{}).
		Where("account_id = ? AND revoked_at IS NULL", accountID).
		Updates(map[string]any{
			"revoked_at":    &revokedAt,
			"revoke_reason": reason,
		}).Error
}

// 刷新令牌重用连坐时间窗：短时间内连续多次检测到重用才吊销全部（防误伤）。
const (
	// reuseGraceWindow 重用计数时间窗（固定窗口，首次检测后 TTL 到期自动重置）。
	reuseGraceWindow = 30 * time.Second
	// reuseGraceThreshold 时间窗内触发连坐（吊销全部）的重用次数阈值。
	reuseGraceThreshold = 2
)

// reuseCounterKey 账号刷新令牌重用计数键。
func reuseCounterKey(accountID int64) string {
	return "oauth:refresh:reuse:" + strconv.FormatInt(accountID, 10)
}

// shouldRevokeAllOnReuse 判断是否应吊销该账号全部刷新令牌（重用连坐）。
// 时间窗内累计重用次数达到阈值才连坐；存储后端不可用时保守回退为立即连坐。
func (s *AuthLogic) shouldRevokeAllOnReuse(ctx context.Context, accountID int64) bool {
	if s.accountStore == nil {
		return true
	}
	// cache.Cache.Increment 原子自增但不返回新值，自增后读回计数判断阈值。
	// 非严格原子（Increment 与 Get 之间可能叠加其他并发计数），方向保守：
	// 计数值只会偏大，只会更早触发连坐，不会削弱安全性。
	key := reuseCounterKey(accountID)
	if err := s.accountStore.Increment(ctx, key, 1); err != nil {
		logger.WarnCtx(ctx, "reuse counter failed, fallback to revoke all",
			logger.Err(err), logger.Int64("account_id", accountID))
		return true
	}
	// 计数窗口：自增后设置 TTL，到期自动重置（滑动窗口语义——持续重放会不断延长
	// 窗口并累积计数，更快触发连坐，方向保守安全）。
	if err := s.accountStore.Expire(ctx, key, reuseGraceWindow); err != nil {
		logger.WarnCtx(ctx, "reuse counter expire failed, fallback to revoke all",
			logger.Err(err), logger.Int64("account_id", accountID))
		return true
	}
	n, err := s.accountStore.Get(ctx, key)
	if err != nil {
		logger.WarnCtx(ctx, "reuse counter get failed, fallback to revoke all",
			logger.Err(err), logger.Int64("account_id", accountID))
		return true
	}
	cnt, ok := cacheGetInt64(n, nil)
	if !ok {
		return true
	}
	return cnt >= reuseGraceThreshold
}

// cleanBusinessClaims 从解析出的令牌声明中提取业务字段（移除标准声明与 jti/token_type）。
// 刷新签发新令牌对时使用，避免旧 jti 被继承导致轮换表记录冲突。
func cleanBusinessClaims(claims jwt.Claims) jwt.Claims {
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

// recordLoginFailure 记录一次登录失败，达到阈值后锁定账号。
// 返回是否触发锁定。
func (s *AuthLogic) recordLoginFailure(ctx context.Context, account *model.Account) bool {
	loginCfg := s.cfg.Security.Login
	if loginCfg.MaxFailCount <= 0 {
		return false
	}

	// 锁定已到期（Login 主流程只拦截未过期的锁）：先重置失败计数与锁定状态，
	// 再重新累计。否则锁定到期后第一次输错仍从旧计数继续（如 5→6 再次锁定），
	// 账号会被"反复锁定"，体验极差。
	if account.LockedUntil != nil {
		if err := s.db.WithContext(ctx).Model(&model.Account{}).
			Where("id = ? AND locked_until IS NOT NULL AND locked_until <= ?", account.ID, time.Now()).
			Updates(map[string]any{"login_fail_count": 0, "locked_until": nil}).Error; err != nil {
			logger.ErrorCtx(ctx, "auth reset expired lock failed", logger.Err(err))
		}
	}

	// 数据库原子自增，避免并发登录时读改写竞态
	// （多个请求同时读到旧计数 +1，导致阈值被绕过）。
	if err := s.db.WithContext(ctx).Model(&model.Account{}).
		Where("id = ?", account.ID).
		UpdateColumn("login_fail_count", gorm.Expr("login_fail_count + 1")).Error; err != nil {
		logger.ErrorCtx(ctx, "auth record login failure failed", logger.Err(err))
		return false
	}

	// 读回最新失败次数，判断是否达到锁定阈值
	var fresh model.Account
	if err := s.db.WithContext(ctx).Select("login_fail_count").First(&fresh, account.ID).Error; err != nil {
		logger.ErrorCtx(ctx, "auth reload login fail count failed", logger.Err(err))
		return false
	}
	if fresh.LoginFailCount < loginCfg.MaxFailCount {
		return false
	}

	// 触发锁定：仅当尚未锁定时设置，避免并发请求反复延长锁定时长
	lockUntil := time.Now().Add(loginCfg.LockDuration)
	res := s.db.WithContext(ctx).Model(&model.Account{}).
		Where("id = ? AND locked_until IS NULL", account.ID).
		UpdateColumn("locked_until", lockUntil)
	if res.Error != nil {
		logger.ErrorCtx(ctx, "auth lock account failed", logger.Err(res.Error))
		return false
	}
	if res.RowsAffected == 0 {
		// 已被其他并发请求锁定
		return true
	}

	logger.WarnCtx(ctx, "auth account locked",
		logger.Int64("account_id", account.ID),
		logger.Int("fail_count", fresh.LoginFailCount),
		logger.Duration("lock_duration", loginCfg.LockDuration))
	return true
}

// Token 应用换取访问令牌（入口转发）。
// 实际签发逻辑在 OAuthLogic.IssueToken（client_credentials / authorization_code），
// 请求/响应 DTO 亦定义在 oauth.go。
func (s *AuthLogic) Token(ctx context.Context, req *TokenRequest) (*TokenResponse, error) {
	if s.oauthLogic == nil {
		return nil, errors.New("应用 Token 功能未启用")
	}
	return s.oauthLogic.IssueToken(ctx, req)
}

// RefreshRequest 刷新令牌请求。
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh 使用刷新令牌换取新的令牌对（轮换）。
//
// 刷新令牌由 q_iam_refresh_tokens 表存储并维护生命周期：
//   - 表校验：token_id 必须存在、未吊销、未过期；
//   - 轮换：旧记录被原子消费（标记吊销），同一事务内签发新令牌对并落库；
//   - 重用检测：已吊销的令牌再次被使用（并发刷新或令牌重放）视为疑似盗用，
//     吊销该账号全部刷新令牌，使泄露的令牌彻底失效。
//
// clientIP / userAgent 记录到新签发记录的来源信息。
func (s *AuthLogic) Refresh(ctx context.Context, req *RefreshRequest, clientIP, userAgent string) (*LoginResponse, error) {
	// 全局限流：防批量无效刷新请求消耗资源
	if !s.refreshLimiter.Allow() {
		return nil, errors.New("刷新请求过于频繁，请稍后再试")
	}

	// 解析刷新令牌一次（校验签名与 token_type），后续复用其业务声明签发新令牌对
	claims, err := s.j.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, errors.New("刷新令牌无效或已过期")
	}

	tokenID, _ := claims[jwt.ClaimKeyJWTID].(string)
	userIDVal, ok := claims[jwt.ClaimKeyUserID].(float64)
	if !ok || userIDVal == 0 {
		return nil, errors.New("刷新令牌无效")
	}
	accountID := int64(userIDVal)

	// 校验账号仍存在且可用
	var account model.Account
	if err := s.db.WithContext(ctx).First(&account, accountID).Error; err != nil {
		return nil, errors.New("账号不存在")
	}
	if !account.Status {
		return nil, errors.New("账号已被禁用")
	}

	now := time.Now()
	var tokenPair *jwt.TokenPair
	reuseDetected := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 查刷新令牌记录（token_id 即唯一 jti）
		var rt model.RefreshToken
		if err := tx.Where("token_id = ?", tokenID).First(&rt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("刷新令牌无效或已过期")
			}
			return err
		}

		// 2. 过期校验（JWT 自身也会校验 exp，这里以表记录为准）
		if now.After(rt.ExpiresAt) {
			return errors.New("刷新令牌无效或已过期")
		}

		// 3. 记录已吊销：区分吊销来源。
		//    - logout（主动退出）：仅当前会话失效，令牌再使用不连坐其他会话（用户主动退出的残留请求）；
		//    - rotated/reuse（轮换后重放、重用）：疑似盗用，事务提交后吊销该账号全部刷新令牌。
		if rt.RevokedAt != nil {
			if rt.RevokeReason == model.RefreshTokenRevokeLogout {
				return errors.New("刷新令牌无效或已过期")
			}
			reuseDetected = true
			return nil
		}

		// 4. 原子消费旧记录：仅当仍有效时标记吊销（rotated）。
		//    RowsAffected=0 说明并发下刚被其他请求轮换（并发刷新/令牌重放），同样视为重用连坐。
		//    注意：此处不能 return error 吊销全部——那会连同本事务一起回滚，
		//    因此仅置标记，事务提交后再统一吊销该账号全部刷新令牌。
		revokedAt := now
		res := tx.Model(&model.RefreshToken{}).
			Where("token_id = ? AND revoked_at IS NULL", tokenID).
			Updates(map[string]any{
				"revoked_at":    &revokedAt,
				"revoke_reason": model.RefreshTokenRevokeRotated,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			reuseDetected = true
			return nil
		}

		// 5. 签发新令牌对（新 jti）并落库（同一事务，保证一致性）
		pair, err := s.issueTokenPair(ctx, tx, cleanBusinessClaims(claims), accountID, clientIP, userAgent)
		if err != nil {
			return err
		}
		tokenPair = pair
		return nil
	})
	if err != nil {
		// 顺带惰性清理该账号已过期的刷新令牌记录，避免表无限膨胀
		s.db.WithContext(ctx).Where("account_id = ? AND expires_at < ?", accountID, now).
			Delete(&model.RefreshToken{})
		return nil, err
	}

	// 重用检测（事务已提交）：疑似盗用。
	// 连坐加时间窗缓冲：短时间内连续多次重用才吊销该账号全部刷新令牌，
	// 偶发一次（客户端弱网重试/并发刷新竞态）只失败本次，不误伤其他设备会话。
	if reuseDetected {
		if s.shouldRevokeAllOnReuse(ctx, accountID) {
			logger.WarnCtx(ctx, "refresh token reuse detected, revoke all",
				logger.Int64("account_id", accountID))
			if err := revokeAccountTokens(ctx, s.db, accountID, model.RefreshTokenRevokeReuse); err != nil {
				logger.ErrorCtx(ctx, "revoke all refresh tokens failed",
					logger.Err(err), logger.Int64("account_id", accountID))
			}
		} else {
			logger.InfoCtx(ctx, "refresh token reuse within grace window, skip revoke-all",
				logger.Int64("account_id", accountID))
		}
		return nil, errors.New("刷新令牌无效或已过期")
	}

	return &LoginResponse{
		ID:           account.ID,
		AccountName:  account.AccountName,
		DisplayName:  account.DisplayName,
		AccessToken:  tokenPair.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.accessTokenTTLSeconds(),
		RefreshToken: tokenPair.RefreshToken,
		AllowConsole: account.AllowConsole,
		// 刷新同样携带过期标记，避免刷新后过期状态丢失
		PasswordExpired: IsPasswordExpired(account.PasswordChangedAt, s.cfg.Security.PasswordPolicy.ExpireDays, now),
	}, nil
}

// Logout 主动退出：吊销当前会话的刷新令牌。
// 幂等：令牌为空/无效/已过期/已吊销时同样返回成功，不向调用方暴露令牌状态。
func (s *AuthLogic) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}

	// 解析刷新令牌（取 jti 定位表记录）；解析失败视为无效令牌，幂等成功
	claims, err := s.j.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil
	}
	tokenID, _ := claims[jwt.ClaimKeyJWTID].(string)
	if tokenID == "" {
		return nil
	}

	// 吊销该会话记录，标记为主动退出（logout，不参与重用连坐）。
	// 已吊销/已轮换时 RowsAffected=0，仍视为成功（幂等）。
	revokedAt := time.Now()
	if err := s.db.WithContext(ctx).Model(&model.RefreshToken{}).
		Where("token_id = ?", tokenID).
		Updates(map[string]any{
			"revoked_at":    &revokedAt,
			"revoke_reason": model.RefreshTokenRevokeLogout,
		}).Error; err != nil {
		logger.ErrorCtx(ctx, "auth logout revoke failed", logger.Err(err))
		return err
	}

	accountID, _ := claims[jwt.ClaimKeyUserID].(float64)
	logger.InfoCtx(ctx, "auth logout ok", logger.Int64("account_id", int64(accountID)))
	return nil
}

// ProfileResponse 当前账号信息。
type ProfileResponse struct {
	ID           int64  `json:"id"`
	AccountName  string `json:"account_name"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Mobile       string `json:"mobile"`
	Status       bool   `json:"status"`
	Remark       string `json:"remark"`
	AllowConsole bool   `json:"allow_console"`
	// PasswordExpired 密码是否已超过有效期（password_policy.expire_days）。
	PasswordExpired bool `json:"password_expired,omitempty"`
	// Permissions 当前账号生效的权限规则（供前端按权限过滤菜单/按钮）。
	Permissions []PermissionStatement `json:"permissions,omitempty"`
}

// GetProfile 获取当前账号信息。
func (s *AuthLogic) GetProfile(ctx context.Context, accountID int64) (*ProfileResponse, error) {
	var account model.Account
	if err := s.db.WithContext(ctx).First(&account, accountID).Error; err != nil {
		return nil, err
	}

	return &ProfileResponse{
		ID:              account.ID,
		AccountName:     account.AccountName,
		DisplayName:     account.DisplayName,
		Email:           derefString(account.Email),
		Mobile:          derefString(account.Mobile),
		Status:          account.Status,
		Remark:          account.Remark,
		AllowConsole:    account.AllowConsole,
		PasswordExpired: IsPasswordExpired(account.PasswordChangedAt, s.cfg.Security.PasswordPolicy.ExpireDays, time.Now()),
	}, nil
}

// GetAccountByID 按 ID 查询账号（用于认证中间件加载账号）。
// 带短 TTL 缓存（accountCacheTTL）：账号信息变更不频繁，认证路径的高频查询命中缓存
// 可显著减少 DB 负载。账号变更点（禁用/删除/改密）通过 InvalidateAccountCache 主动失效。
// 缓存 JSON 不含敏感字段（Password 等带 json:"-" 不序列化），且未启用缓存时不缓存。
func (s *AuthLogic) GetAccountByID(ctx context.Context, accountID int64) (*model.Account, error) {
	// 优先命中缓存
	if s.accountStore != nil {
		key := accountCacheKeyFor(accountID)
		if data, err := s.accountStore.Get(ctx, key); err == nil {
			if str, ok := cacheGetString(data, nil); ok && str != "" {
				var account model.Account
				if err := json.Unmarshal([]byte(str), &account); err == nil {
					return &account, nil
				}
				logger.WarnCtx(ctx, "account cache unmarshal failed",
					logger.Err(err), logger.Int64("account_id", accountID))
			}
		}
	}

	var account model.Account
	if err := s.db.WithContext(ctx).First(&account, accountID).Error; err != nil {
		return nil, err
	}

	// 写入缓存（失败仅告警，不影响主流程）
	if s.accountStore != nil {
		if data, err := json.Marshal(account); err == nil {
			if err := s.accountStore.SetEx(ctx, accountCacheKeyFor(accountID), string(data), accountCacheTTL); err != nil {
				logger.WarnCtx(ctx, "account cache set failed",
					logger.Err(err), logger.Int64("account_id", accountID))
			}
		}
	}

	return &account, nil
}

// ChangeOwnPasswordRequest 当前登录账号修改自己密码的请求。
type ChangeOwnPasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangeOwnPassword 当前登录账号修改自己的密码（需校验旧密码）。
// 与管理员重置密码（ResetPassword）不同：本方法从登录态取账号 ID，不要求 iam:account:write 权限。
func (s *AuthLogic) ChangeOwnPassword(ctx context.Context, accountID int64, req *ChangeOwnPasswordRequest) error {
	var account model.Account
	if err := s.db.WithContext(ctx).First(&account, accountID).Error; err != nil {
		return errors.New("账号不存在")
	}
	if !account.Status {
		return errors.New("账号已被禁用")
	}

	// 校验旧密码
	if err := hash.BcryptCompare(account.Password, req.OldPassword); err != nil {
		return errors.New("旧密码错误")
	}

	// 新密码强度校验（依据全局密码策略）
	validator := NewPasswordValidator(s.cfg.Security.PasswordPolicy)
	if msg := validator.Validate(req.NewPassword, account.AccountName); msg != "" {
		return errors.New(msg)
	}

	// 密码历史检查（防止重复使用最近用过的密码）
	historyCount := s.cfg.Security.PasswordPolicy.HistoryCount
	if msg, err := CheckPasswordReuse(ctx, s.db, account.ID, req.NewPassword, historyCount); err != nil {
		return err
	} else if msg != "" {
		return errors.New(msg)
	}

	hashed, err := hash.BcryptHashDefault(req.NewPassword)
	if err != nil {
		return err
	}

	// 记录旧密码到历史 + 更新新密码与修改时间 + 吊销该账号全部刷新令牌（同一事务）。
	// 密码已变更，旧会话的刷新令牌应全部失效，防止泄露令牌继续使用。
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := RememberPassword(ctx, tx, account.ID, account.Password, historyCount); err != nil {
			return err
		}
		if err := tx.Model(&account).Updates(map[string]any{
			"password":            hashed,
			"password_changed_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		return revokeAccountTokens(ctx, tx, account.ID, model.RefreshTokenRevokeRevoke)
	}); err != nil {
		return err
	}

	// 改密后账号信息（password_changed_at 等）已变更，失效账号缓存
	s.InvalidateAccountCache(ctx, account.ID)
	return nil
}

// RegisterRequest 注册请求。
type RegisterRequest struct {
	AccountName string `json:"account_name" binding:"required"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email" binding:"omitempty,email"`
	Mobile      string `json:"mobile"`
	Password    string `json:"password" binding:"required"`
}

// Register 注册账号（公开接口）。
// 受 security.register.enabled 开关控制；注册即启用、并自动登录（签发 token）。
// clientIP / userAgent 记录到刷新令牌表（签发来源）。
func (s *AuthLogic) Register(ctx context.Context, req *RegisterRequest, clientIP, userAgent string) (*LoginResponse, error) {
	// 注册开关
	if !s.cfg.Security.Register.Enabled {
		return nil, errors.New("注册未开放")
	}

	// 全局限流：防批量注册滥用
	if !s.registerLimiter.Allow() {
		return nil, errors.New("注册请求过于频繁，请稍后再试")
	}

	req.AccountName = strings.TrimSpace(req.AccountName)
	if req.AccountName == "" {
		return nil, errors.New("账号名不能为空")
	}

	// 账号名唯一性检查
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Account{}).Where("account_name = ?", req.AccountName).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("账号名已存在")
	}

	// 密码强度校验
	validator := NewPasswordValidator(s.cfg.Security.PasswordPolicy)
	if msg := validator.Validate(req.Password, req.AccountName); msg != "" {
		return nil, errors.New(msg)
	}

	hashed, err := hash.BcryptHashDefault(req.Password)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	account := model.Account{
		AccountName: req.AccountName,
		DisplayName: req.DisplayName,
		Email:       nullableString(req.Email),
		Mobile:      nullableString(req.Mobile),
		Password:    hashed,
		Status:      true,
		// 注册账号不允许进入管理控制台，仅用于 OAuth2 授权登录
		AllowConsole:      false,
		PasswordChangedAt: &now,
	}
	if err := s.db.WithContext(ctx).Create(&account).Error; err != nil {
		// 并发下唯一约束兜底：转成友好提示，避免暴露原始 DB 错误
		return nil, normalizeDuplicateError(err)
	}

	logger.InfoCtx(ctx, "auth register ok",
		logger.Int64("account_id", account.ID),
		logger.String("account_name", account.AccountName))

	// 注册即登录：签发 token
	claims := jwt.Claims{
		jwt.ClaimKeyUserID:   account.ID,
		jwt.ClaimKeyUsername: account.AccountName,
	}
	tokenPair, err := s.issueTokenPair(ctx, s.db, claims, account.ID, clientIP, userAgent)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		ID:           account.ID,
		AccountName:  account.AccountName,
		DisplayName:  account.DisplayName,
		AccessToken:  tokenPair.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.accessTokenTTLSeconds(),
		RefreshToken: tokenPair.RefreshToken,
		AllowConsole: account.AllowConsole,
	}, nil
}
