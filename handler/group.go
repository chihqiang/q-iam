package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
)

// GroupHandler 账号组 HTTP 处理器。
type GroupHandler struct {
	svc *logic.GroupLogic
}

// NewGroupHandler 创建账号组处理器。
func NewGroupHandler(svc *logic.GroupLogic) *GroupHandler {
	return &GroupHandler{svc: svc}
}

// List 账号组列表（按数据范围过滤）。
func (h *GroupHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.GroupListRequest
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

// AllList 全部启用的账号组（授权下拉选择用，按数据范围过滤）。
func (h *GroupHandler) AllList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	groups, err := h.svc.AllList(ctx, accountIDForScope(ctx))
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, groups)
}

// Detail 账号组详情（按数据范围校验可见性，防止越权查看）。
func (h *GroupHandler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	viewerID := accountIDForScope(ctx)
	if viewerID > 0 {
		ok, err := h.svc.CanViewGroup(ctx, viewerID, id)
		if err != nil {
			httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
			return
		}
		if !ok {
			httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeForbidden, "无权限访问")
			return
		}
	}

	group, err := h.svc.GetByID(ctx, id)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, group)
}

// Create 创建账号组。
func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.GroupCreateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	group, err := h.svc.Create(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, group)
}

// Update 更新账号组。
func (h *GroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.GroupUpdateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}
	req.ID = id

	group, err := h.svc.Update(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, group)
}

// Delete 删除账号组。
func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// AddMembers 添加成员。
func (h *GroupHandler) AddMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.GroupMemberRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	if err := h.svc.AddMembers(ctx, id, req.AccountIDs); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, nil)
}

// RemoveMembers 移除成员。
func (h *GroupHandler) RemoveMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.GroupMemberRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	if err := h.svc.RemoveMembers(ctx, id, req.AccountIDs); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, nil)
}

// ReplaceMembers 覆盖成员。
func (h *GroupHandler) ReplaceMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.GroupMemberRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	if err := h.svc.ReplaceMembers(ctx, id, req.AccountIDs); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, nil)
}
