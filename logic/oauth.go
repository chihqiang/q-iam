package logic

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/cache"
	"github.com/chihqiang/infra-go/jwt"
	"github.com/chihqiang/infra-go/logger"
	"github.com/chihqiang/infra-go/ratelimit"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OAuth 授权码自定义声明键。
const (
	claimOAuthType  = "oauth_type" // 值为 "code"
	claimOAuthAppID = "oauth_app_id"
	claimOAuthScope = "oauth_scope"
)

// 访问令牌主体类型 / 声明键常量已统一收拢到 token.go
// （TokenSubjectTypeUser / TokenSubjectTypeApp / ClaimTokenSubjectType / ClaimTokenAppID）。

// OAuthCodeExpire 授权码有效期（短时效，一次性使用）。
const OAuthCodeExpire = 5 * time.Minute

// OAuth 协议错误码（RFC 6749 §5.2 / RFC 6750 §3）。
// 供 IssueToken / UserInfo 等 OAuth 端点返回，handler 据此映射为标准 HTTP 状态码
// + {error, error_description} 响应体（而非业务层统一的 200 + {code, msg}），
// 兼容标准 OAuth 客户端（第三方接入）。
const (
	// OAuthErrorInvalidRequest 请求缺少必需参数 / 参数非法。
	OAuthErrorInvalidRequest = "invalid_request"
	// OAuthErrorInvalidClient 客户端认证失败（凭证无效 / 应用禁用）。
	OAuthErrorInvalidClient = "invalid_client"
	// OAuthErrorInvalidGrant 授权码 / 刷新令牌无效、过期或已被使用。
	OAuthErrorInvalidGrant = "invalid_grant"
	// OAuthErrorUnauthorizedClient 客户端无权使用该授权类型。
	OAuthErrorUnauthorizedClient = "unauthorized_client"
	// OAuthErrorUnsupportedGrantType 不支持的授权类型。
	OAuthErrorUnsupportedGrantType = "unsupported_grant_type"
	// OAuthErrorInvalidScope 请求的 scope 无效 / 超出范围。
	OAuthErrorInvalidScope = "invalid_scope"
	// OAuthErrorInvalidToken 访问令牌无效 / 过期（RFC 6750）。
	OAuthErrorInvalidToken = "invalid_token"
	// OAuthErrorTemporarilyUnavailable 服务暂时不可用（限流）。
	OAuthErrorTemporarilyUnavailable = "temporarily_unavailable"
)

// OAuthError OAuth 2.0 协议错误（RFC 6749 §5.2 错误码）。
// OAuth 端点（IssueToken / UserInfo）返回此类错误，handler 据此映射为
// 标准 HTTP 状态码 + {error, error_description} 响应体。
type OAuthError struct {
	// Code OAuth 错误码（见 OAuthError* 常量）。
	Code string
	// Description 错误描述（RFC 6749 error_description，可空）。
	Description string
	// Status HTTP 状态码（0 时 handler 默认 400）。
	Status int
}

// Error 实现 error 接口。
func (e *OAuthError) Error() string {
	return e.Description
}

// OAuthLogic OAuth 授权逻辑（authorization_code 流程）。
// 用户授权后签发一次性授权码（code），应用凭 code 换取访问令牌。
type OAuthLogic struct {
	db        *gorm.DB
	j         *jwt.JWT
	appLogic  *AppLogic // 应用凭证校验（应用换取 Token）
	permLogic *PermissionLogic

	// consumedStore 授权码一次性消费的存储后端（注入 infra-go cache.Cache 接口）。
	// 无 Redis 用 MemCache（进程内）；配置 Redis 后由 svc 注入 RedisCache（多节点共享）。
	// 授权码本身短时效（OAuthCodeExpire），消费记录 TTL 相同时长即可。
	consumedStore cache.Cache

	// tokenLimiter 应用换取 Token 全局限流（ratelimit.Limiter 接口，默认内存令牌桶；
	// 配置 Redis 后由 main 注入 RedisTokenBucket，实现多节点共享），防凭证暴力枚举。
	tokenLimiter ratelimit.Limiter
}

