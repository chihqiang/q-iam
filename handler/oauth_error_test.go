package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chihqiang/q-iam/logic"
)

// TestWriteOAuthError_OAuthError 验证 OAuth 协议错误映射为标准状态码 + {error, error_description} 响应体。
func TestWriteOAuthError_OAuthError(t *testing.T) {
	rec := httptest.NewRecorder()
	oauthErr := &logic.OAuthError{
		Code:        logic.OAuthErrorInvalidClient,
		Description: "应用凭证无效",
		Status:      http.StatusUnauthorized,
	}

	if ok := writeOAuthError(rec, oauthErr); !ok {
		t.Fatal("writeOAuthError should handle *OAuthError")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"error":"invalid_client"`) && !strings.Contains(body, `"error": "invalid_client"`) {
		t.Fatalf("response body should contain error code, got: %s", body)
	}
	if !strings.Contains(body, "应用凭证无效") {
		t.Fatalf("response body should contain error_description, got: %s", body)
	}
}

// TestWriteOAuthError_DefaultStatus 验证 Status=0 时回退为 400（RFC 6749 默认）。
func TestWriteOAuthError_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	if ok := writeOAuthError(rec, &logic.OAuthError{Code: logic.OAuthErrorInvalidRequest}); !ok {
		t.Fatal("writeOAuthError should handle *OAuthError")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected default status 400, got %d", rec.Code)
	}
}

// TestWriteOAuthError_PlainError 验证普通错误不写入 OAuth 响应（交由调用方按 500 处理）。
func TestWriteOAuthError_PlainError(t *testing.T) {
	rec := httptest.NewRecorder()
	if ok := writeOAuthError(rec, errors.New("boom")); ok {
		t.Fatal("writeOAuthError should return false for plain errors")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("plain error should not write response, got status %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("plain error should not write body, got: %s", rec.Body.String())
	}
}
