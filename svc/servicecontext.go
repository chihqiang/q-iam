// Package svc 服务上下文（ServiceContext）。
//
// main 只负责「加载配置 → 创建 ServiceContext → 启动服务」，
// 数据库 / Redis / 加密 / JWT / 各业务 Logic 与 Handler 的创建、注入
// 与生命周期管理全部集中在这里，避免 main.go 膨胀。
package svc

import (
	"fmt"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/db"
	"chihqiang/q-iam/handler"
	"chihqiang/q-iam/logic"
	"chihqiang/q-iam/logic/store"

	"github.com/chihqiang/infra-go/jwt"
	"github.com/chihqiang/infra-go/logger"
	"github.com/chihqiang/infra-go/orm"
	"github.com/chihqiang/infra-go/ratelimit"
	"github.com/chihqiang/infra-go/redisx"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ServiceContext 服务上下文：持有应用运行所需的全部依赖。
//
// 字段按层组织，供路由注册（route）与各中间件直接访问：
//   - 基础设施层：DB / JWT / Cipher / KVStore / RedisClient
//   - 业务逻辑层：*Logic（认证、账号、权限等核心逻辑）
//   - 处理器层：*Handler（HTTP 处理函数，供路由绑定）
type ServiceContext struct {
	Config config.Config

	DB          *gorm.DB
	JWT         *jwt.JWT
	Cipher      *logic.Cipher
	KVStore     store.KVStore
	RedisClient redis.UniversalClient

	// 业务逻辑层
	AuthLogic       *logic.AuthLogic
	AccountLogic    *logic.AccountLogic
	GroupLogic      *logic.GroupLogic
	PolicyLogic     *logic.PolicyLogic
	GrantLogic      *logic.GrantLogic
	AppLogic        *logic.AppLogic
	AuditLogic      *logic.AuditLogic
	OAuthLogic      *logic.OAuthLogic
	CleanupLogic    *logic.CleanupLogic
	PermissionLogic *logic.PermissionLogic

	// 处理器层
	AuthHandler    *handler.AuthHandler
	AccountHandler *handler.AccountHandler
	GroupHandler   *handler.GroupHandler
	PolicyHandler  *handler.PolicyHandler
	GrantHandler   *handler.GrantHandler
	AppHandler     *handler.AppHandler
	AuditHandler   *handler.AuditHandler
	OAuthHandler   *handler.OAuthHandler
	CleanupHandler *handler.CleanupHandler
}

// NewServiceContext 装配并返回服务上下文。
// 任一依赖初始化失败都会返回错误，由调用方决定终止启动。
func NewServiceContext(c config.Config) (*ServiceContext, error) {
	gormDB, err := orm.New(c.DB)
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	// SQLite：设置 busy_timeout 避免写冲突时立即报错
	if c.DB.Driver == "sqlite" {
		gormDB.Exec("PRAGMA busy_timeout = 5000")
	}

	// 配置 GORM 会话：预编译语句
	gormDB = gormDB.Session(&gorm.Session{PrepareStmt: true})

	if err := db.Migrate(gormDB); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	j, err := jwt.New(c.JWT)
	if err != nil {
		return nil, fmt.Errorf("JWT 初始化失败: %w", err)
	}

	// 静态数据加密（AppSecret 等）：优先用独立加密密钥，缺省回退 JWT Secret（兼容旧行为）。
	cipherKey := c.Security.Cipher.Key
	if cipherKey == "" {
		// 兼容旧行为：回退用 JWT Secret 派生加密密钥。
		// 注意：与签名密钥混用不符合密码学实践——轮换 JWT_SECRET 会导致存量
		// AppSecret 无法解密（除非同时配置 security.cipher.previous_key）。
		// 生产建议单独配置 security.cipher.key 并保持稳定。
		logger.Warnf("security.cipher.key 未配置，AppSecret 加密密钥回退使用 JWT Secret（生产建议配置独立加密密钥）")
		cipherKey = c.JWT.Secret
	}
	cipher, err := logic.NewCipher(cipherKey, c.Security.Cipher.PreviousKey)
	if err != nil {
		return nil, fmt.Errorf("加密器初始化失败: %w", err)
	}

	ctx := &ServiceContext{
		Config: c,
		DB:     gormDB,
		JWT:    j,
		Cipher: cipher,
	}

	// ---- 业务逻辑层与处理器层装配（注入顺序与原先 main.go 保持一致） ----

	// 应用凭证与密钥管理
	ctx.AppLogic = logic.NewAppLogic(gormDB, cipher)
	ctx.AppHandler = handler.NewAppHandler(ctx.AppLogic)

	// 操作审计（启动落库 worker）
	ctx.AuditLogic = logic.NewAuditLogic(gormDB)
	ctx.AuditHandler = handler.NewAuditHandler(ctx.AuditLogic)

	// 通用键值存储（缓存 / 一次性消费 / 计数等场景的后端抽象）：
	// 默认用数据库表（DBStore，多节点共享）；配置 Redis 后自动切换 RedisStore。
	ctx.KVStore = store.NewDBStore(gormDB)
	if c.Redis.Addr != "" {
		rc, err := redisx.New(c.Redis)
		if err != nil {
			return nil, fmt.Errorf("Redis 初始化失败: %w", err)
		}
		ctx.RedisClient = rc.Client()
		ctx.KVStore = store.NewRedisStore(ctx.RedisClient, c.Redis.KeyPrefix)
		logger.Infof("Redis 存储已启用: %s", c.Redis.Addr)
	}

	// 权限逻辑（加载账号/角色的权限集合，供中间件与各 handler 使用）
	ctx.PermissionLogic = logic.NewPermissionLogic(gormDB)

	// 认证（登录 / 注册 / 刷新 / 签发 Token / 账号缓存）
	ctx.AuthLogic = logic.NewAuthLogic(gormDB, j, c)
	// 账号信息缓存（认证中间件加载账号用）：复用 KVStore（DBStore/RedisStore），
	// 账号变更（禁用/删除/改密）通过 AccountLogic 注入的失效器主动失效。
	ctx.AuthLogic.SetAccountCache(ctx.KVStore)
	ctx.AuthHandler = handler.NewAuthHandler(ctx.AuthLogic)
	// /auth/me 返回当前账号权限，供前端按权限过滤菜单
	ctx.AuthHandler.SetPermissionLogic(ctx.PermissionLogic)

	// 配置 Redis 后，登录/注册/刷新全局限流切换到 Redis 分布式实现（多节点共享限流）
	if ctx.RedisClient != nil {
		ctx.AuthLogic.SetLoginLimiter(ratelimit.NewRedisTokenBucket(ctx.RedisClient, "q-iam:rl:login", 2, 10))
		ctx.AuthLogic.SetRegisterLimiter(ratelimit.NewRedisTokenBucket(ctx.RedisClient, "q-iam:rl:register", 0.1, 5))
		ctx.AuthLogic.SetRefreshLimiter(ratelimit.NewRedisTokenBucket(ctx.RedisClient, "q-iam:rl:refresh", 5, 20))
	}

	// OAuth 授权（authorization_code）：应用换取 Token 的签发逻辑归口 OAuthLogic，
	// 应用凭证校验注入 AppLogic；AuthLogic.Token 仅作为入口转发。
	// 授权码一次性消费默认用 DBStore，配置 Redis 后由 KVStore 统一切换。
	ctx.OAuthLogic = logic.NewOAuthLogic(gormDB, j)
	ctx.OAuthLogic.SetConsumedStore(ctx.KVStore)
	ctx.OAuthLogic.SetAppLogic(ctx.AppLogic)
	ctx.OAuthHandler = handler.NewOAuthHandler(ctx.OAuthLogic)
	ctx.AuthLogic.SetOAuthLogic(ctx.OAuthLogic)

	// 配置 Redis 后，应用换取 Token 限流切换到 Redis 分布式实现
	if ctx.RedisClient != nil {
		ctx.OAuthLogic.SetTokenLimiter(ratelimit.NewRedisTokenBucket(ctx.RedisClient, "q-iam:rl:token", 5, 20))
	}

	// 账号管理（密码策略 + 账号变更后失效认证缓存）
	passwordValidator := logic.NewPasswordValidator(c.Security.PasswordPolicy)
	ctx.AccountLogic = logic.NewAccountLogic(gormDB, passwordValidator)
	// 账号变更（禁用/删除/改密/重置密码）后失效认证中间件的账号缓存（AuthLogic 实现）
	ctx.AccountLogic.SetCacheInvalidator(ctx.AuthLogic)
	ctx.AccountHandler = handler.NewAccountHandler(ctx.AccountLogic)

	// 账号组 / 权限策略 / 授权 / 历史数据清理
	ctx.GroupLogic = logic.NewGroupLogic(gormDB)
	ctx.GroupHandler = handler.NewGroupHandler(ctx.GroupLogic)

	ctx.PolicyLogic = logic.NewPolicyLogic(gormDB)
	ctx.PolicyHandler = handler.NewPolicyHandler(ctx.PolicyLogic)

	ctx.GrantLogic = logic.NewGrantLogic(gormDB)
	ctx.GrantHandler = handler.NewGrantHandler(ctx.GrantLogic)

	// 历史数据清理（管理控制台手动触发，清理 days 天以前的数据）
	ctx.CleanupLogic = logic.NewCleanupLogic(gormDB)
	ctx.CleanupHandler = handler.NewCleanupHandler(ctx.CleanupLogic)

	// OAuth UserInfo 需要加载用户权限
	ctx.OAuthLogic.SetPermissionLogic(ctx.PermissionLogic)

	// 权限集缓存：始终启用（默认 DBStore，配置 Redis 后为 RedisStore）。
	// 权限来源变更（授权/组成员/账号组关联/策略规则）后主动失效缓存。
	ctx.PermissionLogic.SetStore(ctx.KVStore)
	ctx.GrantLogic.SetPermissionLogic(ctx.PermissionLogic)
	ctx.GroupLogic.SetPermissionLogic(ctx.PermissionLogic)
	ctx.AccountLogic.SetPermissionLogic(ctx.PermissionLogic)
	ctx.PolicyLogic.SetPermissionLogic(ctx.PermissionLogic)

	return ctx, nil
}

// Close 释放服务运行期资源（服务退出时调用）：
//   - 停止审计落库 worker 并排空队列，避免丢失已入队日志；
//   - 关闭 Redis 连接（若已启用）。
func (s *ServiceContext) Close() {
	if s.AuditLogic != nil {
		s.AuditLogic.Close()
	}
	if s.RedisClient != nil {
		_ = s.RedisClient.Close()
	}
}