// NewOAuthLogic 创建 OAuth 授权逻辑。
func NewOAuthLogic(db *gorm.DB, j *jwt.JWT) *OAuthLogic {
	return &OAuthLogic{
		db: db,
		j:  j,
		// 授权码一次性消费存储由 svc 装配注入（无 Redis 用 MemCache，配置 Redis 用 RedisCache）；
		// 未注入时 consumeCode 保守拒绝（fail-closed），保证防重放不静默失效。
		consumedStore: nil,
		// 换 Token：每秒补充 5 个令牌、突发 20（单机默认内存实现）
		tokenLimiter: ratelimit.NewTokenBucket(5, 20),
	}
}

// SetConsumedStore 注入授权码一次性消费的存储后端（无 Redis 用 MemCache，配置 Redis 用 RedisCache）。
func (o *OAuthLogic) SetConsumedStore(st cache.Cache) {
	o.consumedStore = st
}

// SetTokenLimiter 注入应用换取 Token 全局限流器（配置 Redis 时传 ratelimit.RedisTokenBucket，
// 实现多节点共享；不传则保持内存实现）。
func (o *OAuthLogic) SetTokenLimiter(l ratelimit.Limiter) {
	o.tokenLimiter = l
}

// SetAppLogic 注入应用逻辑（应用换取 Token 时校验凭证需要）。
func (o *OAuthLogic) SetAppLogic(appLogic *AppLogic) {
	o.appLogic = appLogic
}

// SetPermissionLogic 注入权限逻辑（UserInfo 返回用户权限时需要）。
func (o *OAuthLogic) SetPermissionLogic(permLogic *PermissionLogic) {
	o.permLogic = permLogic
}

// AppInfo 查询应用信息（供授权页展示，校验 client_id 有效性）。
func (o *OAuthLogic) AppInfo(ctx context.Context, clientID string) (*model.App, error) {
	var app model.App
	if err := o.db.WithContext(ctx).Where("app_id = ?", clientID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("应用不存在")
		}
		return nil, err
	}
	return &app, nil
}

// TokenRequest 应用换取 Token 请求（OAuth client_credentials / authorization_code）。
type TokenRequest struct {
	// GrantType 授权类型：client_credentials（默认）| authorization_code。
	GrantType string `json:"grant_type"`
	// AppID 客户端 ID。
	AppID string `json:"app_id" binding:"required"`
	// AppSecret 客户端密钥。
	AppSecret string `json:"app_secret" binding:"required"`
	// Code 授权码（grant_type=authorization_code 时必填）。
	Code string `json:"code"`
	// Scope 请求的授权范围（空格分隔，可选，签发后写入 claims）。
	Scope string `json:"scope"`
}

