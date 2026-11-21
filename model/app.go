package model

import "time"

// 授权类型常量（GrantType 取值，对齐 OAuth 2.0）。
const (
	// AppGrantTypeClientCredentials 客户端凭证模式（默认）。
	AppGrantTypeClientCredentials = "client_credentials"
	// AppGrantTypeAuthorizationCode 授权码模式。
	AppGrantTypeAuthorizationCode = "authorization_code"
)

// App 应用（集成管理），Auth 应用实体。
// 对应 RAM 中的应用/OAuth 客户端：通过客户端 ID + 密钥（client_id/client_secret）
// 获取访问凭证调用受保护 API。
type App struct {
	ID   int64  `json:"id" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	Name string `json:"name" gorm:"size:64;not null;comment:应用名称"`
	// AppID 客户端 ID（唯一，签发后不可变更）。
	AppID string `json:"app_id" gorm:"size:64;uniqueIndex;not null;comment:客户端ID"`
	// AppSecret 客户端密钥（加密存储，仅创建/重置时返回明文）。
	AppSecret   string `json:"-" gorm:"size:256;not null;comment:客户端密钥(加密存储)"`
	Description string `json:"description" gorm:"size:512;comment:描述"`
	// OwnerAccountID 所属账号 ID，0 表示系统应用。
	OwnerAccountID int64 `json:"owner_account_id" gorm:"comment:所属账号ID"`
	// CallbackURL 授权回调地址（authorization_code 模式使用）。
	CallbackURL string `json:"callback_url" gorm:"size:512;comment:授权回调地址"`
	// GrantType 授权类型：client_credentials | authorization_code。
	GrantType string `json:"grant_type" gorm:"size:32;default:client_credentials;comment:授权类型"`
	// Status 状态（启用/禁用），禁用后应用无法换取凭证。
	Status    bool       `json:"status" gorm:"default:true;comment:状态（启用/禁用）"`
	CreatedAt time.Time  `json:"created_at" gorm:"comment:创建时间"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"comment:更新时间"`
	DeletedAt *time.Time `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名。
func (App) TableName() string {
	return "q_iam_apps"
}
