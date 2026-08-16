package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"chihqiang/q-iam/logic"

	"github.com/chihqiang/infra-go/httpx"
	"github.com/chihqiang/infra-go/jwt"
)

// AuditModule 声明当前路由/分组的审计模块（如 account/group/policy/grant/app）。
// 挂载在模块路由组上，审计中间件从 context 读取，不再依赖路径前缀推断。
// 未显式声明动作时，按 HTTP 方法补齐默认动作（POST→create/PUT→update/DELETE→delete），
// 特殊动作由 AuditAction 覆盖。
func AuditModule(module string) httpx.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			prev := AuditMetaFromContext(r.Context())
			action := prev.action
			if action == "" {
				action = defaultAuditAction(r.Method)
			}
			next(w, r.WithContext(ContextWithAuditMeta(r.Context(), auditMeta{
				module: module,
				action: action,
			})))
		}
	}
}

// defaultAuditAction 由 HTTP 方法推断常规审计动作（无路径依赖，通用兜底）。
func defaultAuditAction(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// AuditAction 声明当前路由的审计动作，覆盖默认的方法推断（create/update/delete）。
// 用于特殊动作：reset_secret / reset_password / change_password / add_member 等。
func AuditAction(action string) httpx.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			prev := AuditMetaFromContext(r.Context())
			next(w, r.WithContext(ContextWithAuditMeta(r.Context(), auditMeta{
				module: prev.module,
				action: action,
			})))
		}
	}
}

// Audit 操作审计中间件。
// 自动记录所有写操作（POST/PUT/DELETE/PATCH）：
//   - 操作人：优先从 JWT claims 提取（已认证接口）；公开接口（无 JWT）从请求体
//     解析 account_name / app_id（登录、注册、应用换 Token 等）。
//   - 模块/动作：由路由注册时声明的元数据（AuditModule/AuditAction）决定。
//   - 结果：捕获响应状态码与业务错误信息（code != 0 时记录 msg）
//   - 请求上下文：IP、User-Agent、耗时
//
// 公开接口同样挂载本中间件（操作人从请求体提取），因此登录/注册/刷新/换 Token
// 的审计也走声明式声明，无需在业务 handler 里手动记录。
// 注意：应挂在认证组内（Auth 之后）或公开路由上（无 Auth 时走请求体提取）。
func Audit(auditSvc *logic.AuditLogic) httpx.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// 仅记录写操作（读操作量大且审计价值低）
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			default:
				next(w, r)
				return
			}

			// 操作人：JWT claims 优先（已认证接口）
			operatorID, operatorName := operatorFromClaims(r.Context())

			// 公开接口（无 JWT）操作人在请求体：先缓冲，供 next 后提取（读取后还原 Body）
			var reqBody []byte
			if operatorName == "" {
				reqBody = readBody(r)
			}

			start := time.Now()
			rec := newAuditRecorder(w)

			next(rec, r)

			latencyMs := time.Since(start).Milliseconds()

			// 公开接口：从请求体补全操作人（登录/注册的 account_name、换 Token 的 app_id）
			if operatorName == "" {
				if name := bodyAccountName(reqBody); name != "" {
					operatorName = name
				}
			}

			// 判定成功/失败并提取错误信息（业务错误时 HTTP 状态码仍为 200，须解析响应体 code）
			success, errMsg := rec.auditResult()

			// 模块/动作：取自路由注册时声明的元数据（AuditModule/AuditAction）。
			// 未声明的路由由 AuditLogic.Record 兜底为 system/unknown。
			meta := AuditMetaFromContext(r.Context())
			module := meta.module
			action := meta.action

			auditSvc.Record(r.Context(), logic.AuditEntry{
				OperatorID:   operatorID,
				OperatorName: operatorName,
				Module:       module,
				Action:       action,
				Method:       r.Method,
				Path:         r.URL.Path,
				Detail:       buildAuditDetail(r.Method, r.URL.Path),
				ClientIP:     ClientIP(r),
				UserAgent:    r.UserAgent(),
				Success:      success,
				ErrorMsg:     errMsg,
				LatencyMs:    latencyMs,
			})
		}
	}
}

// operatorFromClaims 从 JWT claims 提取操作人（已认证接口）。
func operatorFromClaims(ctx context.Context) (id int64, name string) {
	claims := jwt.ClaimsFromContext(ctx)
	if claims == nil {
		return 0, ""
	}
	if v, ok := claims[jwt.ClaimKeyUserID].(float64); ok {
		id = int64(v)
	}
	if v, ok := claims[jwt.ClaimKeyUsername].(string); ok {
		name = v
	}
	return id, name
}

// bodyAccountName 从请求体提取审计操作人标识（公开接口无 JWT，操作人在请求体）。
// 优先 account_name（登录/注册），其次 app_id（应用换 Token）。
func bodyAccountName(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m struct {
		AccountName string `json:"account_name"`
		AppID       string `json:"app_id"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	if m.AccountName != "" {
		return m.AccountName
	}
	return m.AppID
}

// buildAuditDetail 构建人类可读的操作详情（如 "POST /accounts"）。
func buildAuditDetail(method, path string) string {
	return method + " " + path
}

// readBody 读取并还原请求体，供公开接口提取操作人（登录/注册/换 Token 无 JWT）。
// 读取后还原 Body，保证下游 handler 仍能正常解析。
func readBody(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	return b
}

// auditRecorder 包装 ResponseWriter，捕获状态码与响应体（用于提取业务错误信息）。
type auditRecorder struct {
	http.ResponseWriter
	status    int
	body      bytes.Buffer
	truncated bool // 响应体超出缓存上限被截断，无法可靠解析业务 code
}

func newAuditRecorder(w http.ResponseWriter) *auditRecorder {
	return &auditRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *auditRecorder) WriteHeader(code int) {
	if r.status == http.StatusOK {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *auditRecorder) Write(b []byte) (int, error) {
	// 限制缓存，避免大响应拖慢审计；超出部分标记截断（不再缓存）
	if !r.truncated {
		remaining := 4096 - r.body.Len()
		if remaining > 0 {
			if len(b) > remaining {
				r.body.Write(b[:remaining])
				r.truncated = true
			} else {
				r.body.Write(b)
			}
		} else {
			r.truncated = true
		}
	}
	return r.ResponseWriter.Write(b)
}

// auditResult 判定操作成功与否并提取错误信息。
// 优先解析响应体 JSON 的 code/msg：
//   - HTTP 状态码非 2xx → 失败（msg 可能为空）
//   - 响应体 {code: 非0} → 业务失败（记录 msg）
//   - 否则 → 成功
func (r *auditRecorder) auditResult() (success bool, errMsg string) {
	// 非 2xx 状态码（认证失败、权限拒绝等）
	if r.status < 200 || r.status >= 300 {
		return false, r.parseErrorMsg()
	}

	// 响应体超出缓存上限被截断：body 不是完整 JSON，无法可靠解析业务 code。
	// （业务错误响应通常很小不会被截断；截断多为大 data 的成功响应，
	//  按 HTTP 状态判定成功，避免截断导致的"解析失败→误判成功/失败"。）
	if r.truncated {
		return true, ""
	}

	data := r.body.Bytes()
	if len(data) == 0 {
		return true, ""
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return true, ""
	}
	if resp.Code != 0 {
		return false, resp.Msg
	}
	return true, ""
}

// parseErrorMsg 解析响应体中的 msg 字段。
func (r *auditRecorder) parseErrorMsg() string {
	data := r.body.Bytes()
	if len(data) == 0 {
		return ""
	}
	var resp struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return ""
	}
	return resp.Msg
}
