package handler

import (
	"net/http"
	"strconv"

	"chihqiang/q-iam/logic"

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

// List 策略列表。
func (h *PolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.PolicyListRequest
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

// AllList 全部启用的策略。
func (h *PolicyHandler) AllList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	policies, err := h.svc.AllList(ctx)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, policies)
}

// Detail 策略详情。
func (h *PolicyHandler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
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
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.PolicyUpdateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}
	req.ID = id

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
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	if err := h.svc.Delete(ctx, id); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, nil)
}
