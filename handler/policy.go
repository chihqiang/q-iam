package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"
	"chihqiang/q-iam/middleware"

	"github.com/chihqiang/infra-go/httpx"
)

// PolicyHandler 权限策略 HTTP 处理器。
// 权限策略 CRUD；策略与主体的多对多关系通过 /grants 授权管理。
type PolicyHandler struct {
	svc *logic.PolicyLogic
}

// NewPolicyHandler 创建权限策略处理器。
func NewPolicyHandler(svc *logic.PolicyLogic) *PolicyHandler {
	return &PolicyHandler{svc: svc}
}

// List 策略列表（按数据范围过滤）。
func (h *PolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.PolicyListRequest
	if err := httpx.MustBindQuery(w, r, &req); err != nil {
		return
	}

	resp, err := h.svc.List(ctx, accountIDForScope(ctx), &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, resp)
}

// AllList 全部启用的策略（授权选择用，按数据范围过滤）。
func (h *PolicyHandler) AllList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	policies, err := h.svc.AllList(ctx, accountIDForScope(ctx))
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, policies)
}

// Detail 策略详情（按数据范围校验可见性，防止越权查看）。
func (h *PolicyHandler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	viewerID := accountIDForScope(ctx)
	if viewerID > 0 {
		ok, err := h.svc.CanViewPolicy(ctx, viewerID, id)
		if err != nil {
			httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
			return
		}
		if !ok {
			httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeForbidden, "无权限访问")
			return
		}
	}

	policy, err := h.svc.GetByID(ctx, id)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, policy)
}

// Create 创建策略。
func (h *PolicyHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.PolicyCreateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	// 注入创建者 ID（从当前登录账号上下文），供数据范围 self=本人创建 过滤
	if account := middleware.AccountFromContext(ctx); account != nil {
		req.CreatedBy = account.ID
	}

	policy, err := h.svc.Create(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, policy)
}

// Update 更新策略。
func (h *PolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.PolicyUpdateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}
	req.ID = id
	// 注入当前操作账号 ID（非 admin 仅可关联本人创建/系统内置的语句）
	if account := middleware.AccountFromContext(ctx); account != nil {
		req.CreatedBy = account.ID
	}

	policy, err := h.svc.Update(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, policy)
}

// Delete 删除策略。
func (h *PolicyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	if err := h.svc.Delete(ctx, id); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, nil)
}
