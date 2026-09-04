package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
)

// AppHandler 应用 HTTP 处理器。
type AppHandler struct {
	svc *logic.AppLogic
}

// NewAppHandler 创建应用处理器。
func NewAppHandler(svc *logic.AppLogic) *AppHandler {
	return &AppHandler{svc: svc}
}

// List 应用列表（按数据范围过滤）。
func (h *AppHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.AppListRequest
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

// AllList 全部启用的应用（授权下拉选择用，按数据范围过滤）。
func (h *AppHandler) AllList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apps, err := h.svc.AllList(ctx, accountIDForScope(ctx))
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, apps)
}

// Detail 应用详情（按数据范围校验可见性，防止越权查看）。
func (h *AppHandler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	viewerID := accountIDForScope(ctx)
	if viewerID > 0 {
		ok, err := h.svc.CanViewApp(ctx, viewerID, id)
		if err != nil {
			httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
			return
		}
		if !ok {
			httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeForbidden, "无权限访问")
			return
		}
	}

	app, err := h.svc.GetByID(ctx, id)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, app)
}

// Create 创建应用（返回明文密钥）。
func (h *AppHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req logic.AppCreateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	resp, err := h.svc.Create(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, resp)
}

// Update 更新应用。
func (h *AppHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	var req logic.AppUpdateRequest
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}
	req.ID = id

	app, err := h.svc.Update(ctx, &req)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, app)
}

// Delete 删除应用。
func (h *AppHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// ResetSecret 重置客户端密钥。
func (h *AppHandler) ResetSecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httpx.PathValue(r, "id", int64(0))
	if id <= 0 {
		httpx.WriteHTTPErrorCtx(ctx, w, httpx.CodeBadRequest, "无效的ID")
		return
	}

	secret, err := h.svc.ResetSecret(ctx, id)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, map[string]string{"app_secret": secret})
}
