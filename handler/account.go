package handler

import (
	"net/http"
	"strconv"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
)

// AccountHandler 账号管理 HTTP 处理器。
type AccountHandler struct {
	svc *logic.AccountLogic
}

// NewAccountHandler 创建账号处理器。
func NewAccountHandler(svc *logic.AccountLogic) *AccountHandler {
	return &AccountHandler{svc: svc}
}

// List 账号列表。
func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.AccountListRequest
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

// AllList 全部启用的账号（授权下拉选择用）。
func (h *AccountHandler) AllList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accounts, err := h.svc.AllList(ctx)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, accounts)
}

// Detail 账号详情。
func (h *AccountHandler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	account, err := h.svc.GetByID(ctx, id)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, account)
}

// Create 创建账号。
func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.AccountCreateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	account, err := h.svc.Create(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, account)
}

// Update 更新账号。
func (h *AccountHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.AccountUpdateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}
	req.ID = id

	account, err := h.svc.Update(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, account)
}

// Delete 删除账号。
func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// ChangePassword 修改密码。
func (h *AccountHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.ChangePasswordRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}
	req.ID = id

	if err := h.svc.ChangePassword(ctx, &req); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, nil)
}

// ResetPassword 重置密码（管理员）。
func (h *AccountHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.ResetPasswordRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}
	req.ID = id

	if err := h.svc.ResetPassword(ctx, &req); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, nil)
}
