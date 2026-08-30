package logic

import (
	"context"
	"errors"
	"strings"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
)

// PolicyLogic 权限策略管理逻辑：策略 CRUD 与授权语句关联维护。
// 授权语句（Statement）独立成池管理（见 statement.go），策略新增/编辑只负责关联（选择已有语句）。
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

// GetByID 策略详情（含关联的授权语句及其数据范围）。
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

// --- 创建 ---

// PolicyCreateRequest 创建策略请求。
// 授权语句通过 StatementIDs 关联语句池中的已有语句（不内嵌编辑语句）。
type PolicyCreateRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	// StatementIDs 关联的授权语句 ID 列表（至少一条）。
	StatementIDs []int64 `json:"statement_ids" binding:"required"`
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

	// 关联语句去重 + 校验存在性
	statementIDs := uniqueIDs(req.StatementIDs)
	if len(statementIDs) == 0 {
		return nil, errors.New("至少关联一条授权语句")
	}
	statements, err := s.statementsByIDs(ctx, statementIDs, req.CreatedBy)
	if err != nil {
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

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&policy).Error; err != nil {
			return err
		}
		// 通过关联表建立策略 ↔ 语句 多对多关联
		return tx.Model(&policy).Association("Statements").Append(statements)
	})
	if err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "policy created", logger.Int64("policy_id", policy.ID), logger.String("name", policy.Name))
	return s.GetByID(ctx, policy.ID)
}

// --- 更新 ---

// PolicyUpdateRequest 更新策略请求。
// StatementIDs 关联的授权语句 ID 列表（nil 表示不修改关联，非 nil 则整体替换关联）。
type PolicyUpdateRequest struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Status      *bool  `json:"status"`
	// StatementIDs 授权语句关联（nil 表示不修改，整体替换）。
	StatementIDs []int64 `json:"statement_ids"`
	// CreatedBy 当前操作账号 ID（由 handler 注入，用于语句归属校验；0 表示 admin）。
	CreatedBy int64 `json:"-"`
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

	// 关联语句去重 + 校验存在性（整体替换前）
	var statements []model.Statement
	if req.StatementIDs != nil {
		ids := uniqueIDs(req.StatementIDs)
		if len(ids) == 0 {
			return nil, errors.New("至少关联一条授权语句")
		}
		var err error
		statements, err = s.statementsByIDs(ctx, ids, req.CreatedBy)
		if err != nil {
			return nil, err
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&policy).Updates(updates).Error; err != nil {
			return err
		}
		// 整体替换关联的授权语句
		if req.StatementIDs != nil {
			if err := tx.Model(&policy).Association("Statements").Replace(statements); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 策略规则/状态/关联变更：失效所有引用该策略的主体的权限缓存，使变更即时生效
	if s.permLogic != nil {
		s.permLogic.InvalidatePolicyReferences(ctx, policy.ID)
	}

	logger.InfoCtx(ctx, "policy updated", logger.Int64("policy_id", policy.ID))
	return s.GetByID(ctx, policy.ID)
}

// --- 删除 ---

// Delete 删除策略。
// 系统内置策略不可删除；删除时清理授权关系并解除与授权语句的关联（不删除语句池中的语句）。
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
		// 解除策略 ↔ 授权语句 关联（保留语句池中的语句）
		if err := tx.Where("policy_id = ?", id).Delete(&model.PolicyStatementLink{}).Error; err != nil {
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

// statementsByIDs 按 ID 批量查询语句池中的语句（用于策略关联），校验存在性与归属可见性。
// ownerID<=0（admin）不校验归属；否则仅允许关联 ownerID 本人创建（created_by=ownerID）
// 或系统内置（created_by=0）的语句，防止越权引用他人私有语句。
// 查询顺序不保证与入参一致，调用方应仅用于关联（排序由语句自身 Sort 决定）。
func (s *PolicyLogic) statementsByIDs(ctx context.Context, ids []int64, ownerID int64) ([]model.Statement, error) {
	var statements []model.Statement
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&statements).Error; err != nil {
		return nil, err
	}
	if len(statements) != len(ids) {
		return nil, errors.New("存在无效的授权语句 ID")
	}
	// 非 admin：归属可见性校验
	if ownerID > 0 {
		for _, st := range statements {
			if st.CreatedBy != 0 && st.CreatedBy != ownerID {
				return nil, errors.New("存在无权关联的授权语句")
			}
		}
	}
	return statements, nil
}
