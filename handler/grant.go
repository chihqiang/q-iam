package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"
	"chihqiang/q-iam/middleware"
	"chihqiang/q-iam/model"

	"github.com/chihqiang/ten-go/rest"
)

// GrantHandler 授权 HTTP 处理器。
type GrantHandler struct {
	svc *logic.GrantLogic
}

// NewGrantHandler 创建授权处理器。
func NewGrantHandler(svc *logic.GrantLogic) *GrantHandler {
	return &GrantHandler{svc: svc}
}

// Grant 绑定策略到主体。
func (h *GrantHandler) Grant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.GrantRequest
	if err := rest.Bind(r, &req); err != nil {
		_ = rest.ErrorJSONCtx(r.Context(), w, rest.NewCodeError(rest.StatusOf(err), err.Error()))
		return
	}

	// 注入授权人 ID（从当前登录账号上下文）
	if account := middleware.AccountFromContext(ctx); account != nil {
		req.CreatedBy = account.ID
	}

	if err := h.svc.Grant(ctx, &req); err != nil {
		rest.OkJSONCtx(ctx, w, rest.NewCodeError(rest.BusinessCodeError, err.Error()))
		return
	}
	rest.OkJSONCtx(ctx, w, nil)
}

// Revoke 解绑主体策略。
func (h *GrantHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.RevokeRequest
	if err := rest.Bind(r, &req); err != nil {
		_ = rest.ErrorJSONCtx(r.Context(), w, rest.NewCodeError(rest.StatusOf(err), err.Error()))
		return
	}

	if err := h.svc.Revoke(ctx, &req); err != nil {
		rest.OkJSONCtx(ctx, w, rest.NewCodeError(rest.BusinessCodeError, err.Error()))
		return
	}
	rest.OkJSONCtx(ctx, w, nil)
}

// ListByPrincipal 查询主体已绑定的策略（按数据范围过滤）。
func (h *GrantHandler) ListByPrincipal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principalType := model.PrincipalType(pathValue(r, "type", ""))
	principalID := pathValue(r, "id", int64(0))
	if principalID <= 0 {
		rest.ErrorJSONCtx(ctx, w, rest.NewCodeError(http.StatusBadRequest, "无效的ID"))
		return
	}

	policies, err := h.svc.ListByPrincipal(ctx, accountIDForScope(ctx), principalType, principalID)
	if err != nil {
		rest.OkJSONCtx(ctx, w, rest.NewCodeError(rest.BusinessCodeError, err.Error()))
		return
	}
	rest.OkJSONCtx(ctx, w, policies)
}

// ListPrincipals 查询策略被哪些主体绑定（按数据范围过滤）。
func (h *GrantHandler) ListPrincipals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	policyID := pathValue(r, "id", int64(0))
	if policyID <= 0 {
		rest.ErrorJSONCtx(ctx, w, rest.NewCodeError(http.StatusBadRequest, "无效的ID"))
		return
	}

	attachments, err := h.svc.ListPrincipals(ctx, accountIDForScope(ctx), policyID)
	if err != nil {
		rest.OkJSONCtx(ctx, w, rest.NewCodeError(rest.BusinessCodeError, err.Error()))
		return
	}
	rest.OkJSONCtx(ctx, w, attachments)
}
