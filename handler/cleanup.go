package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/ten-go/rest"
)

// CleanupHandler 历史数据清理 HTTP 处理器。
type CleanupHandler struct {
	svc *logic.CleanupLogic
}

// NewCleanupHandler 创建历史数据清理处理器。
func NewCleanupHandler(svc *logic.CleanupLogic) *CleanupHandler {
	return &CleanupHandler{svc: svc}
}

// CleanupRequest 清理历史数据请求。
type CleanupRequest struct {
	// Days 清理 days 天以前的数据；<=0 时用默认 30 天。
	Days int `json:"days"`
}

// History 清理历史数据（需 iam:system:cleanup 权限）。
// POST /api/v1/cleanup/history
func (h *CleanupHandler) History(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req CleanupRequest
	if err := rest.Bind(r, &req); err != nil {
		_ = rest.ErrorJSONCtx(r.Context(), w, rest.NewCodeError(rest.StatusOf(err), err.Error()))
		return
	}

	result, err := h.svc.CleanupHistory(ctx, req.Days)
	if err != nil {
		rest.OkJSONCtx(ctx, w, rest.NewCodeError(rest.BusinessCodeError, err.Error()))
		return
	}
	rest.OkJSONCtx(ctx, w, result)
}
