package config

import (
	"time"

	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/jwt"
	"github.com/chihqiang/infra-go/logger"
	"github.com/chihqiang/infra-go/orm"
	"github.com/chihqiang/infra-go/redisx"
)

// Config 全局配置。
type Config struct {
	App       App                `json:"app"`
	Server    httpx.ServerConfig `json:"server"`
	DB        orm.Config         `json:"db"`
	Migration MigrationConfig    `json:"migration"`
	JWT       jwt.Config         `json:"jwt"`
	Logger    logger.Config      `json:"logger"`
	Security  SecurityConfig     `json:"security"`
	CORS      CORSConfig         `json:"cors"`
	Pprof     PprofConfig        `json:"pprof"`
	Redis     redisx.Config      `json:"redis,optional"`
}

// App 应用基础信息。
type App struct {
	Name string `json:"name,default=q-iam"`
	// Version 应用版本号。
	Version string `json:"version,default=0.0.1"`
}

// CORSConfig 跨域配置。
type CORSConfig struct {
	AllowOrigins []string `json:"allow_origins,optional"`
}

// SecurityConfig 全局安全配置。
// 统管密码策略、登录安全、会话、静态加密、代理信任等安全相关配置。
type SecurityConfig struct {
	// PasswordPolicy 密码策略。
	PasswordPolicy PasswordPolicyConfig `json:"password_policy"`
	// Login 登录安全配置。
	Login LoginSecurityConfig `json:"login"`
	// Register 注册配置。
	Register RegisterSecurityConfig `json:"register"`
	// Cipher 静态数据加密配置（AppSecret 等敏感字段）。
	Cipher CipherConfig `json:"cipher,optional"`
	// TrustProxy 是否信任反向代理（X-Forwarded-For / X-Real-IP）。
	// 默认 false：客户端 IP 取 TCP 直连地址，避免被伪造；
	// 部署在可信反代之后时置 true，否则审计/签发来源 IP 将是反代地址。
	TrustProxy bool `json:"trust_proxy,default=false"`
}

// CipherConfig 静态数据加密配置。
// 支持密钥版本化：加密始终用当前 Key（密文 enc:v2），解密兼容存量 enc:v1。
// 轮换流程：把旧密钥填入 PreviousKey，新密钥填入 Key 后重启即可平滑过渡，
// 存量密文用 PreviousKey 解，新密文用 Key 加密。
type CipherConfig struct {
	// Key 当前加密密钥。留空时回退用 JWT Secret 派生（兼容旧行为）。
	Key string `json:"key,optional"`
	// PreviousKey 上一版本密钥。仅用于解密存量 enc:v1 密文，轮换时配置。
	PreviousKey string `json:"previous_key,optional"`
}

// RegisterSecurityConfig 注册配置。
type RegisterSecurityConfig struct {
	// Enabled 是否开放注册，默认 true。
	Enabled bool `json:"enabled,default=true"`
}

// LoginSecurityConfig 登录安全配置。
// 用于登录失败锁定与登录环节的限流。
type LoginSecurityConfig struct {
	// MaxFailCount 连续登录失败锁定阈值，默认 5，<=0 表示不锁定。
	MaxFailCount int `json:"max_fail_count,default=5"`
	// LockDuration 锁定时长，默认 15 分钟。
	LockDuration time.Duration `json:"lock_duration,default=15m"`
}

// PasswordPolicyConfig 密码策略配置。
// 用于账号创建 / 修改密码时的强度校验。
type PasswordPolicyConfig struct {
	// MinLength 最小长度，默认 8。
	MinLength int `json:"min_length,default=8"`
	// MaxLength 最大长度，默认 64，<=0 表示不限制。
	MaxLength int `json:"max_length,default=64"`
	// RequireUppercase 是否要求至少一个大写字母，默认 false。
	RequireUppercase bool `json:"require_uppercase,default=false"`
	// RequireLowercase 是否要求至少一个小写字母，默认 true。
	RequireLowercase bool `json:"require_lowercase,default=true"`
	// RequireDigit 是否要求至少一个数字，默认 true。
	RequireDigit bool `json:"require_digit,default=true"`
	// RequireSpecial 是否要求至少一个特殊字符，默认 false。
	RequireSpecial bool `json:"require_special,default=false"`
	// MinUniqueChars 最少不同字符数，默认 0（不限制）。
	MinUniqueChars int `json:"min_unique_chars,default=0"`
	// ForbidAccountName 是否禁止密码包含账号名，默认 true。
	ForbidAccountName bool `json:"forbid_account_name,default=true"`
	// HistoryCount 密码历史数量（防止重复使用旧密码），<=0 表示不记录。
	HistoryCount int `json:"history_count,default=0"`
	// ExpireDays 密码有效期（天），到期强制修改，<=0 表示不限期。
	ExpireDays int `json:"expire_days,default=0"`
}

// PprofConfig pprof 性能分析配置。
type PprofConfig struct {
	Enabled bool `json:"enabled,default=false"`
}

// MigrationConfig 数据库迁移配置。
type MigrationConfig struct {
	// AutoMigrate 是否自动迁移表结构（建表/加列等），默认 true。
	// 关闭时需自行保证表结构已存在（如由外部迁移工具管理）。
	AutoMigrate bool `json:"auto_migrate,default=true"`
	// SeedData 是否初始化基础数据（内置 admin 账号、系统内置策略等），默认 true。
	SeedData bool `json:"seed_data,default=true"`
}
