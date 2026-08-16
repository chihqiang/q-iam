package store

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

// newTestDBStore 构造基于内存 SQLite 的 DBStore。
func newTestDBStore(t *testing.T) *DBStore {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.KeyStoreItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewDBStore(db)
}

// TestDBStoreGetSetDel 验证 DBStore 的基础读写删。
func TestDBStoreGetSetDel(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	// 空读
	if v, err := s.Get(ctx, "k1"); err != nil || v != "" {
		t.Fatalf("empty get: v=%q err=%v", v, err)
	}

	// 写入并读取
	if err := s.Set(ctx, "k1", "v1", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v, err := s.Get(ctx, "k1"); err != nil || v != "v1" {
		t.Fatalf("get: v=%q err=%v", v, err)
	}

	// 覆盖写入
	if err := s.Set(ctx, "k1", "v2", time.Minute); err != nil {
		t.Fatalf("set2: %v", err)
	}
	if v, _ := s.Get(ctx, "k1"); v != "v2" {
		t.Fatalf("overwrite get: v=%q", v)
	}

	// 删除
	if err := s.Del(ctx, "k1"); err != nil {
		t.Fatalf("del: %v", err)
	}
	if v, _ := s.Get(ctx, "k1"); v != "" {
		t.Fatalf("after del: v=%q", v)
	}
}

// TestDBStoreTTLExpiry 验证过期读取返回空。
func TestDBStoreTTLExpiry(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	if err := s.Set(ctx, "k-ttl", "v", 50*time.Millisecond); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v, _ := s.Get(ctx, "k-ttl"); v != "v" {
		t.Fatalf("before expiry: %q", v)
	}
	time.Sleep(80 * time.Millisecond)
	if v, _ := s.Get(ctx, "k-ttl"); v != "" {
		t.Fatalf("after expiry should be empty, got %q", v)
	}

	// ttl<=0 表示不过期
	if err := s.Set(ctx, "k-forever", "v", 0); err != nil {
		t.Fatalf("set forever: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if v, _ := s.Get(ctx, "k-forever"); v != "v" {
		t.Fatalf("no-ttl should persist: %q", v)
	}
}

// TestDBStoreSetNX 验证 SetNX 的原子占位语义（一次性消费）。
func TestDBStoreSetNX(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	ok, err := s.SetNX(ctx, "once", "1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first setnx should succeed: ok=%v err=%v", ok, err)
	}
	ok, err = s.SetNX(ctx, "once", "1", time.Minute)
	if err != nil || ok {
		t.Fatalf("second setnx should fail: ok=%v err=%v", ok, err)
	}

	// 过期后可再次占位
	if err := s.Set(ctx, "once2", "1", 50*time.Millisecond); err != nil {
		t.Fatalf("set once2: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	ok, err = s.SetNX(ctx, "once2", "1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("setnx after expiry should succeed: ok=%v err=%v", ok, err)
	}
}

// TestDBStoreIncr 验证原子自增。
func TestDBStoreIncr(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	for i, want := range []int64{1, 2, 3} {
		v, err := s.Incr(ctx, "counter", time.Minute)
		if err != nil || v != want {
			t.Fatalf("incr #%d: v=%d want=%d err=%v", i, v, want, err)
		}
	}

	// 首次创建带 TTL；这里验证过期后从 0 重新计
	s2 := newTestDBStore(t)
	ctx2 := context.Background()
	if _, err := s2.Incr(ctx2, "c2", 50*time.Millisecond); err != nil {
		t.Fatalf("incr c2: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if v, err := s2.Incr(ctx2, "c2", time.Minute); err != nil || v != 1 {
		t.Fatalf("incr after expiry should restart at 1: v=%d err=%v", v, err)
	}
}

// TestKVStoreContract 验证接口级契约：DBStore 满足 KVStore 接口（编译期断言 + 基础行为）。
func TestKVStoreContract(t *testing.T) {
	var _ KVStore = NewDBStore(nil)
	var _ KVStore = (*RedisStore)(nil) // 编译期断言 RedisStore 也实现 KVStore
	_ = context.Background()
}
