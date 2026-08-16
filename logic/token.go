package logic

// 访问令牌（JWT）业务声明键与主体类型，全包统一从这里引用。
//
// 重要背景：infra-go 的 AuthMiddleware 会过滤标准声明（sub/exp/iat/nbf/iss/aud/jti）
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
