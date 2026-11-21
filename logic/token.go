package logic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"

	"github.com/chihqiang/ten-go/rest/token"

	"chihqiang/q-iam/config"
)

// 访问令牌（JWT）业务声明键与主体类型，全包统一从这里引用。
//
// 重要背景：认证中间件会过滤标准声明（sub/exp/iat/nbf/iss/aud/jti）
// 和 token_type，只把业务声明注入 context。因此下游（UserInfo Endpoint、权限中间件、
// 审计）判断令牌主体类型时不能依赖 sub，必须使用下面的业务字段。
const (
	// ClaimTokenSubjectType 访问令牌主体类型键（取值见 TokenSubjectType*）。
	ClaimTokenSubjectType = "token_subject_type"
	// ClaimTokenAppID 访问令牌中的应用 ID（client_credentials 令牌）。
	// 与 OAuth 授权码内的 claimOAuthAppID（键 oauth_app_id）是不同声明，勿混淆。
	ClaimTokenAppID = "app_id"
)

// 访问令牌主体类型（ClaimTokenSubjectType 的取值，也用作 sub 主体标识前缀）。
const (
	// TokenSubjectTypeUser 主体为账号（登录 / authorization_code 模式）。
	TokenSubjectTypeUser = "user"
	// TokenSubjectTypeApp 主体为应用（client_credentials 模式）。
	TokenSubjectTypeApp = "app"
)

// 标准声明、令牌类型与常用业务声明键。
const (
	ClaimKeyIssuer         = "iss"
	ClaimKeySubject        = "sub"
	ClaimKeyAudience       = "aud"
	ClaimKeyExpirationTime = "exp"
	ClaimKeyNotBefore      = "nbf"
	ClaimKeyIssuedAt       = "iat"
	ClaimKeyJWTID          = "jti"
	ClaimKeyTokenType      = "token_type"

	ClaimKeyUserID   = "user_id"
	ClaimKeyUsername = "username"
	ClaimKeyRole     = "role"
	ClaimKeyScopes   = "scopes"

	// TokenTypeAccess 访问令牌。
	TokenTypeAccess = "access"
	// TokenTypeRefresh 刷新令牌。
	TokenTypeRefresh = "refresh"

	// minSecretLength 签名密钥最小长度（沿用旧版 jwt 包的校验）。
	minSecretLength = 8
)

// 令牌校验错误。
var (
	ErrInvalidToken    = errors.New("token: invalid token")
	ErrExpiredToken    = errors.New("token: token is expired")
	ErrNotRefreshToken = errors.New("token: token is not a refresh token")
)

// Claims 是 JWT 载荷（ten-go rest/token 的 MapClaims）。
type Claims = jwtlib.MapClaims

// TokenPair 访问令牌 + 刷新令牌。
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

// TokenManager 基于 ten-go rest/token 的令牌签发与校验。
//
// ten-go 的 Parser 只负责 HS256 签名/验签与 exp 校验，业务声明（user_id、
// scopes、jti、token_type 等）由这里拼装；access 与 refresh 使用由主密钥
// 派生的不同签名密钥，因此刷新令牌不能当作访问令牌使用。
type TokenManager struct {
	access        *token.Parser
	refresh       *token.Parser
	issuer        string
	audience      []string
	accessExpire  time.Duration
	refreshExpire time.Duration
}

// NewTokenManager 按配置创建令牌管理器。
func NewTokenManager(cfg config.JWTConfig) (*TokenManager, error) {
	if len(cfg.Secret) < minSecretLength {
		return nil, fmt.Errorf("token: secret must be at least %d characters", minSecretLength)
	}
	if cfg.Algorithm != "" && cfg.Algorithm != "HS256" {
		return nil, fmt.Errorf("token: ten-go rest/token only supports HS256, got %q", cfg.Algorithm)
	}
	accessExpire := cfg.AccessTokenExpire
	if accessExpire <= 0 {
		accessExpire = 2 * time.Hour
	}
	refreshExpire := cfg.RefreshTokenExpire
	if refreshExpire <= 0 {
		refreshExpire = 168 * time.Hour
	}
	return &TokenManager{
		access:        token.NewParser(token.Config{Secret: deriveSecret(cfg.Secret, TokenTypeAccess)}),
		refresh:       token.NewParser(token.Config{Secret: deriveSecret(cfg.Secret, TokenTypeRefresh)}),
		issuer:        cfg.Issuer,
		audience:      cfg.Audience,
		accessExpire:  accessExpire,
		refreshExpire: refreshExpire,
	}, nil
}

// AccessExpire 返回访问令牌有效期。
func (m *TokenManager) AccessExpire() time.Duration { return m.accessExpire }

// RefreshExpire 返回刷新令牌有效期。
func (m *TokenManager) RefreshExpire() time.Duration { return m.refreshExpire }