// TokenResponse 应用 Token 响应。
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// IssueToken 应用换取访问令牌（OAuth client_credentials / authorization_code）。
// client_credentials：校验 app_id + app_secret，签发主体为应用的 JWT。
// authorization_code：校验应用凭证 + 授权码 code，签发主体为被授权用户的 JWT。
func (o *OAuthLogic) IssueToken(ctx context.Context, req *TokenRequest) (*TokenResponse, error) {
	// 全局限流：防应用凭证暴力枚举
	if !o.tokenLimiter.Allow() {
		return nil, &OAuthError{Code: OAuthErrorTemporarilyUnavailable, Description: "请求过于频繁，请稍后再试", Status: http.StatusServiceUnavailable}
	}

	if o.appLogic == nil {
		return nil, errors.New("应用 Token 功能未启用")
	}

	// 校验应用凭证（两种模式都需要）
	app, err := o.appLogic.VerifyCredential(ctx, req.AppID, req.AppSecret)
	if err != nil {
		logger.WarnCtx(ctx, "app token failed: invalid credential")
		return nil, &OAuthError{Code: OAuthErrorInvalidClient, Description: "应用凭证无效", Status: http.StatusUnauthorized}
	}

	// 校验请求的授权类型与应用支持的授权类型一致（防止用不匹配模式换取令牌）
	if req.GrantType == "" {
		req.GrantType = model.AppGrantTypeClientCredentials
	}
	if app.GrantType != req.GrantType {
		return nil, &OAuthError{Code: OAuthErrorUnauthorizedClient, Description: "应用不支持该授权类型", Status: http.StatusBadRequest}
	}

	claims := jwt.Claims{}
	// 访问令牌携带唯一 jti，供撤销黑名单（登出/安全吊销等场景吊销当前令牌）
	claims[jwt.ClaimKeyJWTID] = uuid.NewString()
	scope := req.Scope

	// 两种模式都记录应用 ID 与主体类型，供 UserInfo Endpoint 识别
	claims[ClaimTokenAppID] = app.AppID
	claims[ClaimTokenSubjectType] = TokenSubjectTypeApp

	if req.GrantType == model.AppGrantTypeAuthorizationCode {
		// 授权码模式：校验授权码，令牌代表被授权的用户
		if req.Code == "" {
			return nil, &OAuthError{Code: OAuthErrorInvalidRequest, Description: "缺少授权码 code", Status: http.StatusBadRequest}
		}
		codeClaims, err := o.ExchangeCode(ctx, req.Code)
		if err != nil {
			logger.WarnCtx(ctx, "app token failed: invalid code")
			return nil, &OAuthError{Code: OAuthErrorInvalidGrant, Description: "授权码无效或已使用", Status: http.StatusBadRequest}
		}
		if codeClaims.AppID != req.AppID {
			return nil, &OAuthError{Code: OAuthErrorInvalidGrant, Description: "授权码与应用不匹配", Status: http.StatusBadRequest}
		}
		claims[jwt.ClaimKeySubject] = TokenSubjectTypeUser + ":" + codeClaims.AccountName
		claims[jwt.ClaimKeyUserID] = codeClaims.AccountID
		claims[jwt.ClaimKeyUsername] = codeClaims.AccountName
		claims[ClaimTokenSubjectType] = TokenSubjectTypeUser
		// 授权码中携带的 scope 优先
		if codeClaims.Scope != "" {
			scope = codeClaims.Scope
		}
	} else {
		// 客户端凭证模式：令牌代表应用本身。
		// 注意：不写入 user_id 声明（App 表主键与账号表自增 ID 空间重叠，
		// 写入后若命中某账号会被 LoadAccount 误当成账号加载，绕过权限校验）。
		// 应用令牌的主体识别依赖 token_subject_type=app + app_id，而非 user_id。
		claims[jwt.ClaimKeySubject] = TokenSubjectTypeApp + ":" + app.AppID
		claims[jwt.ClaimKeyUsername] = app.Name
	}

	if scope != "" {
		claims[jwt.ClaimKeyScopes] = scope
	}

	token, err := o.j.GenerateAccessToken(claims)
	if err != nil {
		return nil, err
	}

	expiresIn := int64(o.j.Config().AccessTokenExpire.Seconds())

	logger.InfoCtx(ctx, "app token issued",
		logger.String("grant_type", req.GrantType),
		logger.Int64("app_id", app.ID),
		logger.String("app_name", app.Name))
	return &TokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		Scope:       scope,
	}, nil
}

