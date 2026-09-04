package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"
	"chihqiang/q-iam/middleware"

	"github.com/chihqiang/infra-go/httpx"
)

// StatementHandler 授权语句（语句池）HTTP 处理器。
// 语句独立菜单管理；策略新增/编辑只负责关联（选择已有语句）。
type StatementHandler struct {
	svc *logic.StatementLogic
}

// NewStatementHandler 创建授权语句处理器。
func NewStatementHandler(svc *logic.StatementLogic) *StatementHandler {
	return &StatementHandler{svc: svc}
}

// List 语句池分页列表（按可见性过滤）。
func (h *StatementHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.StatementListRequest
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

// AllList 全部语句（策略关联选择用，按可见性过滤）。
func (h *StatementHandler) AllList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	statements, err := h.svc.AllList(ctx, accountIDForScope(ctx))
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, statements)
}

// Detail 语句详情（含数据范围明细）。
func (h *StatementHandler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	statement, err := h.svc.GetByID(ctx, id)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, statement)
}

// Create 创建授权语句。
func (h *StatementHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.StatementCreateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	// 注入创建者 ID（从当前登录账号上下文），供可见性过滤（self=本人创建）
	if account := middleware.AccountFromContext(ctx); account != nil {
		req.CreatedBy = account.ID
	}

	statement, err := h.svc.Create(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, statement)
}

// Update 更新授权语句。
func (h *StatementHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	// 越权保护：非 admin 仅可管理本人创建的语句，系统内置语句不可由普通账号修改
	if viewerID := accountIDForScope(ctx); viewerID > 0 {
		ok, err := h.svc.CanManage(ctx, viewerID, id)
		if err != nil {
			httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
			return
		}
		if !ok {
			httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeForbidden, "无权限管理该授权语句")
			return
		}
	}

	var req logic.StatementUpdateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}
	req.ID = id

	statement, err := h.svc.Update(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, statement)
}

// Delete 删除授权语句（被策略关联时禁止删除）。
func (h *StatementHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	// 越权保护：非 admin 仅可管理本人创建的语句，系统内置语句不可由普通账号删除
	if viewerID := accountIDForScope(ctx); viewerID > 0 {
		ok, err := h.svc.CanManage(ctx, viewerID, id)
		if err != nil {
			httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
			return
		}
		if !ok {
			httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeForbidden, "无权限管理该授权语句")
			return
		}
	}

	if err := h.svc.Delete(ctx, id); err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, nil)
}
