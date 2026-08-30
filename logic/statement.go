// Package logic 语句池（授权语句独立管理）。
//
// 授权语句（Statement）是权限策略的授权规则载体，独立成池管理：
//   - 语句可被多个策略共享引用（多对多，见 model.PolicyStatementLink）；
//   - 策略新增/编辑只负责关联（选择已有语句），不内嵌编辑语句；
//   - 语句变更后，失效所有引用该语句的策略所对应主体的权限缓存。
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

// StatementLogic 授权语句（语句池）管理逻辑：独立菜单维护授权规则。
type StatementLogic struct {
	db *gorm.DB
	// permLogic 权限逻辑（语句变更后失效引用该语句的策略的主体权限缓存，nil 表示不失效）。
	permLogic *PermissionLogic
}

// NewStatementLogic 创建语句池管理逻辑。
func NewStatementLogic(db *gorm.DB) *StatementLogic {
	return &StatementLogic{db: db}
}

// SetPermissionLogic 注入权限逻辑（语句变更后失效权限缓存）。
func (s *StatementLogic) SetPermissionLogic(permLogic *PermissionLogic) {
	s.permLogic = permLogic
}

// StatementListRequest 语句池列表请求。
type StatementListRequest struct {
	PageRequest
	// Key 关键字（匹配描述 / 操作 Action）。
	Key string `form:"key"`
	// Effect 效果过滤：Allow | Deny，空表示全部。
	Effect string `form:"effect"`
}

// applyScope 按当前账号对语句池的可见性过滤：
// accountID<=0（admin）全量；非 admin 仅本人创建（created_by=本人）与系统内置（created_by=0）。
func (s *StatementLogic) applyScope(ctx context.Context, query *gorm.DB, accountID int64) {
	if accountID <= 0 {
		return
	}
	query.Where("created_by = ? OR created_by = 0", accountID)
}

// List 语句池分页列表。
func (s *StatementLogic) List(ctx context.Context, accountID int64, req *StatementListRequest) (*PageResponse[model.Statement], error) {
	var statements []model.Statement
	var total int64

	query := s.db.WithContext(ctx).Model(&model.Statement{})
	if req.Key != "" {
		like := "%" + req.Key + "%"
		query = query.Where("description LIKE ? OR action LIKE ?", like, like)
	}
	if req.Effect != "" {
		query = query.Where("effect = ?", strings.ToUpper(strings.TrimSpace(req.Effect)))
	}

	s.applyScope(ctx, query, accountID)

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (req.Page - 1) * req.Size
	if err := query.Order("id ASC").Offset(offset).Limit(req.Size).Find(&statements).Error; err != nil {
		return nil, err
	}

	return &PageResponse[model.Statement]{Data: statements, Total: total}, nil
}

// AllList 全部语句（策略关联选择用）。
// accountID<=0 返回全部；否则按可见性过滤，防止下拉选择越权语句。
func (s *StatementLogic) AllList(ctx context.Context, accountID int64) ([]model.Statement, error) {
	var statements []model.Statement
	query := s.db.WithContext(ctx).Model(&model.Statement{}).Order("id ASC")
	s.applyScope(ctx, query, accountID)
	err := query.Find(&statements).Error
	return statements, err
}

// GetByID 语句详情（含数据范围明细）。
func (s *StatementLogic) GetByID(ctx context.Context, id int64) (*model.Statement, error) {
	var statement model.Statement
	if err := s.db.WithContext(ctx).
		Preload("Scopes").
		First(&statement, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("授权语句不存在")
		}
		return nil, err
	}
	return &statement, nil
}

// CanManage 判断账号 managerID 是否有权管理（编辑/删除）语句 targetID。
// 系统内置语句（created_by=0）不可由普通账号管理；非 admin 仅可管理本人创建的语句。
func (s *StatementLogic) CanManage(ctx context.Context, managerID, targetID int64) (bool, error) {
	var statement model.Statement
	if err := s.db.WithContext(ctx).Select("id", "created_by").First(&statement, targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, errors.New("授权语句不存在")
		}
		return false, err
	}
	if managerID <= 0 {
		return true, nil // admin / 系统主体
	}
	if statement.CreatedBy == 0 {
		return false, nil // 系统内置语句仅系统可管
	}
	return statement.CreatedBy == managerID, nil
}

