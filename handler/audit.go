package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
)

// AuditHandler 操作审计 HTTP 处理器。
type AuditHandler struct {
	svc *logic.AuditLogic
}

// NewAuditHandler 创建操作审计处理器。
func NewAuditHandler(svc *logic.AuditLogic) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// List 审计日志分页列表。
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.AuditListRequest
	if err := httpx.MustBindQuery(w, r, &req); err != nil {
		return
	}

	resp, err := h.svc.List(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, resp)
}

// Modules 审计模块枚举（供前端筛选）。
func (h *AuditHandler) Modules(w http.ResponseWriter, r *http.Request) {
	httpx.OkJSONCtx(r.Context(), w, logic.AuditModuleOptions)
}
