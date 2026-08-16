// Package store 提供通用的键值存储抽象。
//
// 为什么叫 store 而不叫 cache：这里的抽象不止「缓存」，还覆盖了
//   - 缓存：权限集等高频读取结果的短期缓存（Get/Set/Del + TTL）；
//   - 一次性消费：OAuth 授权码防重放（SetNX 原子占位）；
//   - 计数：原子自增（Incr，可支撑限流/计数器）。
//
// 通过接口解耦后端实现，业务侧依赖注入即可：
//   - store.RedisStore：基于 Redis（多节点共享、高并发）；
//   - store.DBStore：基于数据库表（无 Redis 时默认，多节点同样共享）；
//   - 自定义：实现 KVStore 接口即可接入，无需改动业务代码。
package store

import (
	"context"
	"time"
)

// KVStore 通用键值存储接口。
//
// 约定：
//   - Get 键不存在时返回 ("", nil)，不视为错误；
//   - ttl <= 0 表示不过期；
//   - 各后端实现需保证 SetNX / Incr 的原子性（并发安全）。
type KVStore interface {
	// Get 读取键值。键不存在返回空串且 error 为 nil。
	Get(ctx context.Context, key string) (string, error)

	// Set 写入键值并设置 TTL（ttl <= 0 表示不过期）。
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// SetNX 原子占位：键不存在则写入并返回 true；键已存在返回 false，不覆盖。
	// 用于一次性消费 / 去重（如 OAuth 授权码防重放）。
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)

	// Del 删除一个或多个键。删除不存在的键不报错。
	Del(ctx context.Context, keys ...string) error

	// Incr 原子自增并返回自增后的值。键不存在时按 0 起增。
	// 首次创建（返回值为 1）时应用 ttl；后续自增不重置过期时间（固定窗口语义）。
	// ttl <= 0 表示不过期。
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}
