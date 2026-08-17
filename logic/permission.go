package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"chihqiang/q-iam/logic/store"
	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
)

// PermissionRule 一条权限规则（由策略 Statement 解析而来）。
type PermissionRule struct {
	// Effect 效果：Allow | Deny。
	Effect string
	// Action 操作（逗号分隔，支持 * 通配）。
	Action string
	// Resource 资源（支持 * 通配，空或 * 表示全部资源）。
	// 仅参与 CheckResource 的资源维度判定；Check（无资源上下文）只认空/* 规则。
	Resource string
	// Source 来源策略名（对外展示用，不参与权限判定）。
	Source string
	// DataScopes 数据范围（数据权限：可见/操作哪部分数据，可为空=不限制）。
	DataScopes []model.DataScope
}

// PermissionSet 账号的权限集合（预构建，避免每次请求重复查库解析）。
type PermissionSet struct {
	rules []PermissionRule
}

// HasRule 是否包含任意规则。
func (ps *PermissionSet) HasRule() bool {
	return ps != nil && len(ps.rules) > 0
}

// Check 判定某个操作是否被允许（无资源上下文）。
// 等价于 CheckResource(action, "")：仅 Resource 为空或 * 的规则参与判定，
// 带资源限定的规则须由带资源上下文的判定入口生效（见 CheckResource）。
func (ps *PermissionSet) Check(action string) bool {
	return ps.CheckResource(action, "")
}

// CheckResource 判定某个操作在指定资源上是否被允许。
// 判定语义：显式 Deny 优先；有 Allow 匹配且无 Deny 匹配 → 允许；否则拒绝。
// resource 为空时视为无资源上下文，仅 Resource 为空或 * 的规则参与。
func (ps *PermissionSet) CheckResource(action, resource string) bool {
	if ps == nil {
		return false
	}

	allowed := false
	for _, rule := range ps.rules {
		if !matchAction(rule.Action, action) {
			continue
		}
		// 资源维度匹配（空资源上下文时仅空/* 规则通过，保守排除带资源限定规则）
		if !matchResource(rule.Resource, resource) {
			continue
		}

		// 显式拒绝优先；仅显式 Allow 计入允许，避免存量脏数据（非 Deny 的非法值）被当作放行
		effect := strings.TrimSpace(rule.Effect)
		if strings.EqualFold(effect, "Deny") {
			return false
		}
		if strings.EqualFold(effect, "Allow") {
			allowed = true
		}
	}
	return allowed
}

// PermissionLogic 权限加载与判定。
type PermissionLogic struct {
	db    *gorm.DB
	store store.KVStore // 权限集缓存后端（注入 store.KVStore 接口，nil 表示不缓存）
}

// NewPermissionLogic 创建权限逻辑。
func NewPermissionLogic(db *gorm.DB) *PermissionLogic {
	return &PermissionLogic{db: db}
}

// SetStore 注入权限集缓存后端（实现 store.KVStore 接口，如 RedisStore / DBStore / 自定义）。
// nil 表示不缓存。权限集缓存短 TTL（permCacheTTL），授权变更通过 InvalidatePermissionCache 主动失效。
func (s *PermissionLogic) SetStore(st store.KVStore) {
	s.store = st
}

// permCacheTTL 权限集缓存有效期。授权变更会主动失效，TTL 作为兜底。
const permCacheTTL = 60 * time.Second

// permKeyFor 生成权限集缓存键。
func permKeyFor(pt model.PrincipalType, principalID int64) string {
	prefix := string(pt)
	if pt == model.PrincipalTypeAccount {
		prefix = "acct"
	}
	return fmt.Sprintf("perm:%s:%d", prefix, principalID)
}

// InvalidatePermissionCache 使某主体的权限缓存失效（授权变更后调用）。
// group 授权变更会连带失效组内全部账号的缓存（组权限会影响组内账号）。
func (s *PermissionLogic) InvalidatePermissionCache(ctx context.Context, pt model.PrincipalType, principalID int64) {
	if s.store == nil {
		return
	}
	keys := []string{permKeyFor(pt, principalID)}
	if pt == model.PrincipalTypeGroup {
		var group model.Group
		if err := s.db.WithContext(ctx).Preload("Accounts").First(&group, principalID).Error; err == nil {
			for _, a := range group.Accounts {
				keys = append(keys, permKeyFor(model.PrincipalTypeAccount, a.ID))
			}
		}
	}
	if err := s.store.Del(ctx, keys...); err != nil {
		logger.WarnCtx(ctx, "invalidate permission cache failed", logger.Err(err))
	}
}

