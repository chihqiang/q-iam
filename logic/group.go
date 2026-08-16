package logic

import (
	"context"
	"errors"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
)

// GroupLogic 账号组管理逻辑：账号组 CRUD 与成员管理。
type GroupLogic struct {
	db *gorm.DB
	// permLogic 权限逻辑（成员变更后失效相关账号的权限缓存，nil 表示不失效）。
	permLogic *PermissionLogic
}

// NewGroupLogic 创建账号组管理逻辑。
func NewGroupLogic(db *gorm.DB) *GroupLogic {
	return &GroupLogic{db: db}
}

// SetPermissionLogic 注入权限逻辑（成员变更后失效权限缓存）。
func (s *GroupLogic) SetPermissionLogic(permLogic *PermissionLogic) {
	s.permLogic = permLogic
}

// GroupListRequest 账号组列表请求。
type GroupListRequest struct {
	PageRequest
	Status *bool  `form:"status"`
	Key    string `form:"key"`
}

// List 账号组分页列表。
func (s *GroupLogic) List(ctx context.Context, req *GroupListRequest) (*PageResponse[model.Group], error) {
	var groups []model.Group
	var total int64

	query := s.db.WithContext(ctx).Model(&model.Group{})
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}
	if req.Key != "" {
		like := "%" + req.Key + "%"
		query = query.Where("name LIKE ? OR display_name LIKE ?", like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (req.Page - 1) * req.Size
	if err := query.Order("id ASC").Offset(offset).Limit(req.Size).Find(&groups).Error; err != nil {
		return nil, err
	}

	return &PageResponse[model.Group]{Data: groups, Total: total}, nil
}

// AllList 返回全部启用的账号组（用于下拉选择）。
func (s *GroupLogic) AllList(ctx context.Context) ([]model.Group, error) {
	var groups []model.Group
	err := s.db.WithContext(ctx).Where("status = ?", true).Order("id ASC").Find(&groups).Error
	return groups, err
}

// GetByID 账号组详情（含组内账号）。
func (s *GroupLogic) GetByID(ctx context.Context, id int64) (*model.Group, error) {
	var group model.Group
	if err := s.db.WithContext(ctx).Preload("Accounts").First(&group, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("账号组不存在")
		}
		return nil, err
	}
	return &group, nil
}

