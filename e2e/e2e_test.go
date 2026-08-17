// Package e2e 端到端冒烟测试。
//
// 以 go test 方式启动**完整服务**（svc.NewServiceContext + route.Register + httptest，
// 内存 SQLite + 种子数据），通过真实 HTTP 接口验证核心链路，替代早期的 shell 脚本：
//
//	P1  账号列表按数据范围（DataScope）过滤（admin 全量 / 普通账号仅本人）
//	P2  账号详情数据范围可见性（防列表外 ID 枚举越权）
//	P3  登出后 access token 立即失效（撤销黑名单）
//	P4  刷新令牌重用连坐时间窗（第 1 次重放不连坐，窗口内第 2 次才连坐）
//	P5  策略 Resource 资源字段保存/回读
//	P6  审计日志落库
//	P7  账号组列表按数据范围过滤（self→仅本人所属组）
//	P8  策略列表按数据范围过滤（self→仅本人创建）
//	P9  应用列表按数据范围过滤（self→仅本人拥有）
//	P10 数据范围 group 语义：self 恒可见 + 组内并集
//	P11 授权管理(grants)列表按数据范围过滤
//	P12 超级管理员 is_admin 语义
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/route"
	"chihqiang/q-iam/svc"

	"github.com/chihqiang/infra-go/httpx"
)

// testEnv 端到端测试环境：完整服务地址 + 请求辅助。
type testEnv struct {
	base string // http://<host>/api/v1
}

// newTestServer 构造完整 q-iam 服务（内存 SQLite + 种子数据），返回 httptest 服务。
func newTestServer(t *testing.T) *testEnv {
	t.Helper()

	cfg := config.Config{}
	cfg.DB.Driver = "sqlite"
	cfg.DB.Database = "file::memory:?cache=shared"
	cfg.Migration.AutoMigrate = true
	cfg.Migration.SeedData = true
	cfg.JWT.Secret = "e2e-test-secret-key"
	cfg.JWT.AccessTokenExpire = time.Hour
	cfg.JWT.RefreshTokenExpire = 24 * time.Hour
	cfg.JWT.Algorithm = "HS256"

	ctx, err := svc.NewServiceContext(cfg)
	if err != nil {
		t.Fatalf("new service context: %v", err)
	}
	t.Cleanup(ctx.Close)

	server := httpx.NewServer(cfg.Server)
	route.Register(server, ctx)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	return &testEnv{base: ts.URL + "/api/v1"}
}

// apiResp 统一响应（{code, msg, data}）。
type apiResp struct {
	status int
	code   float64
	msg    string
	data   any
}

// do 发起请求并解析统一响应体。
func (e *testEnv) do(t *testing.T, method, path, token string, payload any) apiResp {
	t.Helper()

	var body []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = b
	}

	req, err := http.NewRequest(method, e.base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	code, _ := parsed["code"].(float64)
	msg, _ := parsed["msg"].(string)
	return apiResp{status: resp.StatusCode, code: code, msg: msg, data: parsed["data"]}
}

// login 登录并返回 access/refresh token（断言登录成功）。
func (e *testEnv) login(t *testing.T, name, pass string) (at, rt string) {
	t.Helper()
	r := e.do(t, http.MethodPost, "/auth/login", "", map[string]any{
		"account_name": name, "password": pass,
	})
	mustCode(t, r, 0)
	data := mustDataMap(t, r)
	return data["access_token"].(string), data["refresh_token"].(string)
}

// ---------- 断言辅助 ----------

func mustCode(t *testing.T, r apiResp, want float64) {
	t.Helper()
	if r.code != want {
		t.Fatalf("业务 code=%v want=%v (msg=%q, http=%d)", r.code, want, r.msg, r.status)
	}
}

func mustHTTP(t *testing.T, r apiResp, want int) {
	t.Helper()
	if r.status != want {
		t.Fatalf("HTTP %d want %d (body=%+v)", r.status, want, r.raw())
	}
}

