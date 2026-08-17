// Package svc 服务上下文（ServiceContext）。
//
// main 只负责「加载配置 → 创建 ServiceContext → 启动服务」，
// 数据库 / Redis / 加密 / JWT / 各业务 Logic 与 Handler 的创建、注入
// 与生命周期管理全部集中在这里，避免 main.go 膨胀。
package svc

import (
	"fmt"
	"strings"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/db"
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
//
// 处理器层（*Handler）是 HTTP 接口适配层，仅依赖业务逻辑层，
// 由 route 包在注册路由时统一创建（见 route/handler.go），不在此装配。
type ServiceContext struct {
	Config config.Config

	DB          *gorm.DB
	JWT         *jwt.JWT
	Cipher      *logic.Cipher
	KVStore     store.KVStore
	RedisClient redis.UniversalClient
	// KVStoreCloser KVStore 后台生命周期管理（DBStore 过期键清理 worker；
	// RedisStore 键自过期无需清理，配置 Redis 时置 nil）。
	KVStoreCloser *store.DBStore

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
}

// NewServiceContext 装配并返回服务上下文。
// 任一依赖初始化失败都会返回错误，由调用方决定终止启动。
func NewServiceContext(c config.Config) (*ServiceContext, error) {
	// SQLite：通过 DSN 参数 _busy_timeout 让连接池中每个新建连接都带 busy_timeout。
	// 仅对单个连接 Exec `PRAGMA busy_timeout` 不生效于连接池其他连接，
	// 并发写（业务 + 审计批量落库）时未设置该 PRAGMA 的连接会立即报 "database is locked"。
	if c.DB.Driver == "sqlite" && !strings.Contains(c.DB.Database, ":memory:") && !strings.Contains(c.DB.Database, "_busy_timeout") {
		sep := "?"
		if strings.Contains(c.DB.Database, "?") {
			sep = "&"
		}
		c.DB.Database += sep + "_busy_timeout=5000"
	}

	gormDB, err := orm.New(c.DB)
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	// SQLite：对已建立的连接也设置 busy_timeout（DSN 参数仅对后续新建连接生效）
	if c.DB.Driver == "sqlite" {
		gormDB.Exec("PRAGMA busy_timeout = 5000")
	}

	// 配置 GORM 会话：预编译语句
	gormDB = gormDB.Session(&gorm.Session{PrepareStmt: true})

	if err := db.Migrate(gormDB, c.Migration); err != nil {
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

	// ---- 业务逻辑层装配（注入顺序与原先 main.go 保持一致） ----

	// 应用凭证与密钥管理
	ctx.AppLogic = logic.NewAppLogic(gormDB, cipher)

	// 操作审计（启动落库 worker）
	ctx.AuditLogic = logic.NewAuditLogic(gormDB)

	// 通用键值存储（缓存 / 一次性消费 / 计数等场景的后端抽象）：
	// 默认用数据库表（DBStore，多节点共享）；配置 Redis 后自动切换 RedisStore。
	// DBStore 启动后台过期清理 worker：一次性消费键（OAuth 授权码防重放、重用计数、
	// 访问令牌黑名单等）TTL 到期后行残留，须周期清理，否则表无限膨胀。
	dbStore := store.NewDBStore(gormDB)
	dbStore.StartCleanupLoop(store.DefaultCleanupInterval)
	ctx.KVStore = dbStore
	ctx.KVStoreCloser = dbStore
	if c.Redis.Addr != "" {
		rc, err := redisx.New(c.Redis)
		if err != nil {
			return nil, fmt.Errorf("Redis 初始化失败: %w", err)
		}
		ctx.RedisClient = rc.Client()
		ctx.KVStore = store.NewRedisStore(ctx.RedisClient, c.Redis.KeyPrefix)
		ctx.KVStoreCloser = nil // Redis 键自过期，无需后台清理
		logger.Infof("Redis 存储已启用: %s", c.Redis.Addr)
	} else {
		// 多实例部署语义加固：限流/缓存/审计在无 Redis 时均为单进程实现，
		// 多节点部署会被静默弱化（限流被节点数稀释、审计分散、缓存失效不跨节点）。
		// 显式告警，避免把「Redis 可选」误读为「可随意多开」。
		logger.Warnf("未配置 Redis：限流/缓存/审计为进程内实现，多实例部署将弱化安全语义（限流被节点数稀释、审计分散、缓存失效不跨节点），生产多实例部署请配置 Redis")
	}

	// 权限逻辑（加载账号/角色的权限集合，供中间件与各 handler 使用）
	ctx.PermissionLogic = logic.NewPermissionLogic(gormDB)

	// 认证（登录 / 注册 / 刷新 / 签发 Token / 账号缓存）
	ctx.AuthLogic = logic.NewAuthLogic(gormDB, j, c)
	// 账号信息缓存（认证中间件加载账号用）：复用 KVStore（DBStore/RedisStore），
	// 账号变更（禁用/删除/改密）通过 AccountLogic 注入的失效器主动失效。
	ctx.AuthLogic.SetAccountCache(ctx.KVStore)
	// 访问令牌撤销黑名单（登出时吊销当前 access token）：复用 KVStore
	ctx.AuthLogic.SetBlacklistStore(ctx.KVStore)

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

	// 账号组 / 权限策略 / 授权 / 历史数据清理
	ctx.GroupLogic = logic.NewGroupLogic(gormDB)

	ctx.PolicyLogic = logic.NewPolicyLogic(gormDB)

	ctx.GrantLogic = logic.NewGrantLogic(gormDB)

	// 历史数据清理（管理控制台手动触发，清理 days 天以前的数据）
	ctx.CleanupLogic = logic.NewCleanupLogic(gormDB)

	// 应用列表按数据范围过滤（iam:app:read）
	ctx.AppLogic.SetPermissionLogic(ctx.PermissionLogic)

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
//   - 停止 DBStore 过期键清理 worker（若启用）；
//   - 停止审计落库 worker 并排空队列，避免丢失已入队日志；
//   - 关闭 Redis 连接（若已启用）。
func (s *ServiceContext) Close() {
	if s.KVStoreCloser != nil {
		s.KVStoreCloser.Close()
	}
	if s.AuditLogic != nil {
		s.AuditLogic.Close()
	}
	if s.RedisClient != nil {
		_ = s.RedisClient.Close()
	}
}
