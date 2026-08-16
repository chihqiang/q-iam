package model

import "time"

// DataScopeType 数据范围类型（强类型枚举）。
// 数据权限：允许某主体访问某类数据资源中的哪一部分。
// 关联表存储（不存 JSON）：每个数据范围一行，挂在策略 Statement 下。
type DataScopeType string

const (
	// DataScopeAll 全部数据（该 Statement 不做数据范围限制）。
	DataScopeAll DataScopeType = "all"
	// DataScopeGroup 本用户分组的数据（按 GroupID，多行=多组并集）。
	DataScopeGroup DataScopeType = "group"
	// DataScopeSelf 仅本人数据（按 OwnerField 指定数据归属字段，值为当前账号 ID）。
	DataScopeSelf DataScopeType = "self"
	// DataScopeAttribute 按数据属性/标签过滤（AttrKey + AttrValue，多行叠加=OR）。
	DataScopeAttribute DataScopeType = "attribute"
)

// Valid 校验数据范围类型是否为合法枚举值。
func (t DataScopeType) Valid() bool {
	switch t {
	case DataScopeAll, DataScopeGroup, DataScopeSelf, DataScopeAttribute:
		return true
	}
	return false
}

// String 返回数据范围类型的字符串表示。
func (t DataScopeType) String() string {
	return string(t)
}

// DataScope 策略语句的数据范围（明细表）。
// 作为 PolicyStatement 的子表，每条数据范围一行，
// 不存 JSON，随策略 CRUD 整体维护（级联删除 + 重建）。
type DataScope struct {
	ID int64 `json:"id" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	// StatementID 所属 Statement ID。
	StatementID int64 `json:"statement_id" gorm:"not null;index;comment:所属StatementID"`
	// ScopeType 数据范围类型：all | group | self | attribute。
	ScopeType DataScopeType `json:"scope_type" gorm:"size:16;not null;index;check:chk_statement_scope_type,scope_type IN ('all','group','self','attribute');comment:数据范围类型 all/group/self/attribute"`
	// GroupID 用户分组 ID（scope_type=group 时使用，0 表示不适用）。
	GroupID int64 `json:"group_id" gorm:"default:0;comment:用户分组ID(group类型)"`
	// OwnerField 数据归属字段名（scope_type=self 时使用，值为当前账号 ID）。
	OwnerField string `json:"owner_field" gorm:"size:128;comment:数据归属字段(self类型)"`
	// AttrKey 数据属性键（scope_type=attribute 时使用）。
	AttrKey string `json:"attr_key" gorm:"size:128;comment:数据属性键(attribute类型)"`
	// AttrValue 数据属性值（scope_type=attribute 时使用）。
	AttrValue string `json:"attr_value" gorm:"size:255;comment:数据属性值(attribute类型)"`
	// Sort 排序。
	Sort int64 `json:"sort" gorm:"default:0;comment:排序"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (DataScope) TableName() string {
	return "q_iam_statement_scopes"
}
