package handler

import (
	"net/http"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
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
	if err := httpx.MustBindJSON(w, r, &req); err != nil {
		return
	}

	result, err := h.svc.CleanupHistory(ctx, req.Days)
	if err != nil {
		httpx.OkJSONCtx(ctx, w, httpx.NewCodeError(httpx.CodeDefaultError, err.Error()))
		return
	}
	httpx.OkJSONCtx(ctx, w, result)
}