// rulesFromCache 读取缓存的权限规则，未命中返回 false。
func (s *PermissionLogic) rulesFromCache(ctx context.Context, key string) ([]PermissionRule, bool) {
	if s.store == nil {
		return nil, false
	}
	data, err := s.store.Get(ctx, key)
	if err != nil || data == "" {
		return nil, false
	}
	var rules []PermissionRule
	if err := json.Unmarshal([]byte(data), &rules); err != nil {
		logger.WarnCtx(ctx, "permission cache unmarshal failed", logger.Err(err))
		return nil, false
	}
	return rules, true
}

// rulesToCache 写入权限规则缓存（失败仅告警，不影响主流程）。
func (s *PermissionLogic) rulesToCache(ctx context.Context, key string, rules []PermissionRule) {
	if s.store == nil {
		return
	}
	data, err := json.Marshal(rules)
	if err != nil {
		return
	}
	if err := s.store.Set(ctx, key, string(data), permCacheTTL); err != nil {
		logger.WarnCtx(ctx, "permission cache set failed", logger.Err(err))
	}
}

// LoadPermissionSet 加载账号的权限集合。
// 权限来源：直接绑定给账号的策略 + 所属账号组绑定的策略。
func (s *PermissionLogic) LoadPermissionSet(ctx context.Context, accountID int64) (*PermissionSet, error) {
	// 优先命中缓存（权限集缓存 TTL + 授权变更主动失效）
	key := permKeyFor(model.PrincipalTypeAccount, accountID)
	if rules, ok := s.rulesFromCache(ctx, key); ok {
		return &PermissionSet{rules: rules}, nil
	}

	// 查询账号（含所属账号组，用于收集组绑定的策略）
	var account model.Account
	if err := s.db.WithContext(ctx).Preload("Groups").First(&account, accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &PermissionSet{}, nil
		}
		return nil, err
	}

	// 收集主体：账号自身 + 所属账号组
	groupIDs := make([]int64, 0, len(account.Groups))
	for _, g := range account.Groups {
		if g.Status {
			groupIDs = append(groupIDs, g.ID)
		}
	}

	// 查询所有授权关系对应的策略（直接 + 组）
	// 注意：必须 Preload Statements.Scopes，否则 p.Statements 为空，权限规则永远构建不出来。
	var policies []model.Policy
	policyTable := model.Policy{}.TableName()
	attachTable := model.PolicyAttachment{}.TableName()
	query := s.db.WithContext(ctx).
		Distinct(policyTable+".*").
		Joins("JOIN "+attachTable+" pa ON pa.policy_id = "+policyTable+".id").
		Preload("Statements.Scopes").
		Where(policyTable+".status = ?", true)

	conds := []string{"pa.principal_type = ? AND pa.principal_id = ?"}
	args := []any{model.PrincipalTypeAccount, account.ID}
	if len(groupIDs) > 0 {
		conds = append(conds, "pa.principal_type = ? AND pa.principal_id IN ?")
		args = append(args, model.PrincipalTypeGroup, groupIDs)
	}
	if err := query.Where(strings.Join(conds, " OR "), args...).Find(&policies).Error; err != nil {
		logger.ErrorCtx(ctx, "load permission set failed", logger.Err(err), logger.Int64("account_id", accountID))
		return nil, err
	}

	ps := &PermissionSet{}
	for _, p := range policies {
		for _, st := range p.Statements {
			ps.rules = append(ps.rules, PermissionRule{
				Effect:     st.Effect,
				Action:     st.Action,
				Source:     p.Name,
				DataScopes: st.Scopes,
			})
		}
	}
	s.rulesToCache(ctx, key, ps.rules)
	return ps, nil
}