// Authorize 校验授权请求并签发一次性授权码。
// account 为当前已登录的授权人；redirectURI 须与应用的 callback_url 匹配。
// 返回应用信息 + 授权码。
func (o *OAuthLogic) Authorize(ctx context.Context, account *model.Account, clientID, redirectURI, scope string) (*model.App, string, error) {
	app, err := o.AppInfo(ctx, clientID)
	if err != nil {
		return nil, "", err
	}

	// 应用须启用
	if !app.Status {
		return nil, "", errors.New("应用已禁用")
	}

	// 应用须支持授权码模式
	if app.GrantType != model.AppGrantTypeAuthorizationCode {
		return nil, "", errors.New("应用不支持授权码模式")
	}

	// 回调地址校验：授权码模式必须配置回调地址，且精确匹配（防止开放重定向）
	if app.CallbackURL == "" {
		return nil, "", errors.New("应用未配置回调地址")
	}
	if !strings.EqualFold(app.CallbackURL, redirectURI) {
		return nil, "", errors.New("回调地址不匹配")
	}

	// 签发授权码（无状态 JWT，短时效，含授权人与应用信息）
	// 携带唯一 jti，换取令牌时据此做一次性消费校验。
	code, err := o.j.GenerateToken(jwt.Claims{
		jwt.ClaimKeySubject:  "oauth:code",
		jwt.ClaimKeyJWTID:    uuid.NewString(),
		claimOAuthType:       "code",
		claimOAuthAppID:      app.AppID,
		claimOAuthScope:      scope,
		jwt.ClaimKeyUserID:   account.ID,
		jwt.ClaimKeyUsername: account.AccountName,
	}, OAuthCodeExpire)
	if err != nil {
		return nil, "", err
	}

	logger.InfoCtx(ctx, "oauth code issued",
		logger.String("app_id", app.AppID),
		logger.Int64("user_id", account.ID),
		logger.String("scope", scope))
	return app, code, nil
}

// CodeClaims 授权码解析结果。
type CodeClaims struct {
	AppID       string
	AccountID   int64
	AccountName string
	Scope       string
}

// ExchangeCode 校验授权码，返回其中携带的主体与应用信息。
// 校验签名、类型、应用仍启用。
func (o *OAuthLogic) ExchangeCode(ctx context.Context, code string) (*CodeClaims, error) {
	claims, err := o.j.ParseToken(code)
	if err != nil {
		return nil, errors.New("授权码无效或已过期")
	}

	// 类型校验
	if claims[claimOAuthType] != "code" {
		return nil, errors.New("授权码无效")
	}

	// 一次性使用校验：授权码携带唯一 jti，已被消费过的拒绝再次换取令牌
	jti, _ := claims[jwt.ClaimKeyJWTID].(string)
	if jti == "" || !o.consumeCode(ctx, jti) {
		return nil, errors.New("授权码无效或已使用")
	}

	// 主体信息
	userID, _ := claims[jwt.ClaimKeyUserID].(float64)
	accountName, _ := claims[jwt.ClaimKeyUsername].(string)
	appID, _ := claims[claimOAuthAppID].(string)
	scope, _ := claims[claimOAuthScope].(string)
	if appID == "" {
		return nil, errors.New("授权码无效")
	}

	// 应用须仍启用
	app, err := o.AppInfo(ctx, appID)
	if err != nil {
		return nil, err
	}
	if !app.Status {
		return nil, errors.New("应用已禁用")
	}

	return &CodeClaims{
		AppID:       appID,
		AccountID:   int64(userID),
		AccountName: accountName,
		Scope:       scope,
	}, nil
}

// consumeCode 消费一个授权码 jti，返回是否消费成功（false 表示已被消费过）。
// 基于注入的 infra-go cache.Cache 用原子自增实现一次性语义：
//   - Increment 原子自增（键不存在初始化为 1），值 == 1 表示首次消费（成功）；值 > 1 已被消费（拒绝）；
//   - 并发保证：原子自增保证最多一个请求读到 1，杜绝授权码重放（其余请求读到的值只会更大）；
//   - 自增后 Expire 设置 TTL（与授权码有效期一致），过期后键自动释放，无需手动清理。
//
// 注意：Increment 后 Get 非严格原子，极端并发下可能出现多个请求都读到 >1（全部拒绝），
// 方向保守（宁缺勿滥），不会出现两个请求同时消费成功。
func (o *OAuthLogic) consumeCode(ctx context.Context, jti string) bool {
	if o.consumedStore == nil {
		// 未注入消费存储（装配缺失）：fail-closed 拒绝，避免防重放静默失效
		logger.WarnCtx(ctx, "oauth consume store not configured, deny")
		return false
	}

	key := "oauth:code:consumed:" + jti
	if err := o.consumedStore.Increment(ctx, key, 1); err != nil {
		logger.ErrorCtx(ctx, "oauth consume code failed", logger.Err(err))
		// 存储故障时按未消费处理（放行），避免可用性受影响；
		// 授权码本身短时效 + 应用凭证校验兜底，风险可控。
		return true
	}
	if err := o.consumedStore.Expire(ctx, key, OAuthCodeExpire); err != nil {
		logger.ErrorCtx(ctx, "oauth consume code expire failed", logger.Err(err))
	}
	v, err := o.consumedStore.Get(ctx, key)
	if err != nil {
		logger.ErrorCtx(ctx, "oauth consume code get failed", logger.Err(err))
		return true
	}
	n, ok := cacheGetInt64(v, nil)
	if !ok {
		return true
	}
	return n == 1
}

