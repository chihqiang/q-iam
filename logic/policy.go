package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
)

// PolicyLogic 权限策略管理逻辑：策略 CRUD 与授权规则（主从表）维护。
type PolicyLogic struct {
	db *gorm.DB
	// permLogic 权限逻辑（策略变更后失效引用该策略的主体的权限缓存，nil 表示不失效）。
	permLogic *PermissionLogic
}

// NewPolicyLogic 创建权限策略管理逻辑。
func NewPolicyLogic(db *gorm.DB) *PolicyLogic {
	return &PolicyLogic{db: db}
}

// SetPermissionLogic 注入权限逻辑（策略变更后失效权限缓存）。
func (s *PolicyLogic) SetPermissionLogic(permLogic *PermissionLogic) {
	s.permLogic = permLogic
}

// PolicyListRequest 策略列表请求。
type PolicyListRequest struct {
	PageRequest
	Type   string `form:"type"`
	Status *bool  `form:"status"`
	Key    string `form:"key"`
}

// List 策略分页列表（按数据权限过滤可见范围）。
// accountID<=0（admin/系统主体）不过滤；否则按该账号对 iam:policy:read 的
// 数据范围过滤（self=本人创建，group=绑定到可见组的策略）。
func (s *PolicyLogic) List(ctx context.Context, accountID int64, req *PolicyListRequest) (*PageResponse[model.Policy], error) {
	var policies []model.Policy
	var total int64

	query := s.db.WithContext(ctx).Model(&model.Policy{})
	if req.Type != "" {
		query = query.Where("type = ?", req.Type)
	}
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}
	if req.Key != "" {
		like := "%" + req.Key + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}

	// 数据范围过滤（非 admin 账号按 iam:policy:read 的数据范围可见性过滤）
	s.applyPolicyScope(ctx, query, accountID)

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (req.Page - 1) * req.Size
	if err := query.Order("id ASC").Offset(offset).Limit(req.Size).Find(&policies).Error; err != nil {
		return nil, err
	}

	return &PageResponse[model.Policy]{Data: policies, Total: total}, nil
}

// applyPolicyScope 按当前账号对策略资源（iam:policy:read）的数据范围过滤查询。
// accountID<=0 或权限逻辑未注入时不过滤（全量）。数据范围加载失败时保守降级为仅本人创建。
// 语义：
//   - all/未限定 → 全量；
//   - self → 仅本人创建的策略（created_by=本人）；
//   - group → 绑定到指定组的策略（q_iam_policy_attachments 中 principal_type=group）；
//   - attribute → 已降级为 self。
func (s *PolicyLogic) applyPolicyScope(ctx context.Context, query *gorm.DB, accountID int64) {
	if accountID <= 0 || s.permLogic == nil {
		return
	}
	policyTable := model.Policy{}.TableName() // q_iam_policies

	scope, err := s.permLogic.DataScopeForAction(ctx, "iam:policy:read", accountID)
	if err != nil {
		logger.WarnCtx(ctx, "load policy data scope failed, fallback to self",
			logger.Err(err), logger.Int64("account_id", accountID))
		query.Where(policyTable+".created_by = ?", accountID)
		return
	}
	if scope.All {
		return
	}

	conds := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if scope.SelfOnly {
		conds = append(conds, policyTable+".created_by = ?")
		args = append(args, accountID)
	}
	if len(scope.GroupIDs) > 0 {
		// 绑定到可见组的策略（组授权影响组内账号，策略可视为组的数据）
		conds = append(conds, "EXISTS (SELECT 1 FROM q_iam_policy_attachments pa WHERE pa.policy_id = "+policyTable+".id AND pa.principal_type = 'group' AND pa.principal_id IN ?)")
		args = append(args, scope.GroupIDs)
	}
	if len(conds) == 0 {
		query.Where("1 = 0")
		return
	}
	query.Where(strings.Join(conds, " OR "), args...)
}

// AllList 全部启用的策略（用于授权选择）。
// accountID<=0 返回全部；否则按数据范围过滤，防止下拉选择越权策略。
func (s *PolicyLogic) AllList(ctx context.Context, accountID int64) ([]model.Policy, error) {
	var policies []model.Policy
	query := s.db.WithContext(ctx).Where("status = ?", true).Order("id ASC")
	s.applyPolicyScope(ctx, query, accountID)
	err := query.Find(&policies).Error
	return policies, err
}

