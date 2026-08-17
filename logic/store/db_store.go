package store

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DBStore 基于数据库表的 KVStore 实现。
// 无 Redis 时的默认后端：多节点同样共享（走数据库），适合低频到中频读写。
// 原子性说明：
//   - SetNX 依赖主键唯一 + INSERT ON CONFLICT DO NOTHING，跨驱动原子；
//   - Incr 用事务 + 行级读改写 + 唯一索引兜底（跨驱动无原生原子自增，
//     用于低频计数足够；高频计数建议用 RedisStore）。
type DBStore struct {
	db   *gorm.DB
	stop chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

// NewDBStore 创建数据库存储。
func NewDBStore(db *gorm.DB) *DBStore {
	return &DBStore{db: db, stop: make(chan struct{})}
}

// DefaultCleanupInterval 过期键后台清理周期。
// 一次性消费键（OAuth 授权码防重放、重用计数、访问令牌黑名单等）TTL 到期后
// 行记录仍残留表中，须后台周期清理，否则 q_iam_kv_store 会无限膨胀。
const DefaultCleanupInterval = time.Minute

// StartCleanupLoop 启动过期键后台清理 worker（幂等，重复调用不叠加）。
// 清理范围：仅带过期时间（expires_at 非空）且已过期的行；不过期的键永不删除。
func (s *DBStore) StartCleanupLoop(interval time.Duration) {
	s.once.Do(func() {
		s.wg.Add(1)
		go s.cleanupLoop(interval)
	})
}

// Close 停止后台清理 worker（幂等）。
func (s *DBStore) Close() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	s.wg.Wait()
}

// cleanupLoop 周期执行过期键清理。
func (s *DBStore) cleanupLoop(interval time.Duration) {
	defer s.wg.Done()
	if interval <= 0 {
		interval = DefaultCleanupInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.cleanupExpiredAll()
		}
	}
}

// cleanupExpiredAll 批量删除全部已过期的键（后台周期清理）。
func (s *DBStore) cleanupExpiredAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := s.db.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
		Delete(&model.KeyStoreItem{})
	if res.Error != nil {
		logger.WarnCtx(ctx, "db store cleanup expired failed", logger.Err(res.Error))
		return
	}
	if res.RowsAffected > 0 {
		logger.InfoCtx(ctx, "db store cleaned expired keys", logger.Int64("count", res.RowsAffected))
	}
}

// nowPtr 返回当前时间指针（用于写入过期时间）。
func nowPtr(d time.Duration) *time.Time {
	if d <= 0 {
		return nil
	}
	t := time.Now().Add(d)
	return &t
}

// Get 读取键值；键不存在或已过期返回 ("", nil)。
func (s *DBStore) Get(ctx context.Context, key string) (string, error) {
	var item model.KeyStoreItem
	err := s.db.WithContext(ctx).First(&item, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	// 过期校验（Get 是高频读，这里判断即可，不需要每次写库清理）
	if item.ExpiresAt != nil && item.ExpiresAt.Before(time.Now()) {
		return "", nil
	}
	return item.Value, nil
}

// Set 写入键值并设置 TTL（upsert）。
func (s *DBStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	item := model.KeyStoreItem{
		Key:       key,
		Value:     value,
		ExpiresAt: nowPtr(ttl),
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "expires_at", "updated_at"}),
	}).Create(&item).Error
}

// SetNX 原子占位：键不存在（且未过期）则写入并返回 true；已存在返回 false。
// 依赖主键唯一 + ON CONFLICT DO NOTHING，跨驱动原子。
func (s *DBStore) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	// 先清理同键的过期占位（避免永久占用导致 SetNX 永远失败）
	s.cleanupExpired(ctx, key)

	item := model.KeyStoreItem{
		Key:       key,
		Value:     value,
		ExpiresAt: nowPtr(ttl),
	}
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// Del 删除一个或多个键。
func (s *DBStore) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Where("key IN ?", keys).Delete(&model.KeyStoreItem{}).Error
}

// Incr 原子自增并返回自增后的值（事务 + 读改写 + 唯一索引兜底）。
// 首次创建（值为 1）时应用 TTL。
func (s *DBStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	var val int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清理过期旧值，保证"过期即不存在"
		s.cleanupExpiredTx(tx, key)

		// 读取当前值
		var item model.KeyStoreItem
		err := tx.First(&item, "key = ?", key).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 不存在：插入 1（主键唯一兜底并发）
			one := "1"
			item = model.KeyStoreItem{Key: key, Value: one, ExpiresAt: nowPtr(ttl)}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 1 {
				val = 1
				return nil
			}
			// 并发下已被插入，继续走读改写
			if err := tx.First(&item, "key = ?", key).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		cur, err := strconv.ParseInt(item.Value, 10, 64)
		if err != nil {
			cur = 0
		}
		val = cur + 1
		return tx.Model(&model.KeyStoreItem{}).Where("key = ?", key).
			Update("value", strconv.FormatInt(val, 10)).Error
	})
	if err != nil {
		return 0, err
	}
	return val, nil
}

// cleanupExpired 删除指定键的过期记录（SetNX 前置清理）。
func (s *DBStore) cleanupExpired(ctx context.Context, key string) {
	s.db.WithContext(ctx).Where("key = ? AND expires_at IS NOT NULL AND expires_at <= ?", key, time.Now()).
		Delete(&model.KeyStoreItem{})
}

// cleanupExpiredTx 事务内删除指定键的过期记录。
func (s *DBStore) cleanupExpiredTx(tx *gorm.DB, key string) {
	tx.Where("key = ? AND expires_at IS NOT NULL AND expires_at <= ?", key, time.Now()).
		Delete(&model.KeyStoreItem{})
}
