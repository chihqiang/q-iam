package model

import "time"

// AdminAccountName 内置超级管理员账号名。
// 该账号拥有全部权限（权限中间件按账号名放行）、不可删除（删除保护），
// 由 db.Migrate 种子数据创建。全包统一引用此常量，避免硬编码字符串分散。
const AdminAccountName = "admin"

// Account 账号（RAM 账号），身份管理核心实体。
type Account struct {
	ID          int64  `json:"id" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	AccountName string `json:"account_name" gorm:"size:64;uniqueIndex;not null;comment:账号名（登录名）"`
	DisplayName string `json:"display_name" gorm:"size:64;comment:显示名"`
	// Email 邮箱（可空，非空时唯一，空字符串存 NULL）。
	Email *string `json:"email" gorm:"size:128;uniqueIndex;comment:邮箱"`
	// Mobile 手机号（可空，非空时唯一，空字符串存 NULL）。
	Mobile   *string `json:"mobile" gorm:"size:32;uniqueIndex;comment:手机号"`
	Password string  `json:"-" gorm:"size:256;not null;comment:密码（Bcrypt）"`
	Status   bool    `json:"status" gorm:"default:true;comment:状态（启用/禁用）"`
	// AllowConsole 是否允许进入管理控制台。注册账号为 false（仅用于 OAuth2 授权登录）；
	// 管理员创建的账号默认 true（在创建逻辑中显式赋值，避免 GORM bool 零值被忽略）。
	AllowConsole bool       `json:"allow_console" gorm:"comment:是否允许进入控制台"`
	LastLoginAt  *time.Time `json:"last_login_at" gorm:"comment:最后登录时间"`
	// LoginFailCount 连续登录失败次数。
	LoginFailCount int `json:"-" gorm:"default:0;comment:连续登录失败次数"`
	// LockedUntil 登录锁定截止时间，nil 表示未锁定。
	LockedUntil *time.Time `json:"-" gorm:"comment:登录锁定截止时间"`
	// PasswordChangedAt 密码最后修改时间（密码过期策略 expire_days 依据；nil 表示未记录/未过期）。
	PasswordChangedAt *time.Time `json:"-" gorm:"comment:密码最后修改时间"`
	Remark            string     `json:"remark" gorm:"size:512;comment:备注"`
	// Groups 所属账号组（多对多）。
	Groups    []Group    `json:"groups" gorm:"many2many:q_iam_account_groups;comment:所属账号组"`
	CreatedAt time.Time  `json:"created_at" gorm:"comment:创建时间"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"comment:更新时间"`
	DeletedAt *time.Time `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名。
func (Account) TableName() string {
	return "q_iam_accounts"
}

// PasswordHistory 密码历史（防止重复使用最近用过的密码）。
// 仅在配置 password_policy.history_count > 0 时记录，每行保存一个历史密码哈希。
// 通过模型软删除级联管理：账号删除时一并清理。
type PasswordHistory struct {
	ID           int64     `json:"-" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	AccountID    int64     `json:"account_id" gorm:"not null;index;comment:账号ID"`
	PasswordHash string    `json:"-" gorm:"size:256;not null;comment:历史密码(Bcrypt)"`
	CreatedAt    time.Time `json:"created_at" gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (PasswordHistory) TableName() string {
	return "q_iam_password_history"
}

// 刷新令牌吊销原因（RevokeReason 取值）。
// 用于区分吊销来源，决定「已吊销令牌再次被使用」时是否触发重用连坐：
//   - rotated（轮换）：旧令牌重放视为疑似盗用，吊销该账号全部刷新令牌；
//   - logout（主动退出）：仅吊销当前会话，令牌再使用不连坐其他会话；
//   - reuse（重用检测）：已吊销令牌被再次使用后统一吊销全部（连坐后的标记）；
//   - revoke（安全吊销）：改密/禁用等场景吊销全部。
const (
	RefreshTokenRevokeRotated = "rotated"
	RefreshTokenRevokeLogout  = "logout"
	RefreshTokenRevokeReuse   = "reuse"
	RefreshTokenRevokeRevoke  = "revoke"
)

// RefreshToken 刷新令牌（数据库表存储，用于刷新令牌的轮换与吊销）。
//
// 设计说明：
//   - 每次签发刷新令牌时落库一条记录，token_id 为刷新令牌携带的唯一 jti；
//   - 刷新轮换时旧记录标记 revoked_at + revoke_reason=rotated（原子消费，防并发/重用竞态）；
//   - 主动退出标记 revoke_reason=logout，仅吊销当前会话；
//   - 已吊销令牌再次被使用：rotated/reuse 触发吊销该账号全部刷新令牌（疑似盗用），
//     logout 不连坐（用户主动退出后的残留请求不应误伤其他会话）；
//   - 记录带签发来源（IP/User-Agent），便于安全追溯。
type RefreshToken struct {
	ID        int64     `json:"-" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	TokenID   string    `json:"-" gorm:"size:64;uniqueIndex;not null;comment:刷新令牌唯一标识(jti)"`
	AccountID int64     `json:"-" gorm:"not null;index;comment:账号ID"`
	ExpiresAt time.Time `json:"-" gorm:"not null;index;comment:过期时间"`
	// RevokedAt 吊销时间，nil 表示有效。已轮换/已吊销的记录该字段非空。
	RevokedAt *time.Time `json:"-" gorm:"index;comment:吊销时间(nil=有效)"`
	// RevokeReason 吊销原因：rotated（轮换）/ logout（主动退出）/ reuse（重用）/ revoke（安全吊销）。
	RevokeReason string `json:"-" gorm:"size:16;comment:吊销原因"`
	// ClientIP 签发时客户端 IP。
	ClientIP string `json:"-" gorm:"size:64;comment:签发IP"`
	// UserAgent 签发时客户端 User-Agent。
	UserAgent string    `json:"-" gorm:"size:512;comment:签发User-Agent"`
	CreatedAt time.Time `json:"-" gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (RefreshToken) TableName() string {
	return "q_iam_refresh_tokens"
}
