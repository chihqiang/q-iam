package handler

import (
	"errors"
	"net/http"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
)

// writeOAuthError 将 OAuth 协议错误（logic.OAuthError）写为标准响应：
//   - HTTP 状态码取 OAuthError.Status（400/401/503 等），而非业务层统一的 200；
//   - 响应体对齐 RFC 6749 §5.2：{ "error": "...", "error_description": "..." }，
//     供标准 OAuth 客户端解析（第三方接入）。
//
// 返回是否已写出响应。传入非 OAuthError 的普通错误时返回 false（由调用方按 500 处理）。
func writeOAuthError(w http.ResponseWriter, err error) bool {
	var oauthErr *logic.OAuthError
	if !errors.As(err, &oauthErr) {
		return false
	}
	body := map[string]string{"error": oauthErr.Code}
	if oauthErr.Description != "" {
		body["error_description"] = oauthErr.Description
	}
	status := oauthErr.Status
	if status == 0 {
		status = http.StatusBadRequest
	}
	httpx.WriteJSON(w, status, body)
	return true
}
