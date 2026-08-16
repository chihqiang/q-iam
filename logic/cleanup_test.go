package logic

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"chihqiang/q-iam/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestCleanupDB 构造独立命名内存库（含审计日志与刷新令牌表）。
func newTestCleanupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}, &model.RefreshToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestCleanupHistory_DefaultDays 验证默认 30 天清理语义：
// 审计日志 30 天以前删除、30 天内保留；
// 刷新令牌仅清理已过期（expires_at < now），未过期的（含已吊销但未过期）保留。
func TestCleanupHistory_DefaultDays(t *testing.T) {
	db := newTestCleanupDB(t)
	svc := NewCleanupLogic(db)
	ctx := context.Background()

	now := time.Now()

	// 审计日志
	oldAudit := now.AddDate(0, 0, -60)   // 60 天前 → 应删
	recentAudit := now.AddDate(0, 0, -5) // 5 天前 → 保留
	if err := db.Create(&model.AuditLog{Module: "auth", Action: "login", CreatedAt: oldAudit}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Create(&model.AuditLog{Module: "auth", Action: "login", CreatedAt: recentAudit}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 刷新令牌
	expiredRT := now.Add(-24 * time.Hour)   // 已过期 → 应删
	validRT := now.Add(24 * time.Hour)      // 未过期 → 保留
	revokedValid := now.Add(24 * time.Hour) // 已吊销但未过期 → 保留（重用检测依赖）
	revokedAt := now.Add(-1 * time.Hour)
	if err := db.Create(&model.RefreshToken{TokenID: "expired", AccountID: 1, ExpiresAt: expiredRT}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Create(&model.RefreshToken{TokenID: "valid", AccountID: 1, ExpiresAt: validRT}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Create(&model.RefreshToken{TokenID: "revoked-valid", AccountID: 1, ExpiresAt: revokedValid, RevokedAt: &revokedAt, RevokeReason: "rotated"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 默认（days=0 → 30）
	result, err := svc.CleanupHistory(ctx, 0)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.AuditLogs != 1 {
		t.Fatalf("expected 1 audit log deleted, got %d", result.AuditLogs)
	}
	if result.RefreshTokens != 1 {
		t.Fatalf("expected 1 refresh token deleted, got %d", result.RefreshTokens)
	}

	// 保留项校验
	var auditCount int64
	db.Model(&model.AuditLog{}).Count(&auditCount)
	if auditCount != 1 {
		t.Fatalf("expected 1 audit log remaining, got %d", auditCount)
	}
	var rtCount int64
	db.Model(&model.RefreshToken{}).Count(&rtCount)
	if rtCount != 2 {
		t.Fatalf("expected 2 refresh tokens remaining (valid + revoked-valid), got %d", rtCount)
	}
}

// TestCleanupHistory_CustomDays 验证自定义保留天数。
func TestCleanupHistory_CustomDays(t *testing.T) {
	db := newTestCleanupDB(t)
	svc := NewCleanupLogic(db)
	ctx := context.Background()

	now := time.Now()
	// 10 天前 / 20 天前
	if err := db.Create(&model.AuditLog{Module: "auth", Action: "login", CreatedAt: now.AddDate(0, 0, -10)}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Create(&model.AuditLog{Module: "auth", Action: "login", CreatedAt: now.AddDate(0, 0, -20)}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 保留 15 天：10 天前保留、20 天前删除
	result, err := svc.CleanupHistory(ctx, 15)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.AuditLogs != 1 {
		t.Fatalf("expected 1 audit log deleted (20d old), got %d", result.AuditLogs)
	}
	var auditCount int64
	db.Model(&model.AuditLog{}).Count(&auditCount)
	if auditCount != 1 {
		t.Fatalf("expected 1 audit log remaining (10d old), got %d", auditCount)
	}
}