// CanViewPolicy 判断账号 viewerID 是否有权查看策略 targetID 的详情。
// 委托 PermissionLogic.CanAccessPolicy 基于 iam:policy:read 数据范围判定。
// 供详情接口做数据范围校验，防止「列表已过滤但详情接口按 ID 枚举绕过」越权。
// 权限逻辑未注入时保守拒绝（返回 false）。
func (s *PolicyLogic) CanViewPolicy(ctx context.Context, viewerID, targetID int64) (bool, error) {
	if s.permLogic == nil {
		return false, nil
	}
	return s.permLogic.CanAccessPolicy(ctx, viewerID, targetID)
}

// GetByID 策略详情（含授权规则主从表）。
func (s *PolicyLogic) GetByID(ctx context.Context, id int64) (*model.Policy, error) {
	var policy model.Policy
	if err := s.db.WithContext(ctx).
		Preload("Statements.Scopes").
		First(&policy, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("策略不存在")
		}
		return nil, err
	}
	return &policy, nil
}

// --- 策略 DTO ---

// PolicyStatementDTO 策略授权规则。
type PolicyStatementDTO struct {
	// Description 语句描述（小标题，说明本条授权规则的用途）。
	Description string `json:"description"`
	// Effect 效果：Allow | Deny。
	Effect string `json:"effect"`
	// Action 操作（逗号分隔）。
	Action string `json:"action"`
	// Resource 资源（支持 * 通配，默认 * 表示全部资源）。
	Resource string `json:"resource"`
	// Scopes 数据范围明细（数据权限）。
	Scopes []PolicyScopeDTO `json:"scopes"`
	// Sort 排序。
	Sort int64 `json:"sort"`
}

// PolicyScopeDTO 策略语句的数据范围。
// 类型：all（全部）/ group（本用户分组）/ self（本人数据）/ attribute（属性过滤）。
type PolicyScopeDTO struct {
	// ScopeType 数据范围类型：all | group | self | attribute。
	ScopeType model.DataScopeType `json:"scope_type"`
	// GroupID 用户分组 ID（scope_type=group 时使用）。
	GroupID int64 `json:"group_id"`
	// OwnerField 数据归属字段名（scope_type=self 时使用）。
	OwnerField string `json:"owner_field"`
	// AttrKey 数据属性键（scope_type=attribute 时使用）。
	AttrKey string `json:"attr_key"`
	// AttrValue 数据属性值（scope_type=attribute 时使用）。
	AttrValue string `json:"attr_value"`
	// Sort 排序。
	Sort int64 `json:"sort"`
}

// --- 创建 ---

// PolicyCreateRequest 创建策略请求。
type PolicyCreateRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	// Statements 授权规则明细（至少一条）。
	Statements []PolicyStatementDTO `json:"statements" binding:"required"`
	// Status 状态（默认启用，nil 视为 true；与账号/账号组创建语义一致）。
	Status *bool `json:"status"`
	// CreatedBy 创建者 ID（由 handler 注入）。
	CreatedBy int64 `json:"-"`
}

// Create 创建策略。
func (s *PolicyLogic) Create(ctx context.Context, req *PolicyCreateRequest) (*model.Policy, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, errors.New("策略名不能为空")
	}

	// 策略名唯一性检查
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Policy{}).Where("name = ?", req.Name).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("策略名已存在")
	}

	// 默认启用（与账号/账号组创建一致），nil 视为 true
	status := true
	if req.Status != nil {
		status = *req.Status
	}

	// 授权规则入参校验（Effect/Action/ScopeType 及数据范围必填字段）
	if err := validateStatements(req.Statements); err != nil {
		return nil, err
	}

	policy := model.Policy{
		Name:        req.Name,
		Version:     "1",
		Description: req.Description,
		Type:        model.PolicyTypeCustom,
		Status:      status,
		CreatedBy:   req.CreatedBy,
	}

	buildStatements(req.Statements, &policy)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&policy).Error
	})
	if err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "policy created", logger.Int64("policy_id", policy.ID), logger.String("name", policy.Name))
	return s.GetByID(ctx, policy.ID)
}

