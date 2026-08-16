package logic

import (
	"context"
	"time"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
)

// DefaultCleanupDays 默认清理天数：清理 30 天以前的历史数据。
const DefaultCleanupDays = 30

// cleanupBatchSize 每批清理条数，分批删除避免长事务锁表。
const cleanupBatchSize = 5000

// CleanupLogic 历史数据清理逻辑（管理控制台手动触发）。
type CleanupLogic struct {
	db *gorm.DB
}

// NewCleanupLogic 创建历史数据清理逻辑。
func NewCleanupLogic(db *gorm.DB) *CleanupLogic {
	return &CleanupLogic{db: db}
}

// CleanupResult 清理结果。
type CleanupResult struct {
	// AuditLogs 清理的审计日志条数。
	AuditLogs int64 `json:"audit_logs"`
	// RefreshTokens 清理的刷新令牌条数（仅已过期）。
	RefreshTokens int64 `json:"refresh_tokens"`
}

// CleanupHistory 清理历史数据。
//
// days 为保留天数：清理 days 天以前的数据；days <= 0 时用默认值 30。
//
// 清理范围：
//   - 审计日志：created_at 早于 (now - days) 的记录；
//   - 刷新令牌：expires_at 早于 now 的**已过期**记录。
//
// 刷新令牌只清已过期的（绝不动仍有效的令牌，避免误伤活跃会话），
// 且「已吊销但未过期」的记录必须保留——重用检测依赖其存在。
func (s *CleanupLogic) CleanupHistory(ctx context.Context, days int) (*CleanupResult, error) {
	if days <= 0 {
		days = DefaultCleanupDays
	}
	result := &CleanupResult{}
	now := time.Now()

	// 1. 审计日志：清理 days 天以前的记录
	auditCutoff := now.AddDate(0, 0, -days)
	n, err := s.deleteBatch(ctx, &model.AuditLog{}, "created_at < ?", auditCutoff)
	if err != nil {
		return nil, err
	}
	result.AuditLogs = n

	// 2. 刷新令牌：清理已过期（expires_at < now）的记录
	n, err = s.deleteBatch(ctx, &model.RefreshToken{}, "expires_at < ?", now)
	if err != nil {
		return nil, err
	}
	result.RefreshTokens = n

	logger.InfoCtx(ctx, "cleanup history done",
		logger.Int("days", days),
		logger.Int64("audit_logs", result.AuditLogs),
		logger.Int64("refresh_tokens", result.RefreshTokens))
	return result, nil
}

// deleteBatch 按条件分批删除，返回删除总条数。
func (s *CleanupLogic) deleteBatch(ctx context.Context, model any, query string, args ...any) (int64, error) {
	var total int64
	for {
		res := s.db.WithContext(ctx).
			Where(query, args...).
			Limit(cleanupBatchSize).
			Delete(model)
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if res.RowsAffected < cleanupBatchSize {
			break
		}
	}
	return total, nil
}
