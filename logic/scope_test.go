package logic

import (
	"testing"

	"chihqiang/q-iam/model"
)

// TestScopeAllowsAction 验证 scope 到权限动作的匹配语义。
func TestScopeAllowsAction(t *testing.T) {
	cases := []struct {
		name   string
		scope  string
		action string
		want   bool
	}{
		{"empty scope allows all", "", "iam:account:read", true},
		{"star scope allows all", "*", "iam:account:read", true},
		{"exact match", "iam:account:read", "iam:account:read", true},
		{"read capability matches read actions", "iam:read", "iam:account:read", true},
		{"read capability matches group read", "iam:read", "iam:group:read", true},
		{"read capability does not match write", "iam:read", "iam:account:write", false},
		{"write capability covers module", "iam:write", "iam:account:write", true},
		{"write capability covers read too", "iam:write", "iam:account:read", true},
		{"module:resource prefix", "iam:account", "iam:account:write", true},
		{"module:resource excludes other module", "iam:account", "iam:group:read", false},
		{"plain module prefix", "iam", "iam:account:read", true},
		{"plain module excludes other", "iam", "other:read", false},
		{"multiple scopes any match", "profile email iam:read", "iam:audit:read", true},
		{"multiple scopes no match", "profile email", "iam:account:write", false},
		{"glob wildcard", "iam:*", "iam:account:write", true},
		{"glob module read", "iam:*:read", "iam:account:read", true},
		{"glob no match", "iam:*:read", "iam:account:write", false},
	}
	for _, c := range cases {
		if got := scopeAllowsAction(c.scope, c.action); got != c.want {
			t.Errorf("%s: scopeAllowsAction(%q, %q) = %v, want %v", c.name, c.scope, c.action, got, c.want)
		}
	}
}

// TestFilterPermissionsByScope 验证权限语句按 scope 过滤。
func TestFilterPermissionsByScope(t *testing.T) {
	perms := []PermissionStatement{
		{Effect: model.EffectAllow, Action: "iam:account:read", Source: "p1"},
		{Effect: model.EffectAllow, Action: "iam:account:write", Source: "p2"},
		{Effect: model.EffectAllow, Action: "iam:audit:read", Source: "p3"},
	}

	// 空 scope：原样返回
	if got := filterPermissionsByScope(perms, ""); len(got) != 3 {
		t.Fatalf("empty scope should keep all, got %d", len(got))
	}
	// iam:read：只留 read 动作
	got := filterPermissionsByScope(perms, "iam:read")
	if len(got) != 2 {
		t.Fatalf("iam:read should keep 2, got %d", len(got))
	}
	for _, p := range got {
		if !scopeAllowsAction("iam:read", p.Action) {
			t.Fatalf("filtered item %s not allowed by scope", p.Action)
		}
	}
	// iam:write：全留
	if got := filterPermissionsByScope(perms, "iam:write"); len(got) != 3 {
		t.Fatalf("iam:write should keep all, got %d", len(got))
	}
	// 无匹配 scope：空
	if got := filterPermissionsByScope(perms, "email profile"); len(got) != 0 {
		t.Fatalf("unrelated scope should keep none, got %d", len(got))
	}
}

// TestScopeAllowsField 验证用户信息字段按 scope 裁剪。
func TestScopeAllowsField(t *testing.T) {
	cases := []struct {
		scope string
		field string
		want  bool
	}{
		{"", "email", true}, // 空 scope 全放行
		{"openid profile", "profile", true},
		{"openid profile", "email", false},
		{"openid email", "email", true},
		{"openid phone", "phone", true},
		{"openid phone", "mobile", false},
	}
	for _, c := range cases {
		if got := scopeAllowsField(c.scope, c.field); got != c.want {
			t.Errorf("scopeAllowsField(%q, %q) = %v, want %v", c.scope, c.field, got, c.want)
		}
	}
}
