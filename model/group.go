package model

import "time"

// Group 账号组（RAM 账号组），身份管理核心实体。
// 账号加入账号组后自动获得组内绑定的权限策略。
type Group struct {
	ID          int64  `json:"id" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	Name        string `json:"name" gorm:"size:64;uniqueIndex;not null;comment:账号组名（唯一标识）"`
	DisplayName string `json:"display_name" gorm:"size:64;comment:显示名称"`
	Description string `json:"description" gorm:"size:512;comment:描述"`
	// Status 状态（启用/禁用），禁用后组内账号不再获得该组绑定的权限。
	Status bool `json:"status" gorm:"default:true;comment:状态（启用/禁用）"`
	// Accounts 组内账号（多对多）。
	Accounts  []Account  `json:"accounts" gorm:"many2many:q_iam_account_groups;comment:组内账号"`
	CreatedAt time.Time  `json:"created_at" gorm:"comment:创建时间"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"comment:更新时间"`
	DeletedAt *time.Time `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名。
func (Group) TableName() string {
	return "q_iam_groups"
}
