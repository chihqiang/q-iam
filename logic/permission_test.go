package logic

import (
	"slices"
	"testing"

	"chihqiang/q-iam/model"
)

// TestMatchResource 验证资源匹配语义（空/* 通配、无资源上下文保守排除、层级通配）。
func TestMatchResource(t *testing.T) {
	cases := []struct {
		pattern  string
		resource string
		want     bool
	}{
		{"", "anything", true},                // 空 pattern 视为全匹配
		{"*", "anything", true},               // * 全匹配
		{"deptA:*", "", false},                // 无资源上下文 + 已限定资源 → 保守不匹配
		{"deptA:*", "deptA:account:1", true},  // 层级通配命中
		{"deptA:*", "deptB:account:1", false}, // 其他资源不命中
		{"deptA:account:1", "deptA:account:1", true},
		{"deptA:account:1", "deptA:account:2", false},
		{"deptA:account:*", "deptA:account:2", true},
		{"  *  ", "x", true}, // 空白容忍
	}
	for _, c := range cases {
		if got := matchResource(c.pattern, c.resource); got != c.want {
			t.Errorf("matchResource(%q, %q) = %v, want %v", c.pattern, c.resource, got, c.want)
		}
	}
}

// TestPermissionSetCheckResource 验证带资源维度的权限判定语义：
//   - Check（无资源上下文）只认空/* 资源规则；
//   - CheckResource 按资源匹配，显式 Deny 优先。
func TestPermissionSetCheckResource(t *testing.T) {
	ps := &PermissionSet{rules: []PermissionRule{
		{Effect: "Allow", Action: "iam:account:read", Resource: "*"},
		{Effect: "Allow", Action: "iam:account:write", Resource: "deptA:*"},
		{Effect: "Deny", Action: "iam:account:delete", Resource: "*"},
	}}

	// Check（无资源上下文）：仅全资源规则生效
	if !ps.Check("iam:account:read") {
		t.Error("Check: all-resource read should be allowed")
	}
	if ps.Check("iam:account:write") {
		t.Error("Check: resource-scoped write should be denied without resource context")
	}
	if ps.Check("iam:account:delete") {
		t.Error("Check: explicit deny should be denied")
	}

	// CheckResource（有资源上下文）
	if !ps.CheckResource("iam:account:read", "anything") {
		t.Error("CheckResource: all-resource read should be allowed")
	}
	if !ps.CheckResource("iam:account:write", "deptA:account:1") {
		t.Error("CheckResource: write within deptA should be allowed")
	}
	if ps.CheckResource("iam:account:write", "deptB:account:1") {
		t.Error("CheckResource: write outside deptA should be denied")
	}
	if ps.CheckResource("iam:account:delete", "deptA:x") {
		t.Error("CheckResource: explicit deny should win")
	}

	// 空权限集
	if (&PermissionSet{}).Check("iam:account:read") {
		t.Error("empty permission set should deny")
	}
	if (*PermissionSet)(nil).CheckResource("iam:account:read", "x") {
		t.Error("nil permission set should deny")
	}
}

// TestAggregateDataScopes 验证数据范围聚合语义（含 self 恒可见）：
//   - 无规则/受限 → self 恒可见；
//   - 未限定或 all → 全量；
//   - group → 指定组并集 + self；attribute → 降级 self。
func TestAggregateDataScopes(t *testing.T) {
	mkRule := func(action string, scopes ...model.DataScope) PermissionRule {
		return PermissionRule{Effect: "Allow", Action: action, DataScopes: scopes}
	}
	groupScope := func(id int64) model.DataScope {
		return model.DataScope{ScopeType: model.DataScopeGroup, GroupID: id}
	}

	cases := []struct {
		name   string
		rules  []PermissionRule
		action string
		want   ResourceDataScope
	}{
		{"nil 权限集 self 可见", nil, "iam:account:read", ResourceDataScope{SelfOnly: true}},
		{"空规则 self 可见", []PermissionRule{}, "iam:account:read", ResourceDataScope{SelfOnly: true}},
		{"未限定范围全量", []PermissionRule{mkRule("iam:account:read")}, "iam:account:read", ResourceDataScope{All: true}},
		{"all 范围全量", []PermissionRule{mkRule("iam:account:read", model.DataScope{ScopeType: model.DataScopeAll})}, "iam:account:read", ResourceDataScope{All: true}},
		{"self 范围仅本人", []PermissionRule{mkRule("iam:account:read", model.DataScope{ScopeType: model.DataScopeSelf})}, "iam:account:read", ResourceDataScope{SelfOnly: true}},
		{"group 范围 = 组并集 + self", []PermissionRule{mkRule("iam:account:read", groupScope(5), groupScope(7))}, "iam:account:read", ResourceDataScope{SelfOnly: true, GroupIDs: []int64{5, 7}}},
		{"attribute 降级 self", []PermissionRule{mkRule("iam:account:read", model.DataScope{ScopeType: model.DataScopeAttribute, AttrKey: "k"})}, "iam:account:read", ResourceDataScope{SelfOnly: true}},
		{"action 不匹配忽略", []PermissionRule{mkRule("iam:group:read", groupScope(5))}, "iam:account:read", ResourceDataScope{SelfOnly: true}},
		{"self + group 并集", []PermissionRule{
			mkRule("iam:account:read", model.DataScope{ScopeType: model.DataScopeSelf}),
			mkRule("iam:account:read", groupScope(9)),
		}, "iam:account:read", ResourceDataScope{SelfOnly: true, GroupIDs: []int64{9}}},
	}
	for _, c := range cases {
		ps := &PermissionSet{rules: c.rules}
		got := aggregateDataScopes(ps, c.action)
		if got.All != c.want.All || got.SelfOnly != c.want.SelfOnly {
			t.Errorf("%s: aggregate = {All:%v SelfOnly:%v GroupIDs:%v}, want {All:%v SelfOnly:%v GroupIDs:%v}",
				c.name, got.All, got.SelfOnly, got.GroupIDs, c.want.All, c.want.SelfOnly, c.want.GroupIDs)
			continue
		}
		if !slices.Equal(got.GroupIDs, c.want.GroupIDs) {
			t.Errorf("%s: GroupIDs = %v, want %v", c.name, got.GroupIDs, c.want.GroupIDs)
		}
	}
}