// LoadPermissionSetByPrincipal 加载指定主体（app/group 等）的权限集合。
// 用于角色扮演令牌的权限判定。
func (s *PermissionLogic) LoadPermissionSetByPrincipal(ctx context.Context, pt model.PrincipalType, principalID int64) (*PermissionSet, error) {
	if !pt.Valid() {
		return &PermissionSet{}, nil
	}
	key := permKeyFor(pt, principalID)
	if rules, ok := s.rulesFromCache(ctx, key); ok {
		return &PermissionSet{rules: rules}, nil
	}

	var policies []model.Policy
	policyTable := model.Policy{}.TableName()
	attachTable := model.PolicyAttachment{}.TableName()
	err := s.db.WithContext(ctx).
		Distinct(policyTable+".*").
		Joins("JOIN "+attachTable+" pa ON pa.policy_id = "+policyTable+".id").
		Preload("Statements.Scopes").
		Where(policyTable+".status = ? AND pa.principal_type = ? AND pa.principal_id = ?",
			true, pt, principalID).
		Find(&policies).Error
	if err != nil {
		logger.ErrorCtx(ctx, "load permission set by principal failed",
			logger.Err(err),
			logger.String("principal_type", pt.String()),
			logger.Int64("principal_id", principalID))
		return nil, err
	}

	ps := &PermissionSet{}
	for _, p := range policies {
		for _, st := range p.Statements {
			ps.rules = append(ps.rules, PermissionRule{
				Effect:     st.Effect,
				Action:     st.Action,
				Source:     p.Name,
				DataScopes: st.Scopes,
			})
		}
	}
	s.rulesToCache(ctx, key, ps.rules)
	return ps, nil
}

// PermissionStatement 可读权限条目（供 UserInfo 等对外展示）。
type PermissionStatement struct {
	// Effect 效果：Allow | Deny。
	Effect string `json:"effect"`
	// Action 操作。
	Action string `json:"action"`
	// Resource 资源（支持 * 通配，空或 * 表示全部资源）。
	Resource string `json:"resource,omitempty"`
	// Source 来源策略名。
	Source string `json:"source,omitempty"`
	// DataScopes 数据范围（数据权限：可见/操作哪部分数据）。
	DataScopes []model.DataScope `json:"data_scopes,omitempty"`
}

// permissionStatementsFromSet 将权限集合转换为可读权限条目（含来源策略名）。
// 与 LoadPermissionSet 共用缓存：/auth/me、UserInfo、DataPermissions 等对外展示
// 路径复用权限集缓存，避免每次全量查库解析。
func permissionStatementsFromSet(ps *PermissionSet) []PermissionStatement {
	if ps == nil {
		return nil
	}
	items := make([]PermissionStatement, 0, len(ps.rules))
	for _, r := range ps.rules {
		items = append(items, PermissionStatement{
			Effect:     r.Effect,
			Action:     r.Action,
			Resource:   r.Resource,
			Source:     r.Source,
			DataScopes: r.DataScopes,
		})
	}
	return items
}