// GroupCreateRequest 创建账号组请求。
type GroupCreateRequest struct {
	Name        string `json:"name" binding:"required"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	// Status 状态（默认启用，nil 视为 true）。
	Status *bool `json:"status"`
}

// Create 创建账号组。
func (s *GroupLogic) Create(ctx context.Context, req *GroupCreateRequest) (*model.Group, error) {
	// 名称唯一性检查
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Group{}).Where("name = ?", req.Name).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("账号组名已存在")
	}

	// 默认启用（与账号创建一致），nil 视为 true
	status := true
	if req.Status != nil {
		status = *req.Status
	}

	group := model.Group{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Status:      status,
	}
	if err := s.db.WithContext(ctx).Create(&group).Error; err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "account group created", logger.Int64("group_id", group.ID), logger.String("name", group.Name))
	return &group, nil
}

// GroupUpdateRequest 更新账号组请求。
type GroupUpdateRequest struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Status      *bool  `json:"status"`
}

// Update 更新账号组。
func (s *GroupLogic) Update(ctx context.Context, req *GroupUpdateRequest) (*model.Group, error) {
	var group model.Group
	if err := s.db.WithContext(ctx).First(&group, req.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("账号组不存在")
		}
		return nil, err
	}

	statusChanged := req.Status != nil && *req.Status != group.Status

	updates := map[string]any{
		"display_name": req.DisplayName,
		"description":  req.Description,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if err := s.db.WithContext(ctx).Model(&group).Updates(updates).Error; err != nil {
		return nil, err
	}

	// 状态变更（如启用→禁用）：组内账号不再获得该组绑定的策略，须失效其权限缓存
	if statusChanged && s.permLogic != nil {
		s.permLogic.InvalidatePermissionCache(ctx, model.PrincipalTypeGroup, group.ID)
	}

	logger.InfoCtx(ctx, "account group updated", logger.Int64("group_id", group.ID))
	return s.GetByID(ctx, group.ID)
}

// Delete 删除账号组。
// 同时清理账号组关联与授权关系。
func (s *GroupLogic) Delete(ctx context.Context, id int64) error {
	var group model.Group
	if err := s.db.WithContext(ctx).First(&group, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("账号组不存在")
		}
		return err
	}

	// 组删除后无法再查询组内账号，须在删除前失效组内账号的权限缓存（连带组本身）
	if s.permLogic != nil {
		s.permLogic.InvalidatePermissionCache(ctx, model.PrincipalTypeGroup, group.ID)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清理账号组关联
		if err := tx.Model(&group).Association("Accounts").Clear(); err != nil {
			return err
		}
		// 清理授权关系
		if err := tx.Where("principal_type = ? AND principal_id = ?",
			model.PrincipalTypeGroup, id).Delete(&model.PolicyAttachment{}).Error; err != nil {
			return err
		}
		// 物理删除：释放 name 唯一索引，避免删除后无法重建同名账号组
		return tx.Unscoped().Delete(&group).Error
	})
}

// GroupMemberRequest 组内成员账号 ID 列表。
type GroupMemberRequest struct {
	AccountIDs []int64 `json:"account_ids"`
}

// AddMembers 批量添加成员到账号组。
func (s *GroupLogic) AddMembers(ctx context.Context, groupID int64, accountIDs []int64) error {
	var group model.Group
	if err := s.db.WithContext(ctx).First(&group, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("账号组不存在")
		}
		return err
	}
	if len(accountIDs) == 0 {
		return nil
	}

	// 去重并校验账号 ID 均有效（避免静默忽略不存在的账号）
	unique := uniqueIDs(accountIDs)
	var accounts []model.Account
	if err := s.db.WithContext(ctx).Where("id IN ?", unique).Find(&accounts).Error; err != nil {
		return err
	}
	if len(accounts) != len(unique) {
		return errors.New("存在无效的账号")
	}

	// 追加（不覆盖现有成员）
	if err := s.db.WithContext(ctx).Model(&group).Association("Accounts").Append(accounts); err != nil {
		return err
	}

	// 成员变更：失效组（连带组内全部账号）的权限缓存，使新成员立即获得组权限
	if s.permLogic != nil {
		s.permLogic.InvalidatePermissionCache(ctx, model.PrincipalTypeGroup, groupID)
	}
	return nil
}

// RemoveMembers 批量移除账号组成员。
func (s *GroupLogic) RemoveMembers(ctx context.Context, groupID int64, accountIDs []int64) error {
	var group model.Group
	if err := s.db.WithContext(ctx).First(&group, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("账号组不存在")
		}
		return err
	}
	if len(accountIDs) == 0 {
		return nil
	}

	// 去重并校验账号 ID 均有效
	unique := uniqueIDs(accountIDs)
	var accounts []model.Account
	if err := s.db.WithContext(ctx).Where("id IN ?", unique).Find(&accounts).Error; err != nil {
		return err
	}
	if len(accounts) != len(unique) {
		return errors.New("存在无效的账号")
	}

	if err := s.db.WithContext(ctx).Model(&group).Association("Accounts").Delete(accounts); err != nil {
		return err
	}

	// 成员变更：被移除的账号不再属于该组，须逐个失效其账号权限缓存
	if s.permLogic != nil {
		for _, a := range accounts {
			s.permLogic.InvalidatePermissionCache(ctx, model.PrincipalTypeAccount, a.ID)
		}
	}
	return nil
}

// ReplaceMembers 覆盖账号组成员（nil 表示清空）。
func (s *GroupLogic) ReplaceMembers(ctx context.Context, groupID int64, accountIDs []int64) error {
	var group model.Group
	if err := s.db.WithContext(ctx).First(&group, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("账号组不存在")
		}
		return err
	}

	// 去重并校验账号 ID 均有效
	var accounts []model.Account
	if len(accountIDs) > 0 {
		unique := uniqueIDs(accountIDs)
		if err := s.db.WithContext(ctx).Where("id IN ?", unique).Find(&accounts).Error; err != nil {
			return err
		}
		if len(accounts) != len(unique) {
			return errors.New("存在无效的账号")
		}
	}

	// 记录替换前的组内账号（被替换掉的旧成员不再属于该组，须显式失效）
	var oldAccounts []model.Account
	if err := s.db.WithContext(ctx).Model(&group).Association("Accounts").Find(&oldAccounts); err != nil {
		return err
	}

	if err := s.db.WithContext(ctx).Model(&group).Association("Accounts").Replace(accounts); err != nil {
		return err
	}

	// 成员变更：失效组（连带新成员）+ 被替换掉的旧成员
	if s.permLogic != nil {
		s.permLogic.InvalidatePermissionCache(ctx, model.PrincipalTypeGroup, groupID)
		for _, a := range oldAccounts {
			s.permLogic.InvalidatePermissionCache(ctx, model.PrincipalTypeAccount, a.ID)
		}
	}
	return nil
}
