package middleware

import (
	"context"

	"chihqiang/q-iam/logic"
	"chihqiang/q-iam/model"
)

type contextKey string

const (
	accountContextKey       contextKey = "account"
	permissionSetContextKey contextKey = "permission_set"
	auditMetaContextKey     contextKey = "audit_meta"
)

// ContextWithAccount 将当前登录账号注入上下文。
func ContextWithAccount(ctx context.Context, account *model.Account) context.Context {
	return context.WithValue(ctx, accountContextKey, account)
}

// AccountFromContext 从上下文提取当前登录账号，不存在时返回 nil。
func AccountFromContext(ctx context.Context) *model.Account {
	account, _ := ctx.Value(accountContextKey).(*model.Account)
	return account
}

// ContextWithPermissionSet 将当前账号已加载的权限集合注入上下文，
// 供同一请求链路内后续中间件/处理器复用，避免重复加载。
func ContextWithPermissionSet(ctx context.Context, ps *logic.PermissionSet) context.Context {
	return context.WithValue(ctx, permissionSetContextKey, ps)
}

// PermissionSetFromContext 从上下文提取当前账号的权限集合，不存在时返回 nil。
func PermissionSetFromContext(ctx context.Context) *logic.PermissionSet {
	ps, _ := ctx.Value(permissionSetContextKey).(*logic.PermissionSet)
	return ps
}

// auditMeta 路由注册时声明的审计元数据（模块/动作），随请求写入 context。
// 这样新增路由只需在 route.go 声明，无需再修改 logic/audit.go 的路径分类逻辑。
type auditMeta struct {
	module string
	action string
}

// ContextWithAuditMeta 将审计元数据（模块/动作）注入上下文。
func ContextWithAuditMeta(ctx context.Context, meta auditMeta) context.Context {
	return context.WithValue(ctx, auditMetaContextKey, meta)
}

// AuditMetaFromContext 从上下文提取审计元数据，不存在时返回零值。
func AuditMetaFromContext(ctx context.Context) auditMeta {
	meta, _ := ctx.Value(auditMetaContextKey).(auditMeta)
	return meta
}