// GenerateToken 按指定有效期签发一个令牌（OAuth 授权码等场景）。
func (m *TokenManager) GenerateToken(claims Claims, expire time.Duration) (string, error) {
	return m.access.Generate(m.buildClaims(claims, expire, ""))
}

// GenerateAccessToken 签发访问令牌。
func (m *TokenManager) GenerateAccessToken(claims Claims) (string, error) {
	return m.access.Generate(m.buildClaims(claims, m.accessExpire, TokenTypeAccess))
}

// GenerateRefreshToken 签发刷新令牌。
func (m *TokenManager) GenerateRefreshToken(claims Claims) (string, error) {
	return m.refresh.Generate(m.buildClaims(claims, m.refreshExpire, TokenTypeRefresh))
}

// ParseToken 校验令牌签名与有效期，不限制令牌类型。
func (m *TokenManager) ParseToken(raw string) (Claims, error) {
	return m.parse(m.access, raw)
}

// ParseAccessToken 校验访问令牌。
func (m *TokenManager) ParseAccessToken(raw string) (Claims, error) {
	claims, err := m.parse(m.access, raw)
	if err != nil {
		return nil, err
	}
	if typ, _ := claims[ClaimKeyTokenType].(string); typ != TokenTypeAccess {
		return nil, fmt.Errorf("%w: expected access token, got %q", ErrInvalidToken, typ)
	}
	return claims, nil
}

// ParseRefreshToken 校验刷新令牌。
func (m *TokenManager) ParseRefreshToken(raw string) (Claims, error) {
	claims, err := m.parse(m.refresh, raw)
	if err != nil {
		return nil, err
	}
	if typ, _ := claims[ClaimKeyTokenType].(string); typ != TokenTypeRefresh {
		return nil, ErrNotRefreshToken
	}
	return claims, nil
}

// buildClaims 复制业务声明并补齐标准声明（sub/iat/exp/iss/aud/token_type）。
func (m *TokenManager) buildClaims(src Claims, expire time.Duration, tokenType string) Claims {
	now := time.Now()
	claims := make(Claims, len(src)+6)
	for k, v := range src {
		claims[k] = v
	}
	if sub, _ := claims[ClaimKeySubject].(string); sub == "" {
		claims[ClaimKeySubject] = subjectOf(claims)
	}
	claims[ClaimKeyIssuedAt] = now.Unix()
	claims[ClaimKeyExpirationTime] = now.Add(expire).Unix()
	if tokenType != "" {
		claims[ClaimKeyTokenType] = tokenType
	}
	if m.issuer != "" {
		claims[ClaimKeyIssuer] = m.issuer
	}
	if len(m.audience) > 0 {
		claims[ClaimKeyAudience] = m.audience
	}
	return claims
}

// parse 校验签名/有效期，并补做 ten-go Parser 未覆盖的 iss/aud 校验。
func (m *TokenManager) parse(parser *token.Parser, raw string) (Claims, error) {
	claims, err := parser.Parse(raw)
	if err != nil {
		if errors.Is(err, jwtlib.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if m.issuer != "" {
		if iss, _ := claims[ClaimKeyIssuer].(string); iss != m.issuer {
			return nil, ErrInvalidToken
		}
	}
	if len(m.audience) > 0 && !hasAudience(claims[ClaimKeyAudience], m.audience) {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// subjectOf 在调用方未显式设置 sub 时，从业务声明派生主体标识。
func subjectOf(claims Claims) string {
	if subjectType, _ := claims[ClaimTokenSubjectType].(string); subjectType == TokenSubjectTypeApp {
		if appID, _ := claims[ClaimTokenAppID].(string); appID != "" {
			return TokenSubjectTypeApp + ":" + appID
		}
	}
	switch v := claims[ClaimKeyUserID].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	}
	if name, _ := claims[ClaimKeyUsername].(string); name != "" {
		return name
	}
	return "unknown"
}

func hasAudience(claim any, want []string) bool {
	for _, target := range want {
		switch aud := claim.(type) {
		case string:
			if aud == target {
				return true
			}
		case []string:
			for _, v := range aud {
				if v == target {
					return true
				}
			}
		case []any:
			for _, item := range aud {
				if v, _ := item.(string); v == target {
					return true
				}
			}
		}
	}
	return false
}

func deriveSecret(secret, scope string) string {
	sum := sha256.Sum256([]byte(secret + ":" + scope))
	return hex.EncodeToString(sum[:])
}

// Claims 上下文：认证中间件把过滤后的业务声明写入请求上下文，
// 业务 Handler / Logic 沿用原来的 map 读取方式。
type claimsContextKey struct{}

// WithClaims 把令牌业务声明写入上下文。
func WithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, claimsContextKey{}, claims)
}

// ClaimsFromContext 读取认证中间件写入的令牌业务声明。
func ClaimsFromContext(ctx context.Context) Claims {
	claims, _ := ctx.Value(claimsContextKey{}).(Claims)
	return claims
}