// --- 语句 DTO ---

// ScopeDTO 语句的数据范围。
// 类型：all（全部）/ group（本用户分组）/ self（本人数据）/ attribute（属性过滤）。
type ScopeDTO struct {
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

// StatementDTO 授权语句（语句池）。
type StatementDTO struct {
	// Description 语句描述（小标题，说明本条授权规则的用途）。
	Description string `json:"description"`
	// Effect 效果：Allow | Deny。
	Effect string `json:"effect"`
	// Action 操作（逗号分隔）。
	Action string `json:"action"`
	// Resource 资源（支持 * 通配，默认 * 表示全部资源）。
	Resource string `json:"resource"`
	// Scopes 数据范围明细（数据权限）。
	Scopes []ScopeDTO `json:"scopes"`
	// Sort 排序。
	Sort int64 `json:"sort"`
}

// --- 创建 ---

// StatementCreateRequest 创建授权语句请求。
type StatementCreateRequest struct {
	StatementDTO
	// CreatedBy 创建者 ID（由 handler 注入）。
	CreatedBy int64 `json:"-"`
}

// Create 创建授权语句（语句池）。
func (s *StatementLogic) Create(ctx context.Context, req *StatementCreateRequest) (*model.Statement, error) {
	// 授权规则入参校验（Effect/Action/ScopeType 及数据范围必填字段）
	if err := validateStatementDTO(req.StatementDTO); err != nil {
		return nil, err
	}

	statement := buildStatement(req.StatementDTO)
	statement.CreatedBy = req.CreatedBy

	if err := s.db.WithContext(ctx).Create(&statement).Error; err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "statement created", logger.Int64("statement_id", statement.ID))
	return s.GetByID(ctx, statement.ID)
}

// --- 更新 ---

// StatementUpdateRequest 更新授权语句请求。
type StatementUpdateRequest struct {
	ID int64 `json:"id"`
	StatementDTO
}

// Update 更新授权语句（整体替换语句内容与数据范围）。
// 语句被策略共享引用，更新后所有关联策略同步生效（失效相关主体权限缓存）。
func (s *StatementLogic) Update(ctx context.Context, req *StatementUpdateRequest) (*model.Statement, error) {
	var statement model.Statement
	if err := s.db.WithContext(ctx).First(&statement, req.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("授权语句不存在")
		}
		return nil, err
	}

	// 授权规则入参校验（整体替换前），避免写入非法 Effect 等脏数据
	if err := validateStatementDTO(req.StatementDTO); err != nil {
		return nil, err
	}

	updates := map[string]any{
		"description": req.Description,
		"effect":      normalizeEffect(req.Effect),
		"action":      strings.TrimSpace(req.Action),
		"resource":    normalizeResource(req.Resource),
		"sort":        req.Sort,
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&statement).Updates(updates).Error; err != nil {
			return err
		}
		// 整体替换数据范围明细
		return replaceStatementScopes(tx, statement.ID, req.Scopes)
	})
	if err != nil {
		return nil, err
	}

	// 语句变更：失效引用该语句的策略对应主体的权限缓存，使变更即时生效
	if s.permLogic != nil {
		s.invalidateStatementReferences(ctx, statement.ID)
	}

	logger.InfoCtx(ctx, "statement updated", logger.Int64("statement_id", statement.ID))
	return s.GetByID(ctx, statement.ID)
}

// --- 删除 ---

// Delete 删除授权语句。
// 系统内置语句不可删除；被任一策略关联时禁止删除（提示先解除关联），
// 防止误删导致权限意外失效。
func (s *StatementLogic) Delete(ctx context.Context, id int64) error {
	var statement model.Statement
	if err := s.db.WithContext(ctx).First(&statement, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("授权语句不存在")
		}
		return err
	}

	if statement.CreatedBy == 0 {
		return errors.New("系统内置授权语句不可删除")
	}

	// 被策略关联时禁止删除
	var refCount int64
	if err := s.db.WithContext(ctx).Model(&model.PolicyStatementLink{}).
		Where("statement_id = ?", id).Count(&refCount).Error; err != nil {
		return err
	}
	if refCount > 0 {
		return fmt.Errorf("该授权语句已被 %d 个策略关联，请先在策略中解除关联", refCount)
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清理语句的数据范围明细
		if err := tx.Where("statement_id = ?", id).Delete(&model.DataScope{}).Error; err != nil {
			return err
		}
		// 物理删除语句
		return tx.Unscoped().Delete(&model.Statement{}, id).Error
	})
	if err != nil {
		return err
	}

	logger.InfoCtx(ctx, "statement deleted", logger.Int64("statement_id", id))
	return nil
}