// UserInfo 用户信息（UserInfo Endpoint 响应，对齐 OAuth 2.0 / OIDC 规范）。
type UserInfo struct {
	// Sub 主体标识（OAuth 2.0 标准字段）。
	Sub string `json:"sub"`
	// ClientID 应用客户端 ID。
	ClientID string `json:"client_id"`
	// AppName 应用名称。
	AppName string `json:"app_name"`
	// Scope 已授权范围。
	Scope string `json:"scope,omitempty"`
	// Aud 受众（应用 ID，对齐 OAuth 2.0 标准字段）。
	Aud string `json:"aud,omitempty"`

	// User 用户信息（仅 authorization_code 模式签发的令牌有）。
	User *UserInfoDetail `json:"user,omitempty"`
	// Permissions 用户生效的权限规则（仅用户令牌有）。
	Permissions []PermissionStatement `json:"permissions,omitempty"`
}

// UserInfoDetail 用户详情。
type UserInfoDetail struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Mobile      string `json:"mobile"`
}

// UserInfo 根据访问令牌解析 UserInfo（资源服务器接口）。
// 识别令牌主体类型（token_subject_type 业务声明，AuthMiddleware 会保留业务声明）：
//   - user （authorization_code 模式）→ 返回用户信息 + 权限 + 应用信息
//   - app  （client_credentials 模式）→ 返回应用信息 + 令牌携带的 scope
func (o *OAuthLogic) UserInfo(ctx context.Context, claims jwt.Claims) (*UserInfo, error) {
	if claims == nil {
		return nil, &OAuthError{Code: OAuthErrorInvalidToken, Description: "无效的访问令牌", Status: http.StatusUnauthorized}
	}

	subjectType, _ := claims[ClaimTokenSubjectType].(string)
	appID, _ := claims[ClaimTokenAppID].(string)
	scope, _ := claims[jwt.ClaimKeyScopes].(string)
	if appID == "" {
		return nil, &OAuthError{Code: OAuthErrorInvalidToken, Description: "令牌缺少应用信息", Status: http.StatusUnauthorized}
	}

	// 应用信息（不存在/被删 → 令牌无效）
	app, err := o.AppInfo(ctx, appID)
	if err != nil {
		return nil, &OAuthError{Code: OAuthErrorInvalidToken, Description: "令牌无效或已过期", Status: http.StatusUnauthorized}
	}

	// 构造主体标识（标准 sub 被 AuthMiddleware 过滤，这里由业务字段重建）
	accountName, _ := claims[jwt.ClaimKeyUsername].(string)
	sub := TokenSubjectTypeApp + ":" + appID
	if subjectType == TokenSubjectTypeUser {
		sub = TokenSubjectTypeUser + ":" + accountName
	}

	resp := &UserInfo{
		Sub:      sub,
		ClientID: appID,
		AppName:  app.Name,
		Scope:    scope,
		Aud:      appID,
	}

	// 用户令牌：补充用户信息与权限
	if subjectType == TokenSubjectTypeUser {
		userID, _ := claims[jwt.ClaimKeyUserID].(float64)
		var account model.Account
		if err := o.db.WithContext(ctx).First(&account, int64(userID)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, &OAuthError{Code: OAuthErrorInvalidToken, Description: "用户不存在或已被删除", Status: http.StatusUnauthorized}
			}
			return nil, err
		}
		// 用户信息字段按 scope 裁剪（OIDC 风格）：email/phone/mobile/profile 需对应 scope
		detail := &UserInfoDetail{
			AccountID:   account.ID,
			AccountName: account.AccountName,
		}
		if scopeAllowsField(scope, "profile") {
			detail.DisplayName = account.DisplayName
		}
		if scopeAllowsField(scope, "email") {
			detail.Email = derefString(account.Email)
		}
		if scopeAllowsField(scope, "phone") || scopeAllowsField(scope, "mobile") {
			detail.Mobile = derefString(account.Mobile)
		}
		resp.User = detail
		// 权限列表：按 scope 裁剪
		if o.permLogic != nil {
			perms, err := o.permLogic.LoadPermissionStatements(ctx, account.ID)
			if err != nil {
				return nil, err
			}
			resp.Permissions = filterPermissionsByScope(perms, scope)
		}
	} else {
		// 应用令牌（client_credentials）：返回应用绑定的策略权限，供资源服务器做授权判定。
		if o.permLogic != nil {
			perms, err := o.permLogic.LoadPermissionStatementsByPrincipal(ctx, model.PrincipalTypeApp, app.ID)
			if err != nil {
				return nil, err
			}
			resp.Permissions = filterPermissionsByScope(perms, scope)
		}
	}

	logger.InfoCtx(ctx, "oauth userinfo",
		logger.String("sub", sub),
		logger.String("app_id", appID))
	return resp, nil
}

