package model

import "time"

// AuditLog 操作审计日志（RAM 操作审计）。
// 记录所有写操作（增删改、登录、授权等）的关键信息，用于安全追溯与合规审计。
type AuditLog struct {
	ID int64 `json:"id" gorm:"primaryKey;autoIncrement;comment:主键ID"`
	// OperatorID 操作人账号 ID，0 表示匿名（未登录）或系统操作。
	OperatorID int64 `json:"operator_id" gorm:"index;comment:操作人账号ID"`
	// OperatorName 操作人账号名（冗余存储，便于检索展示）。
	OperatorName string `json:"operator_name" gorm:"size:64;index;comment:操作人账号名"`
	// Module 操作模块：auth/account/group/policy/grant/app/oauth。
	Module string `json:"module" gorm:"size:32;index;comment:操作模块"`
	// Action 操作动作：login/create/update/delete/grant/revoke/reset_secret/authorize 等。
	Action string `json:"action" gorm:"size:32;index;comment:操作动作"`
	// Method HTTP 方法。
	Method string `json:"method" gorm:"size:16;comment:HTTP方法"`
	// Path 请求路径。
	Path string `json:"path" gorm:"size:256;comment:请求路径"`
	// Detail 操作详情（人类可读描述）。
	Detail string `json:"detail" gorm:"size:512;comment:操作详情"`
	// ClientIP 客户端 IP。
	ClientIP string `json:"client_ip" gorm:"size:64;comment:客户端IP"`
	// UserAgent 客户端 User-Agent。
	UserAgent string `json:"user_agent" gorm:"size:512;comment:User-Agent"`
	// Success 是否成功。
	Success bool `json:"success" gorm:"index;comment:是否成功"`
	// ErrorMsg 失败原因（成功时为空）。
	ErrorMsg string `json:"error_msg" gorm:"size:512;comment:失败原因"`
	// LatencyMs 耗时（毫秒）。
	LatencyMs int64 `json:"latency_ms" gorm:"comment:耗时(毫秒)"`
	// CreatedAt 操作时间。
	CreatedAt time.Time `json:"created_at" gorm:"index;comment:操作时间"`
}

// TableName 指定表名。
func (AuditLog) TableName() string {
	return "q_iam_audit_logs"
}