// --- 更新 ---

// PolicyUpdateRequest 更新策略请求。
type PolicyUpdateRequest struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Status      *bool  `json:"status"`
	// Statements 授权规则（nil 表示不修改，整体替换）。
	Statements []PolicyStatementDTO `json:"statements"`
}

// Update 更新策略。
func (s *PolicyLogic) Update(ctx context.Context, req *PolicyUpdateRequest) (*model.Policy, error) {
	var policy model.Policy
	if err := s.db.WithContext(ctx).First(&policy, req.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("策略不存在")
		}
		return nil, err
	}

	// 系统内置策略不可修改
	if policy.Type == model.PolicyTypeSystem {
		return nil, errors.New("系统内置策略不可修改")
	}

	updates := map[string]any{
		"description": req.Description,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	// 授权规则入参校验（整体替换前），避免写入非法 Effect 等脏数据
	if req.Statements != nil {
		if err := validateStatements(req.Statements); err != nil {
			return nil, err
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&policy).Updates(updates).Error; err != nil {
			return err
		}
		// 整体替换授权规则
		if req.Statements != nil {
			if err := replacePolicyStatements(tx, &policy, req.Statements); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 策略规则/状态变更：失效所有引用该策略的主体的权限缓存，使变更即时生效
	if s.permLogic != nil {
		s.invalidatePolicyReferences(ctx, policy.ID)
	}

	logger.InfoCtx(ctx, "policy updated", logger.Int64("policy_id", policy.ID))
	return s.GetByID(ctx, policy.ID)
}

// --- 删除 ---

// Delete 删除策略。
// 系统内置策略不可删除；删除时级联清理授权关系与授权规则。
func (s *PolicyLogic) Delete(ctx context.Context, id int64) error {
	var policy model.Policy
	if err := s.db.WithContext(ctx).First(&policy, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("策略不存在")
		}
		return err
	}

	if policy.Type == model.PolicyTypeSystem {
		return errors.New("系统内置策略不可删除")
	}

	// 先查询引用该策略的主体（删除授权关系后无法再查），用于删除后失效缓存
	var attachments []model.PolicyAttachment
	if err := s.db.WithContext(ctx).Where("policy_id = ?", id).Find(&attachments).Error; err != nil {
		return err
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清理授权关系
		if err := tx.Where("policy_id = ?", id).Delete(&model.PolicyAttachment{}).Error; err != nil {
			return err
		}
		// 删除策略（显式清理语句明细后删除语句，再删除策略本身）
		if err := deletePolicyStatements(tx, id); err != nil {
			return err
		}
		// 物理删除：释放 name 唯一索引，避免删除后无法重建同名策略
		return tx.Unscoped().Delete(&model.Policy{}, id).Error
	})
	if err != nil {
		return err
	}

	// 策略删除：失效所有曾引用该策略的主体的权限缓存
	if s.permLogic != nil {
		for _, a := range attachments {
			s.permLogic.InvalidatePermissionCache(ctx, a.PrincipalType, a.PrincipalID)
		}
	}
	return nil
}

// invalidatePolicyReferences 使所有引用指定策略的主体的权限缓存失效。
func (s *PolicyLogic) invalidatePolicyReferences(ctx context.Context, policyID int64) {
	if s.permLogic == nil {
		return
	}
	var attachments []model.PolicyAttachment
	if err := s.db.WithContext(ctx).Where("policy_id = ?", policyID).Find(&attachments).Error; err != nil {
		return
	}
	for _, a := range attachments {
		s.permLogic.InvalidatePermissionCache(ctx, a.PrincipalType, a.PrincipalID)
	}
}

// --- 内部辅助 ---

// deletePolicyStatements 删除策略下全部授权语句及其明细（数据范围）。
// 说明：GORM 的 Select 级联 Delete 不支持多级递归（Statements.Scopes 等），
// 会产生孤儿明细，因此这里显式分步删除：先删明细（scopes），再删语句。
func deletePolicyStatements(tx *gorm.DB, policyID int64) error {
	var statementIDs []int64
	if err := tx.Model(&model.PolicyStatement{}).Where("policy_id = ?", policyID).Pluck("id", &statementIDs).Error; err != nil {
		return err
	}
	if len(statementIDs) == 0 {
		return nil
	}
	if err := tx.Where("statement_id IN ?", statementIDs).Delete(&model.DataScope{}).Error; err != nil {
		return err
	}
	return tx.Where("policy_id = ?", policyID).Delete(&model.PolicyStatement{}).Error
}

