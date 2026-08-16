// Package model 定义 q-iam 的数据模型。
//
// policy.go 承载权限策略体系的全部模型：
//   - Policy / PolicyStatement / DataScope：权限策略及其授权规则明细
//   - PrincipalType / PolicyAttachment：授权主体类型与授权关系（策略绑定到主体）
package model

import "time"

// 策略类型。
const (
	// PolicyTypeSystem 系统内置策略，由系统创建，不可修改/删除。
	PolicyTypeSystem = "system"
	// PolicyTypeCustom 自定义策略，可编辑/删除。
	PolicyTypeCustom = "custom"
)

// Policy 权限策略（系统/自定义策略），权限管理核心实体。
// 策略声明某类主体（账号/账号组/角色/应用）可执行的操作（Action）与资源（Resource）；
// 授权规则以 Statement 明细行存储，每条一行（q_iam_policy_statements），避免单字段过大。
// 策略与主体的绑定关系见 PolicyAttachment。
type Policy struct {
	ID   int64  `json:"id" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	Name string `json:"name" gorm:"size:64;uniqueIndex;not null;comment:策略名"`
	// Version 策略版本。
	Version string `json:"version" gorm:"size:16;default:1;comment:策略版本"`
	// Description 策略描述。
	Description string `json:"description" gorm:"size:512;comment:描述"`
	// Type 策略类型：system（系统内置，不可修改/删除）| custom（自定义）。
	Type string `json:"type" gorm:"size:16;default:custom;comment:策略类型 system/custom"`
	// Status 状态（启用/禁用），禁用后授权关系不生效。
	Status bool `json:"status" gorm:"default:true;comment:状态（启用/禁用）"`
	// Statements 策略授权规则明细。
	Statements []PolicyStatement `json:"statements" gorm:"foreignKey:PolicyID"`
	// CreatedBy 创建者用户 ID，0 表示系统创建。
	CreatedBy int64      `json:"created_by" gorm:"comment:创建者用户ID"`
	CreatedAt time.Time  `json:"created_at" gorm:"comment:创建时间"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"comment:更新时间"`
	DeletedAt *time.Time `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名。
func (Policy) TableName() string {
	return "q_iam_policies"
}

// PolicyStatement 策略授权规则（明细表）。
// 对应策略 JSON 中的一条 Statement：
//
//	{
//	  "Effect": "Allow",
//	  "Action": "iam:CreateAccount,iam:UpdateAccount",
//	  "Resource": "*",
//	  "Condition": {...}
//	}
type PolicyStatement struct {
	ID int64 `json:"id" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	// PolicyID 所属权限策略 ID。
	PolicyID int64 `json:"policy_id" gorm:"not null;index;comment:权限策略ID"`
	// Description 语句描述（小标题，说明本条授权规则的用途）。
	Description string `json:"description" gorm:"size:255;comment:语句描述"`
	// Effect 效果：Allow（允许）| Deny（拒绝）。
	Effect string `json:"effect" gorm:"size:8;not null;comment:效果 Allow/Deny"`
	// Action 操作（逗号分隔，如 "iam:CreateAccount,iam:UpdateAccount"）。
	Action string `json:"action" gorm:"type:text;not null;comment:操作(逗号分隔)"`
	// Scopes 数据范围明细（数据权限：可见/操作哪部分数据，每条一行）。
	Scopes []DataScope `json:"scopes" gorm:"foreignKey:StatementID"`
	// Sort 排序。
	Sort int64 `json:"sort" gorm:"default:0;comment:排序"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (PolicyStatement) TableName() string {
	return "q_iam_policy_statements"
}

// PrincipalType 授权主体类型（强类型枚举）。
// 决定权限策略可绑定到哪些类型的主体。
type PrincipalType string

const (
	// PrincipalTypeAccount 账号。
	PrincipalTypeAccount PrincipalType = "account"
	// PrincipalTypeGroup 账号组。
	PrincipalTypeGroup PrincipalType = "group"
	// PrincipalTypeApp 应用（OAuth2 接入的服务/系统）。
	PrincipalTypeApp PrincipalType = "app"
)

// Valid 校验主体类型是否为合法枚举值。
func (t PrincipalType) Valid() bool {
	switch t {
	case PrincipalTypeAccount, PrincipalTypeGroup, PrincipalTypeApp:
		return true
	}
	return false
}

// String 返回主体类型的字符串表示。
func (t PrincipalType) String() string {
	return string(t)
}

// PolicyAttachment 授权关系（RAM 授权），权限管理核心实体。
// 将权限策略绑定到某个主体（账号/账号组/应用），
// 账号最终权限 = 直接绑定的策略 + 所属账号组绑定的策略。
type PolicyAttachment struct {
	ID int64 `json:"id" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	// PrincipalType 主体类型：account | group | app。
	// 数据库层通过 CHECK 约束强制取值，避免写入非法枚举值。
	PrincipalType PrincipalType `json:"principal_type" gorm:"size:16;not null;index;check:chk_principal_type,principal_type IN ('account','group','app');comment:主体类型 account/group/app"`
	// PrincipalID 主体 ID。
	PrincipalID int64 `json:"principal_id" gorm:"not null;index;comment:主体ID"`
	// PolicyID 绑定的策略 ID。
	PolicyID  int64     `json:"policy_id" gorm:"not null;index;comment:策略ID"`
	CreatedBy int64     `json:"created_by" gorm:"comment:授权人用户ID"`
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (PolicyAttachment) TableName() string {
	return "q_iam_policy_attachments"
}