// DataPermissionUser 数据权限响应中的用户信息（含所属账号组，供子系统按组解析数据范围）。
type DataPermissionUser struct {
	// AccountID 账号 ID。
	AccountID int64 `json:"account_id"`
	// AccountName 账号名。
	AccountName string `json:"account_name"`
	// DisplayName 显示名。
	DisplayName string `json:"display_name"`
	// GroupIDs 所属启用的账号组 ID（数据范围 scope_type=group 时按此解析）。
	GroupIDs []int64 `json:"group_ids"`
}

// DataPermissionApp 数据权限响应中的应用信息。
type DataPermissionApp struct {
	// AppID 客户端 ID。
	AppID string `json:"app_id"`
	// Name 应用名。
	Name string `json:"name"`
}

// DataPermissionsResponse 数据权限响应（子系统按需拉取，含权限规则 + 数据范围）。
type DataPermissionsResponse struct {
	// SubjectType 主体类型：user | app。
	SubjectType string `json:"subject_type"`
	// User 用户信息（用户令牌）。
	User *DataPermissionUser `json:"user,omitempty"`
	// App 应用信息（应用令牌）。
	App *DataPermissionApp `json:"app,omitempty"`
	// Permissions 生效的权限规则（每条含 data_scopes 数据范围）。
	Permissions []PermissionStatement `json:"permissions"`
}