// validateStatements 校验策略授权规则的合法性（创建/更新统一入口）。
// 校验 Effect 枚举、Action 非空、ScopeType 枚举及各类型数据范围的必填字段，
// 避免非法取值写入后导致权限判定异常（如非 Deny 的 Effect 被当作允许）。
func validateStatements(dtos []PolicyStatementDTO) error {
	for i, sd := range dtos {
		effect := strings.ToUpper(strings.TrimSpace(sd.Effect))
		if effect != "ALLOW" && effect != "DENY" {
			return fmt.Errorf("第 %d 条规则的 effect 必须为 Allow 或 Deny", i+1)
		}
		if strings.TrimSpace(sd.Action) == "" {
			return fmt.Errorf("第 %d 条规则的 action 不能为空", i+1)
		}
		for j, sc := range sd.Scopes {
			if !sc.ScopeType.Valid() {
				return fmt.Errorf("第 %d 条规则第 %d 个数据范围的 scope_type 无效", i+1, j+1)
			}
			switch sc.ScopeType {
			case model.DataScopeGroup:
				if sc.GroupID <= 0 {
					return fmt.Errorf("第 %d 条规则第 %d 个数据范围（group 类型）必须指定 group_id", i+1, j+1)
				}
			case model.DataScopeSelf:
				if strings.TrimSpace(sc.OwnerField) == "" {
					return fmt.Errorf("第 %d 条规则第 %d 个数据范围（self 类型）必须指定 owner_field", i+1, j+1)
				}
			case model.DataScopeAttribute:
				if strings.TrimSpace(sc.AttrKey) == "" {
					return fmt.Errorf("第 %d 条规则第 %d 个数据范围（attribute 类型）必须指定 attr_key", i+1, j+1)
				}
			}
		}
	}
	return nil
}

// normalizeResource 资源字段归一化：去空格，空视为 *（全部资源）。
func normalizeResource(r string) string {
	r = strings.TrimSpace(r)
	if r == "" {
		return "*"
	}
	return r
}

// buildStatements 构建策略的授权规则明细（Effect/Action 归一化：去空格 + 统一大写）。
func buildStatements(dtos []PolicyStatementDTO, policy *model.Policy) {
	for i, sd := range dtos {
		statement := model.PolicyStatement{
			Description: sd.Description,
			Effect:      strings.ToUpper(strings.TrimSpace(sd.Effect)),
			Action:      strings.TrimSpace(sd.Action),
			Resource:    normalizeResource(sd.Resource),
			Sort:        sd.Sort,
		}
		if statement.Sort == 0 {
			statement.Sort = int64(i)
		}
		for _, sc := range sd.Scopes {
			statement.Scopes = append(statement.Scopes, model.DataScope{
				ScopeType:  sc.ScopeType,
				GroupID:    sc.GroupID,
				OwnerField: sc.OwnerField,
				AttrKey:    sc.AttrKey,
				AttrValue:  sc.AttrValue,
				Sort:       sc.Sort,
			})
		}
		policy.Statements = append(policy.Statements, statement)
	}
}

// replacePolicyStatements 整体替换策略的授权规则（显式清理旧明细后重建）。
func replacePolicyStatements(tx *gorm.DB, policy *model.Policy, dtos []PolicyStatementDTO) error {
	// 显式清理旧授权语句及明细
	if err := deletePolicyStatements(tx, policy.ID); err != nil {
		return err
	}

	if len(dtos) == 0 {
		return nil
	}

	newPolicy := model.Policy{ID: policy.ID}
	buildStatements(dtos, &newPolicy)
	// 显式填充 PolicyID：直接 Create slice 时 GORM 不会自动推断父 ID，否则会产生 policy_id=0 的孤儿语句。
	for i := range newPolicy.Statements {
		newPolicy.Statements[i].PolicyID = policy.ID
	}
	// 仅重建明细表（不触碰策略主表）
	return tx.Create(&newPolicy.Statements).Error
}