func mustDataMap(t *testing.T, r apiResp) map[string]any {
	t.Helper()
	m, ok := r.data.(map[string]any)
	if !ok {
		t.Fatalf("data 不是对象: %#v", r.data)
	}
	return m
}

// dataMap 返回 data 中的对象（与 mustDataMap 等价，命名更简短便于行内断言）。
func dataMap(t *testing.T, r apiResp) map[string]any {
	t.Helper()
	return mustDataMap(t, r)
}

func mustDataList(t *testing.T, v any) []any {
	t.Helper()
	l, ok := v.([]any)
	if !ok {
		t.Fatalf("data 不是数组: %#v", v)
	}
	return l
}

// totalOf 返回分页响应 data.total。
func totalOf(t *testing.T, r apiResp) float64 {
	t.Helper()
	return dataMap(t, r)["total"].(float64)
}

// strField 取 map 中 string 字段。
func strField(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, _ := m[key].(string)
	return v
}

// contains 判断字符串切片是否包含某元素。
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// (apiResp).raw 供 mustHTTP 打印完整响应。
func (r apiResp) raw() string {
	b, _ := json.Marshal(map[string]any{"code": r.code, "msg": r.msg, "data": r.data})
	return string(b)
}

// ---------- 主流程 ----------

// TestEndToEndSmoke 端到端冒烟测试：按顺序覆盖 P1–P12（状态累积，需共享同一服务）。
func TestEndToEndSmoke(t *testing.T) {
	e := newTestServer(t)

	// ============ 准备：登录 admin、创建账号 ============
	t.Run("准备数据", func(t *testing.T) {
		if _, rt := e.login(t, "admin", "admin123"); rt == "" {
			t.Fatal("admin 登录无 refresh token")
		}
	})
	adminAT, _ := e.login(t, "admin", "admin123")
	for _, name := range []string{"user1", "user2"} {
		r := e.do(t, http.MethodPost, "/accounts", adminAT, map[string]any{
			"account_name": name, "password": "Passw0rd!", "allow_console": true,
		})
		mustCode(t, r, 0)
	}

	// 账号 ID
	accts := e.do(t, http.MethodGet, "/accounts/all", adminAT, nil)
	mustCode(t, accts, 0)
	u1, u2 := float64(0), float64(0)
	for _, item := range mustDataList(t, accts.data) {
		m := item.(map[string]any)
		switch m["account_name"] {
		case "user1":
			u1 = m["id"].(float64)
		case "user2":
			u2 = m["id"].(float64)
		}
	}
	if u1 == 0 || u2 == 0 {
		t.Fatal("未能定位 user1/user2 ID")
	}
	t.Logf("admin=%s user1=%.0f user2=%.0f", adminAT[:12], u1, u2)

	// ============ P5 策略 Resource 字段 ============
	var policyID float64
	t.Run("P5 策略 Resource 字段", func(t *testing.T) {
		r := e.do(t, http.MethodPost, "/policies", adminAT, map[string]any{
			"name": "ViewOwnAccounts", "description": "仅本人",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:account:read", "resource": "*",
				"scopes": []any{map[string]any{"scope_type": "self", "owner_field": "id", "sort": 0}},
				"sort":   0,
			}},
		})
		mustCode(t, r, 0)
		policyID = dataMap(t, r)["id"].(float64)

		detail := e.do(t, http.MethodGet, fmt.Sprintf("/policies/%.0f", policyID), adminAT, nil)
		mustCode(t, detail, 0)
		stmts := dataMap(t, detail)["statements"].([]any)
		stmt := stmts[0].(map[string]any)
		if res, _ := stmt["resource"].(string); res != "*" {
			t.Fatalf("resource 字段保存/回读异常: %q", res)
		}
	})

	// ============ P1 账号列表数据范围（self→仅本人）============
	var user1AT string
	t.Run("P1 账号列表按数据范围过滤", func(t *testing.T) {
		g := e.do(t, http.MethodPost, "/grants", adminAT, map[string]any{
			"principal_type": "account", "principal_id": u1, "policy_ids": []any{policyID},
		})
		mustCode(t, g, 0)

		// admin 全量
		adminList := e.do(t, http.MethodGet, "/accounts?page=1&size=10", adminAT, nil)
		mustCode(t, adminList, 0)
		if total := totalOf(t, adminList); total < 3 {
			t.Fatalf("admin 列表应全量(≥3), total=%v", total)
		}

		// user1 仅本人
		user1AT, _ = e.login(t, "user1", "Passw0rd!")
		u1List := e.do(t, http.MethodGet, "/accounts?page=1&size=10", user1AT, nil)
		mustCode(t, u1List, 0)
		if total := totalOf(t, u1List); total != 1 {
			t.Fatalf("user1 列表应仅本人, total=%v", total)
		}
	})

	// ============ P2 账号详情数据范围可见性 ============
	t.Run("P2 账号详情越权拦截", func(t *testing.T) {
		// 自己详情可见
		me := e.do(t, http.MethodGet, fmt.Sprintf("/accounts/%.0f", u1), user1AT, nil)
		mustCode(t, me, 0)
		// 他人详情 403
		forb := e.do(t, http.MethodGet, fmt.Sprintf("/accounts/%.0f", u2), user1AT, nil)
		mustHTTP(t, forb, http.StatusForbidden)
	})

	// ============ P3 登出后 access token 立即失效 ============
	t.Run("P3 登出后 access token 立即失效", func(t *testing.T) {
		at, rt := e.login(t, "user1", "Passw0rd!")
		lo := e.do(t, http.MethodPost, "/auth/logout", at, map[string]any{"refresh_token": rt})
		mustCode(t, lo, 0)
		after := e.do(t, http.MethodGet, "/auth/me", at, nil)
		mustHTTP(t, after, http.StatusUnauthorized)
	})

	// ============ P4 刷新令牌重用连坐时间窗 ============
	t.Run("P4 刷新令牌重用连坐时间窗", func(t *testing.T) {
		_, oldRT := e.login(t, "user2", "Passw0rd!")
		ref := e.do(t, http.MethodPost, "/auth/refresh", "", map[string]any{"refresh_token": oldRT})
		mustCode(t, ref, 0)
		newRT := dataMap(t, ref)["refresh_token"].(string)

		// 第 1 次重放：失败但新令牌仍有效（不连坐）
		r1 := e.do(t, http.MethodPost, "/auth/refresh", "", map[string]any{"refresh_token": oldRT})
		mustCode(t, r1, -1)
		nv := e.do(t, http.MethodPost, "/auth/refresh", "", map[string]any{"refresh_token": newRT})
		mustCode(t, nv, 0)

		// 第 2 次重放（时间窗内）：连坐吊销全部
		r2 := e.do(t, http.MethodPost, "/auth/refresh", "", map[string]any{"refresh_token": oldRT})
		mustCode(t, r2, -1)
		na := e.do(t, http.MethodPost, "/auth/refresh", "", map[string]any{"refresh_token": newRT})
		mustCode(t, na, -1)
	})

	// ============ P6 审计日志落库 ============
	t.Run("P6 审计日志落库", func(t *testing.T) {
		// 审计异步落库，轮询等待
		deadline := time.Now().Add(5 * time.Second)
		for {
			r := e.do(t, http.MethodGet, "/audit-logs?page=1&size=50", adminAT, nil)
			mustCode(t, r, 0)
			if total := totalOf(t, r); total >= 1 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("审计日志迟迟未落库")
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	// ============ P7 账号组列表数据范围（self→仅本人所属组）============
	var groupA float64
	t.Run("P7 账号组列表按数据范围过滤", func(t *testing.T) {
		ga := e.do(t, http.MethodPost, "/groups", adminAT, map[string]any{"name": "groupA"})
		mustCode(t, ga, 0)
		groupA = dataMap(t, ga)["id"].(float64)
		gb := e.do(t, http.MethodPost, "/groups", adminAT, map[string]any{"name": "groupB"})
		mustCode(t, gb, 0)
		groupB := dataMap(t, gb)["id"].(float64)

		mem := e.do(t, http.MethodPost, fmt.Sprintf("/groups/%.0f/members", groupA), adminAT,
			map[string]any{"account_ids": []any{u1}})
		mustCode(t, mem, 0)

		// ViewOwnGroups: iam:group:read + scope self
		pg := e.do(t, http.MethodPost, "/policies", adminAT, map[string]any{
			"name": "ViewOwnGroups", "description": "仅本人所属组",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:group:read", "resource": "*",
				"scopes": []any{map[string]any{"scope_type": "self", "owner_field": "id", "sort": 0}},
				"sort":   0,
			}},
		})
		mustCode(t, pg, 0)
		gg := e.do(t, http.MethodPost, "/grants", adminAT, map[string]any{
			"principal_type": "account", "principal_id": u1, "policy_ids": []any{dataMap(t, pg)["id"]},
		})
		mustCode(t, gg, 0)

		// user1 重新登录（P3 已吊销其 token）
		user1AT, _ = e.login(t, "user1", "Passw0rd!")
		u1Groups := e.do(t, http.MethodGet, "/groups?page=1&size=10", user1AT, nil)
		mustCode(t, u1Groups, 0)
		names := namesOf(t, mustDataList(t, dataMap(t, u1Groups)["data"]))
		if total := totalOf(t, u1Groups); total != 1 || !contains(names, "groupA") {
			t.Fatalf("user1 组列表应仅 groupA: total=%v names=%v", totalOf(t, u1Groups), names)
		}

		// 查看 groupB 详情 403
		forb := e.do(t, http.MethodGet, fmt.Sprintf("/groups/%.0f", groupB), user1AT, nil)
		mustHTTP(t, forb, http.StatusForbidden)
	})

	// ============ P8 策略列表数据范围（self→仅本人创建）============
	t.Run("P8 策略列表按数据范围过滤", func(t *testing.T) {
		// 给 user1 授 iam:policy:write + iam:policy:read(self)
		pw := e.do(t, http.MethodPost, "/policies", adminAT, map[string]any{
			"name": "WritePolicies",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:policy:write", "resource": "*", "sort": 0,
			}},
		})
		mustCode(t, pw, 0)
		pp := e.do(t, http.MethodPost, "/policies", adminAT, map[string]any{
			"name": "ViewOwnPolicies", "description": "仅本人创建的策略",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:policy:read", "resource": "*",
				"scopes": []any{map[string]any{"scope_type": "self", "owner_field": "id", "sort": 0}},
				"sort":   0,
			}},
		})
		mustCode(t, pp, 0)
		for _, pid := range []any{dataMap(t, pw)["id"], dataMap(t, pp)["id"]} {
			g := e.do(t, http.MethodPost, "/grants", adminAT, map[string]any{
				"principal_type": "account", "principal_id": u1, "policy_ids": []any{pid},
			})
			mustCode(t, g, 0)
		}

		// user1 创建自己的策略
		my := e.do(t, http.MethodPost, "/policies", user1AT, map[string]any{
			"name": "MyPolicy",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:account:read", "resource": "*", "sort": 0,
			}},
		})
		mustCode(t, my, 0)

		// user1 策略列表仅本人创建的 MyPolicy
		u1Pols := e.do(t, http.MethodGet, "/policies?page=1&size=50", user1AT, nil)
		mustCode(t, u1Pols, 0)
		pnames := namesOf(t, mustDataList(t, dataMap(t, u1Pols)["data"]))
		if total := totalOf(t, u1Pols); total != 1 || !contains(pnames, "MyPolicy") {
			t.Fatalf("user1 策略列表应仅 MyPolicy: total=%v names=%v", totalOf(t, u1Pols), pnames)
		}

		// 查看 admin 策略详情 403
		forb := e.do(t, http.MethodGet, fmt.Sprintf("/policies/%.0f", policyID), user1AT, nil)
		mustHTTP(t, forb, http.StatusForbidden)
	})

	// ============ P9 应用列表数据范围（self→仅本人拥有）============
	t.Run("P9 应用列表按数据范围过滤", func(t *testing.T) {
		app1 := e.do(t, http.MethodPost, "/apps", adminAT, map[string]any{
			"name": "AppOne", "grant_type": "client_credentials",
		})
		mustCode(t, app1, 0)
		app1ID := dataMap(t, app1)["id"].(float64)

		pw := e.do(t, http.MethodPost, "/policies", adminAT, map[string]any{
			"name": "WriteApps",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:app:write", "resource": "*", "sort": 0,
			}},
		})
		mustCode(t, pw, 0)
		pa := e.do(t, http.MethodPost, "/policies", adminAT, map[string]any{
			"name": "ViewOwnApps", "description": "仅本人拥有的应用",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:app:read", "resource": "*",
				"scopes": []any{map[string]any{"scope_type": "self", "owner_field": "id", "sort": 0}},
				"sort":   0,
			}},
		})
		mustCode(t, pa, 0)
		for _, pid := range []any{dataMap(t, pw)["id"], dataMap(t, pa)["id"]} {
			g := e.do(t, http.MethodPost, "/grants", adminAT, map[string]any{
				"principal_type": "account", "principal_id": u1, "policy_ids": []any{pid},
			})
			mustCode(t, g, 0)
		}

		// user1 创建自己的应用（显式 owner=u1）
		app2 := e.do(t, http.MethodPost, "/apps", user1AT, map[string]any{
			"name": "AppTwo", "grant_type": "client_credentials", "owner_account_id": u1,
		})
		mustCode(t, app2, 0)

		// user1 应用列表仅本人拥有的 AppTwo
		u1Apps := e.do(t, http.MethodGet, "/apps?page=1&size=50", user1AT, nil)
		mustCode(t, u1Apps, 0)
		anames := namesOf(t, mustDataList(t, dataMap(t, u1Apps)["data"]))
		if total := totalOf(t, u1Apps); total != 1 || !contains(anames, "AppTwo") {
			t.Fatalf("user1 应用列表应仅 AppTwo: total=%v names=%v", totalOf(t, u1Apps), anames)
		}

		// 查看 admin 应用详情 403
		forb := e.do(t, http.MethodGet, fmt.Sprintf("/apps/%.0f", app1ID), user1AT, nil)
		mustHTTP(t, forb, http.StatusForbidden)
	})

	// ============ P10 数据范围 group 语义：self 恒可见 + 组内并集 ============
	t.Run("P10 group 语义：self 恒可见 + 组内并集", func(t *testing.T) {
		// user3 加入 groupA
		u3 := e.do(t, http.MethodPost, "/accounts", adminAT, map[string]any{
			"account_name": "user3", "password": "Passw0rd!", "allow_console": true,
		})
		mustCode(t, u3, 0)
		u3ID := dataMap(t, u3)["id"].(float64)
		mem := e.do(t, http.MethodPost, fmt.Sprintf("/groups/%.0f/members", groupA), adminAT,
			map[string]any{"account_ids": []any{u3ID}})
		mustCode(t, mem, 0)

		// ViewGroupAccounts: iam:account:read + scope group(groupA)
		pg := e.do(t, http.MethodPost, "/policies", adminAT, map[string]any{
			"name": "ViewGroupAccounts", "description": "查看 groupA 内账号",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:account:read", "resource": "*",
				"scopes": []any{map[string]any{"scope_type": "group", "group_id": groupA, "sort": 0}},
				"sort":   0,
			}},
		})
		mustCode(t, pg, 0)
		g := e.do(t, http.MethodPost, "/grants", adminAT, map[string]any{
			"principal_type": "account", "principal_id": u1, "policy_ids": []any{dataMap(t, pg)["id"]},
		})
		mustCode(t, g, 0)

		// user1 账号列表 = 本人 ∪ groupA 成员
		u1List := e.do(t, http.MethodGet, "/accounts?page=1&size=10", user1AT, nil)
		mustCode(t, u1List, 0)
		acctNames := namesOf(t, mustDataList(t, dataMap(t, u1List)["data"]))
		if total := totalOf(t, u1List); total != 2 || !contains(acctNames, "user1") || !contains(acctNames, "user3") {
			t.Fatalf("user1 账号列表应含本人+groupA 成员: total=%v names=%v", totalOf(t, u1List), acctNames)
		}
	})

	// ============ P11 授权管理(grants)列表数据范围 ============
	t.Run("P11 grants 列表按数据范围过滤", func(t *testing.T) {
		// 给 user1 授 iam:grant
		pg := e.do(t, http.MethodPost, "/policies", adminAT, map[string]any{
			"name": "GrantPolicy",
			"statements": []any{map[string]any{
				"effect": "Allow", "action": "iam:grant", "resource": "*", "sort": 0,
			}},
		})
		mustCode(t, pg, 0)
		g := e.do(t, http.MethodPost, "/grants", adminAT, map[string]any{
			"principal_type": "account", "principal_id": u1, "policy_ids": []any{dataMap(t, pg)["id"]},
		})
		mustCode(t, g, 0)

		// admin 把 ViewOwnAccounts 授权给 user2
		g2 := e.do(t, http.MethodPost, "/grants", adminAT, map[string]any{
			"principal_type": "account", "principal_id": u2, "policy_ids": []any{policyID},
		})
		mustCode(t, g2, 0)

		// user1 查 user2 授权 → 不可见主体 → 空
		v := e.do(t, http.MethodGet, fmt.Sprintf("/grants/principals/account/%.0f", u2), user1AT, nil)
		mustCode(t, v, 0)
		if n := len(mustDataList(t, v.data)); n != 0 {
			t.Fatalf("user1 查 user2 授权应为空, count=%d", n)
		}

		// user1 查自己授权 → 非空（self 恒可见）
		own := e.do(t, http.MethodGet, fmt.Sprintf("/grants/principals/account/%.0f", u1), user1AT, nil)
		mustCode(t, own, 0)
		if n := len(mustDataList(t, own.data)); n == 0 {
			t.Fatal("user1 查自己授权应非空")
		}

		// admin 查 user2 授权 → 全量可见
		ad := e.do(t, http.MethodGet, fmt.Sprintf("/grants/principals/account/%.0f", u2), adminAT, nil)
		mustCode(t, ad, 0)
		if n := len(mustDataList(t, ad.data)); n == 0 {
			t.Fatal("admin 查 user2 授权应非空")
		}
	})

	// ============ P12 超级管理员 is_admin 语义 ============
	t.Run("P12 is_admin 语义", func(t *testing.T) {
		detail := e.do(t, http.MethodGet, "/accounts/1", adminAT, nil)
		mustCode(t, detail, 0)
		if isAdmin, _ := dataMap(t, detail)["is_admin"].(bool); !isAdmin {
			t.Fatal("内置 admin 账号 is_admin 应为 true")
		}

		del := e.do(t, http.MethodDelete, "/accounts/1", adminAT, nil)
		if !strings.Contains(del.msg, "不能删除内置管理员") {
			t.Fatalf("删除 admin 应被拒绝, msg=%q", del.msg)
		}

		// user1 看不到 admin 的授权关系
		v := e.do(t, http.MethodGet, "/grants/principals/account/1", user1AT, nil)
		mustCode(t, v, 0)
		if n := len(mustDataList(t, v.data)); n != 0 {
			t.Fatalf("user1 看 admin 授权应为空, count=%d", n)
		}
	})
}

// namesOf 提取列表项显示名（账号列表用 account_name，其余用 name）。
func namesOf(t *testing.T, items []any) []string {
	t.Helper()
	out := make([]string, 0, len(items))
	for _, item := range items {
		m := item.(map[string]any)
		switch {
		case m["name"] != nil:
			if n, ok := m["name"].(string); ok {
				out = append(out, n)
			}
		case m["account_name"] != nil:
			if n, ok := m["account_name"].(string); ok {
				out = append(out, n)
			}
		}
	}
	return out
}