// DataPermissions 返回当前主体的权限规则与数据范围（供子系统按需拉取）。
// 用户令牌：账号 + 所属账号组聚合出的权限（含 data_scopes）；
// 应用令牌：应用绑定策略的权限（含 data_scopes）。
// 令牌识别：client_credentials 令牌带 token_subject_type=app；登录令牌与
// authorization_code 令牌均携带 user_id（登录令牌无 token_subject_type，按用户处理）。
func (o *OAuthLogic) DataPermissions(ctx context.Context, claims jwt.Claims) (*DataPermissionsResponse, error) {
	if claims == nil {
		return nil, errors.New("无效的访问令牌")
	}

	subjectType, _ := claims[ClaimTokenSubjectType].(string)
	userID, _ := claims[jwt.ClaimKeyUserID].(float64)
	scope, _ := claims[jwt.ClaimKeyScopes].(string)
	resp := &DataPermissionsResponse{SubjectType: TokenSubjectTypeUser}

	switch {
	case subjectType == TokenSubjectTypeApp:
		resp.SubjectType = TokenSubjectTypeApp
		appID, _ := claims[ClaimTokenAppID].(string)
		app, err := o.AppInfo(ctx, appID)
		if err != nil {
			return nil, err
		}
		resp.App = &DataPermissionApp{AppID: app.AppID, Name: app.Name}
		if o.permLogic != nil {
			perms, err := o.permLogic.LoadPermissionStatementsByPrincipal(ctx, model.PrincipalTypeApp, app.ID)
			if err != nil {
				return nil, err
			}
			resp.Permissions = filterPermissionsByScope(perms, scope)
		}
	case userID > 0:
		var account model.Account
		if err := o.db.WithContext(ctx).Preload("Groups").First(&account, int64(userID)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("用户不存在或已被删除")
			}
			return nil, err
		}
		// 账号所属启用的账号组（数据范围 scope_type=group 时按此解析）
		groupIDs := make([]int64, 0, len(account.Groups))
		for _, g := range account.Groups {
			if g.Status {
				groupIDs = append(groupIDs, g.ID)
			}
		}
		resp.User = &DataPermissionUser{
			AccountID:   account.ID,
			AccountName: account.AccountName,
			DisplayName: account.DisplayName,
			GroupIDs:    groupIDs,
		}
		if o.permLogic != nil {
			perms, err := o.permLogic.LoadPermissionStatements(ctx, account.ID)
			if err != nil {
				return nil, err
			}
			resp.Permissions = filterPermissionsByScope(perms, scope)
		}
	default:
		return nil, errors.New("无法识别的令牌主体类型")
	}

	logger.InfoCtx(ctx, "data permissions",
		logger.String("subject_type", resp.SubjectType),
		logger.Int("permission_count", len(resp.Permissions)))
	return resp, nil
}

// filterPermissionsByScope 按 scope 裁剪权限语句（UserInfo / DataPermissions 共用）。
// scope 为空或 "*" 时原样返回（兼容现状：不限制）。
func filterPermissionsByScope(perms []PermissionStatement, scope string) []PermissionStatement {
	if strings.TrimSpace(scope) == "" || strings.TrimSpace(scope) == "*" {
		return perms
	}
	out := make([]PermissionStatement, 0, len(perms))
	for _, p := range perms {
		if scopeAllowsAction(scope, p.Action) {
			out = append(out, p)
		}
	}
	return out
}

// scopeAllowsAction 判断 scope 是否授权某权限动作（任一 scope 命中即可）。
//
// 语义（支持空格/逗号分隔多个 scope，scope 可含 * 通配）：
//   - scope 为空 / "*" → 放行所有；
//   - scope == action → 放行；
//   - 资源级：scope 是 action 的模块前缀（如 iam:account 放行 iam:account:* 全部动作）；
//   - 能力级：scope 末段为通用能力（如 iam:read 放行该模块下 :read 结尾的动作，
//     iam:write 放行该模块下全部动作）；
//   - scope 含 "*" → 按 glob 通配匹配（如 iam:* 放行 iam 模块全部动作）。
func scopeAllowsAction(scope, action string) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "*" {
		return true
	}

	scopes := strings.FieldsFunc(scope, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.Contains(s, "*") {
			if globMatch(s, action) {
				return true
			}
			continue
		}
		if s == action {
			return true
		}
		// 资源级/模块级前缀：scope 是 action 的前缀段
		if strings.HasPrefix(action, s+":") {
			return true
		}
		// 能力级：scope 末段为通用能力词
		if i := strings.LastIndexByte(s, ':'); i > 0 {
			mod, capa := s[:i], s[i+1:]
			if capa == "write" && strings.HasPrefix(action, mod+":") {
				return true // write 覆盖该模块全部动作
			}
			if capa != "" && strings.HasSuffix(action, ":"+capa) {
				return true
			}
		}
	}
	return false
}

// scopeAllowsField 判断 scope 是否授权返回某用户信息字段（OIDC 风格）。
// scope 为空时视为全部授权（兼容现状）。
func scopeAllowsField(scope, name string) bool {
	if strings.TrimSpace(scope) == "" {
		return true
	}
	for _, s := range strings.Fields(scope) {
		if s == name {
			return true
		}
	}
	return false
}
