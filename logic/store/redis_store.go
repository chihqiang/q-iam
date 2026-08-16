package store

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore 基于 Redis 的 KVStore 实现。
// 适用于多节点水平扩展、高并发场景；通过 SetNX/Incr 的 Redis 原子命令保证并发安全。
//
// 注意：SetNX/Incr 使用了 Lua 脚本 / SetNX 原子命令，保证跨请求原子性。
type RedisStore struct {
	client redis.UniversalClient
	prefix string
}

// NewRedisStore 创建 Redis 存储。
// rc 为已初始化的底层 redis 客户端（支持 *redis.Client / ClusterClient / Ring）；
// prefix 为键名前缀（与 redisx 配置的 key_prefix 一致，保证与历史 redisx 直接操作的数据可共享）。
func NewRedisStore(rc redis.UniversalClient, prefix string) *RedisStore {
	return &RedisStore{client: rc, prefix: prefix}
}

// wrap 为键添加前缀。
func (s *RedisStore) wrap(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + ":" + key
}

// Get 读取键值；键不存在返回 ("", nil)。
func (s *RedisStore) Get(ctx context.Context, key string) (string, error) {
	val, err := s.client.Get(ctx, s.wrap(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

// Set 写入键值并设置 TTL。
func (s *RedisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, s.wrap(key), value, ttl).Err()
}

// SetNX 原子占位（SET NX + EXPIRE 原子执行），用于一次性消费。
func (s *RedisStore) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, s.wrap(key), value, ttl).Result()
}

// Del 删除一个或多个键。
func (s *RedisStore) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	wrapped := make([]string, len(keys))
	for i, k := range keys {
		wrapped[i] = s.wrap(k)
	}
	return s.client.Del(ctx, wrapped...).Err()
}

// incrScript INCR + 首次设 TTL 的原子脚本（固定窗口语义）。
// KEYS[1]=键；ARGV[1]=TTL 秒数（<=0 时不设过期）。
var incrScript = redis.NewScript(`
local v = redis.call('INCR', KEYS[1])
local ttl = tonumber(ARGV[1])
if v == 1 and ttl and ttl > 0 then
  redis.call('EXPIRE', KEYS[1], ttl)
end
return v
`)

// Incr 原子自增；首次创建（值为 1）时应用 TTL，后续不重置过期时间。
func (s *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	seconds := int64(ttl / time.Second)
	return incrScript.Run(ctx, s.client, []string{s.wrap(key)}, seconds).Int64()
}