// LoadPermissionStatements 返回账号生效的权限规则列表（含来源策略名）。
// 权限来源：直接绑定给账号的策略 + 所属账号组绑定的策略。
// 复用 LoadPermissionSet 的权限集缓存（TTL + 授权变更主动失效）。
func (s *PermissionLogic) LoadPermissionStatements(ctx context.Context, accountID int64) ([]PermissionStatement, error) {
	ps, err := s.LoadPermissionSet(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return permissionStatementsFromSet(ps), nil
}

// LoadPermissionStatementsByPrincipal 返回指定主体（app/group 等非账号主体）生效的权限规则列表（含来源策略名）。
// 与 LoadPermissionStatements 的区别：后者面向账号（账号 + 所属账号组），本方法按单一主体类型查询。
// 复用 LoadPermissionSetByPrincipal 的权限集缓存。
func (s *PermissionLogic) LoadPermissionStatementsByPrincipal(ctx context.Context, principalType model.PrincipalType, principalID int64) ([]PermissionStatement, error) {
	ps, err := s.LoadPermissionSetByPrincipal(ctx, principalType, principalID)
	if err != nil {
		return nil, err
	}
	return permissionStatementsFromSet(ps), nil
}

// ResourceDataScope 某资源的可见数据范围（由当前账号对指定 action 的授权规则
// data_scopes 聚合而来），供管理列表/下拉/详情按数据权限过滤。
type ResourceDataScope struct {
	// All 全部可见（任一规则未限定数据范围或 scope_type=all）。
	All bool
	// SelfOnly 仅本人数据可见（scope_type=self）。
	SelfOnly bool
	// GroupIDs 可见的数据所属账号组 ID（scope_type=group，多行=多组并集）。
	GroupIDs []int64
}

// merge 合并一条数据范围到聚合结果。
func (a *ResourceDataScope) merge(d model.DataScope) {
	switch d.ScopeType {
	case model.DataScopeAll:
		a.All = true
	case model.DataScopeSelf:
		a.SelfOnly = true
	case model.DataScopeGroup:
		if d.GroupID > 0 {
			a.GroupIDs = append(a.GroupIDs, d.GroupID)
		}
	case model.DataScopeAttribute:
		// 管理资源无属性维度，保守降级为仅本人可见（避免越权）
		a.SelfOnly = true
	}
}

// DataScopeForAction 计算某账号对指定 action 资源（如 iam:account:read）的可见数据范围。
// 基于账号对该 action 的授权规则（复用权限集缓存，含直接 + 组绑定策略）：
//   - 任一规则无 data_scopes（未限定）或 scope_type=all → 全部可见；
//   - scope_type=self → 仅本人；
//   - scope_type=group → 指定组的数据可见（与 self 并集）；
//   - scope_type=attribute → 保守降级为仅本人；
//   - 数据范围受限时（非 all）self 恒可见（与 group/attribute 取并集）。
//
// 返回 All=true 时调用方无需过滤；否则按 SelfOnly/GroupIDs 构造过滤条件。
func (s *PermissionLogic) DataScopeForAction(ctx context.Context, action string, accountID int64) (ResourceDataScope, error) {
	ps, err := s.LoadPermissionSet(ctx, accountID)
	if err != nil {
		return ResourceDataScope{}, err
	}
	return aggregateDataScopes(ps, action), nil
}

// aggregateDataScopes 从权限集合聚合某 action 的可见数据范围（纯内存，便于单元测试）。
// 语义：
//   - 任一匹配规则未限定数据范围或含 scope_type=all → All=true（全量）；
//   - 否则按各规则 data_scopes 合并（group 多组并集）；attribute 保守降级为 self；
//   - 受限时（非 all）self 恒可见：即使只授予组级数据权限，也能查看自己的数据/资源。
func aggregateDataScopes(ps *PermissionSet, action string) ResourceDataScope {
	var out ResourceDataScope
	if ps == nil {
		out.SelfOnly = true
		return out
	}
	for _, rule := range ps.rules {
		if !matchAction(rule.Action, action) {
			continue
		}
		// 规则未限定数据范围：视为全部可见
		if len(rule.DataScopes) == 0 {
			out.All = true
			continue
		}
		for _, d := range rule.DataScopes {
			out.merge(d)
			if out.All {
				break
			}
		}
	}
	if out.All {
		return ResourceDataScope{All: true}
	}
	// 数据范围受限时 self 恒可见（与 group/attribute 取并集）
	out.SelfOnly = true
	return out
}

// AccountDataScope 计算某账号对账号资源（iam:account:read）的可见数据范围（便捷封装）。
func (s *PermissionLogic) AccountDataScope(ctx context.Context, accountID int64) (ResourceDataScope, error) {
	return s.DataScopeForAction(ctx, "iam:account:read", accountID)
}

// CanAccessPrincipal 判断账号 viewerID 对指定主体（account/group/app）是否可见。
// 基于 viewerID 对对应资源（iam:account:read / iam:group:read / iam:app:read）的数据范围：
//   - viewerID<=0（admin/系统主体）→ 可见；
//   - 数据范围 all/未限定 → 可见；
//   - 其余按资源语义：account 自见恒可见 + 属于可见组；group 命中可见组；app 归属者属于可见组。
//
// 供授权管理（grants）列表与各资源详情接口按数据范围过滤，防止越权查看/管理不可见主体。
func (s *PermissionLogic) CanAccessPrincipal(ctx context.Context, viewerID int64, pt model.PrincipalType, principalID int64) (bool, error) {
	if viewerID <= 0 {
		return true, nil
	}
	var action string
	switch pt {
	case model.PrincipalTypeAccount:
		action = "iam:account:read"
	case model.PrincipalTypeGroup:
		action = "iam:group:read"
	case model.PrincipalTypeApp:
		action = "iam:app:read"
	default:
		return false, errors.New("无效的主体类型")
	}
	scope, err := s.DataScopeForAction(ctx, action, viewerID)
	if err != nil {
		return false, err
	}
	if scope.All {
		return true, nil
	}
	switch pt {
	case model.PrincipalTypeAccount:
		if principalID == viewerID {
			return true, nil // self 恒可见
		}
		if len(scope.GroupIDs) > 0 {
			var n int64
			acctTable := model.Account{}.TableName()
			if err := s.db.WithContext(ctx).Model(&model.Account{}).
				Where(acctTable+".id = ?", principalID).
				Where("EXISTS (SELECT 1 FROM q_iam_account_groups ag WHERE ag.account_id = "+acctTable+".id AND ag.group_id IN ?)", scope.GroupIDs).
				Count(&n).Error; err != nil {
				return false, err
			}
			return n > 0, nil
		}
		return false, nil
	case model.PrincipalTypeGroup:
		for _, gid := range scope.GroupIDs {
			if gid == principalID {
				return true, nil
			}
		}
		return false, nil
	case model.PrincipalTypeApp:
		if len(scope.GroupIDs) > 0 {
			var n int64
			appTable := model.App{}.TableName()
			if err := s.db.WithContext(ctx).Model(&model.App{}).
				Where(appTable+".id = ?", principalID).
				Where("EXISTS (SELECT 1 FROM q_iam_account_groups ag WHERE ag.account_id = "+appTable+".owner_account_id AND ag.group_id IN ?)", scope.GroupIDs).
				Count(&n).Error; err != nil {
				return false, err
			}
			return n > 0, nil
		}
		return false, nil
	}
	return false, nil
}

// CanAccessPolicy 判断账号 viewerID 对指定策略是否可见（iam:policy:read 数据范围）：
//   - viewerID<=0（admin/系统主体）→ 可见；
//   - 数据范围 all/未限定 → 可见；
//   - self → 仅本人创建（created_by=viewerID）；
//   - group → 绑定到可见组的策略。
//
// 供授权管理（grants）列表与策略详情接口按数据范围过滤。
func (s *PermissionLogic) CanAccessPolicy(ctx context.Context, viewerID, policyID int64) (bool, error) {
	if viewerID <= 0 {
		return true, nil
	}
	scope, err := s.DataScopeForAction(ctx, "iam:policy:read", viewerID)
	if err != nil {
		return false, err
	}
	if scope.All {
		return true, nil
	}
	if scope.SelfOnly {
		var n int64
		if err := s.db.WithContext(ctx).Model(&model.Policy{}).
			Where("id = ? AND created_by = ?", policyID, viewerID).Count(&n).Error; err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	if len(scope.GroupIDs) > 0 {
		var n int64
		if err := s.db.WithContext(ctx).Model(&model.PolicyAttachment{}).
			Where("policy_id = ? AND principal_type = ? AND principal_id IN ?", policyID, model.PrincipalTypeGroup, scope.GroupIDs).
			Count(&n).Error; err != nil {
			return false, err
		}
		return n > 0, nil
	}
	return false, nil
}

// --- 匹配辅助 ---

// matchAction 匹配操作（逗号分隔，支持 * 通配）。
func matchAction(pattern, action string) bool {
	return matchList(pattern, action)
}

// matchResource 资源匹配。
//   - pattern 为空或 "*" 视为全匹配（未限定资源）；
//   - 无资源上下文（resource 为空）时，限定过资源的 pattern 不匹配（保守排除）；
//   - 其余按 * 通配匹配（复用 globMatch，支持 `deptA:*` 等层级通配）。
func matchResource(pattern, resource string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	if resource == "" {
		return false
	}
	return globMatch(pattern, resource)
}

// matchList 匹配逗号分隔列表：pattern 中任一项与 value 匹配即命中。
func matchList(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	for _, p := range strings.Split(pattern, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if globMatch(p, value) {
			return true
		}
	}
	return false
}

// globMatch 简单通配匹配：* 匹配任意字符序列。
func globMatch(pattern, s string) bool {
	// 无通配符时精确匹配
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}

	// 拆分 * 分段
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}

	// 前缀匹配
	if parts[0] != "" && !strings.HasPrefix(s, parts[0]) {
		return false
	}
	// 后缀匹配
	last := parts[len(parts)-1]
	if last != "" && !strings.HasSuffix(s, last) {
		return false
	}
	// 中间部分依次查找
	pos := len(parts[0])
	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		idx := strings.Index(s[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	return true
}