// invalidateStatementReferences 使所有引用指定语句的策略对应主体的权限缓存失效。
func (s *StatementLogic) invalidateStatementReferences(ctx context.Context, statementID int64) {
	if s.permLogic == nil {
		return
	}
	var policyIDs []int64
	if err := s.db.WithContext(ctx).Model(&model.PolicyStatementLink{}).
		Where("statement_id = ?", statementID).
		Pluck("policy_id", &policyIDs).Error; err != nil {
		return
	}
	for _, pid := range policyIDs {
		s.permLogic.InvalidatePolicyReferences(ctx, pid)
	}
}

// --- 内部辅助 ---

// validateStatementDTO 校验授权语句的合法性（创建/更新统一入口）。
// 校验 Effect 枚举、Action 非空、ScopeType 枚举及各类型数据范围的必填字段，
// 避免非法取值写入后导致权限判定异常（如非 Deny 的 Effect 被当作允许）。
func validateStatementDTO(sd StatementDTO) error {
	if !model.IsValidEffect(sd.Effect) {
		return errors.New("effect 必须为 Allow 或 Deny")
	}
	if strings.TrimSpace(sd.Action) == "" {
		return errors.New("action 不能为空")
	}
	for j, sc := range sd.Scopes {
		if !sc.ScopeType.Valid() {
			return fmt.Errorf("第 %d 个数据范围的 scope_type 无效", j+1)
		}
		switch sc.ScopeType {
		case model.DataScopeGroup:
			if sc.GroupID <= 0 {
				return fmt.Errorf("第 %d 个数据范围（group 类型）必须指定 group_id", j+1)
			}
		case model.DataScopeSelf:
			if strings.TrimSpace(sc.OwnerField) == "" {
				return fmt.Errorf("第 %d 个数据范围（self 类型）必须指定 owner_field", j+1)
			}
		case model.DataScopeAttribute:
			if strings.TrimSpace(sc.AttrKey) == "" {
				return fmt.Errorf("第 %d 个数据范围（attribute 类型）必须指定 attr_key", j+1)
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

// normalizeEffect 授权效果归一化：大小写不敏感，统一为标准值 Allow / Deny。
// 校验不通过时返回 EffectAllow（调用方须先经 validateStatementDTO 校验）。
func normalizeEffect(e string) string {
	if strings.EqualFold(e, model.EffectDeny) {
		return model.EffectDeny
	}
	return model.EffectAllow
}

// buildStatement 从 DTO 构建语句模型（Effect/Action 归一化：去空格 + 统一标准效果）。
func buildStatement(sd StatementDTO) model.Statement {
	statement := model.Statement{
		Description: sd.Description,
		Effect:      normalizeEffect(sd.Effect),
		Action:      strings.TrimSpace(sd.Action),
		Resource:    normalizeResource(sd.Resource),
		Sort:        sd.Sort,
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
	return statement
}

// replaceStatementScopes 整体替换语句的数据范围明细（显式清理旧明细后重建）。
func replaceStatementScopes(tx *gorm.DB, statementID int64, scopes []ScopeDTO) error {
	if err := tx.Where("statement_id = ?", statementID).Delete(&model.DataScope{}).Error; err != nil {
		return err
	}
	if len(scopes) == 0 {
		return nil
	}
	newScopes := make([]model.DataScope, 0, len(scopes))
	for _, sc := range scopes {
		newScopes = append(newScopes, model.DataScope{
			StatementID: statementID,
			ScopeType:   sc.ScopeType,
			GroupID:     sc.GroupID,
			OwnerField:  sc.OwnerField,
			AttrKey:     sc.AttrKey,
			AttrValue:   sc.AttrValue,
			Sort:        sc.Sort,
		})
	}
	return tx.Create(&newScopes).Error
}
