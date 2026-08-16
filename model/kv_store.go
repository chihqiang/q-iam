package model

import "time"

// KeyStoreItem 通用键值存储条目（数据库版 KVStore 的底层表）。
// 用于无 Redis 时提供多节点共享的缓存 / 一次性消费 / 计数存储。
type KeyStoreItem struct {
	// Key 键（主键，天然唯一，支撑 SetNX 原子占位）。
	Key string `json:"key" gorm:"primaryKey;size:255;comment:键"`
	// Value 值（字符串存储，计数时存数字字符串）。
	Value string `json:"value" gorm:"type:text;comment:值"`
	// ExpiresAt 过期时间，nil 表示不过期。
	ExpiresAt *time.Time `json:"expires_at" gorm:"index;comment:过期时间(nil=不过期)"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (KeyStoreItem) TableName() string {
	return "q_iam_kv_store"
}
