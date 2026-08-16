package logic

import (
	"context"
	"errors"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
)

// GrantLogic 授权管理逻辑：策略绑定到主体（账号/账号组/角色/应用）。
type GrantLogic struct {
	db        *gorm.DB
	permLogic *PermissionLogic // 授权变更后失效权限缓存
}

// NewGrantLogic 创建授权管理逻辑。
func NewGrantLogic(db *gorm.DB) *GrantLogic {
	return &GrantLogic{db: db}
}

// SetPermissionLogic 注入权限逻辑（授权变更后主动失效缓存）。
func (s *GrantLogic) SetPermissionLogic(permLogic *PermissionLogic) {
	s.permLogic = permLogic
}

// GrantRequest 授权请求。
type GrantRequest struct {
	// PrincipalType 主体类型：account | group | app。
	PrincipalType model.PrincipalType `json:"principal_type" binding:"required"`
	// PrincipalID 主体 ID。
	PrincipalID int64 `json:"principal_id" binding:"required"`
	// PolicyIDs 要绑定的策略 ID 列表。
	PolicyIDs []int64 `json:"policy_ids" binding:"required"`
	// CreatedBy 授权人 ID（由 handler 注入）。
	CreatedBy int64 `json:"-"`
}

// Grant 为某主体绑定一组策略。
func (s *GrantLogic) Grant(ctx context.Context, req *GrantRequest) error {
	if !req.PrincipalType.Valid() {
		return errors.New("无效的主体类型")
	}
	if len(req.PolicyIDs) == 0 {
		return errors.New("策略列表不能为空")
	}

	// 去重，避免重复 ID 导致误判「存在无效策略」或重复绑定
	req.PolicyIDs = uniqueIDs(req.PolicyIDs)

	// 校验策略存在且启用
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Policy{}).
		Where("id IN ? AND status = ?", req.PolicyIDs, true).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(req.PolicyIDs)) {
		return errors.New("存在无效或未启用的策略")
	}

	// 校验主体存在
	if err := s.ensurePrincipal(ctx, req.PrincipalType, req.PrincipalID); err != nil {
		return err
	}

	// 幂等绑定（已存在的跳过）
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, policyID := range req.PolicyIDs {
			var count int64
			if err := tx.Model(&model.PolicyAttachment{}).
				Where("principal_type = ? AND principal_id = ? AND policy_id = ?",
					req.PrincipalType, req.PrincipalID, policyID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			attachment := model.PolicyAttachment{
				PrincipalType: req.PrincipalType,
				PrincipalID:   req.PrincipalID,
				PolicyID:      policyID,
				CreatedBy:     req.CreatedBy,
			}
			if err := tx.Create(&attachment).Error; err != nil {
				return err
			}
		}
		grantLog(ctx, "policy granted", req.PrincipalType, req.PrincipalID, int64(len(req.PolicyIDs)))
		return nil
	}); err != nil {
		return err
	}

	// 授权变更：失效该主体（group 连带组内账号）的权限缓存，使新策略即时生效
	if s.permLogic != nil {
		s.permLogic.InvalidatePermissionCache(ctx, req.PrincipalType, req.PrincipalID)
	}
	return nil
}

// RevokeRequest 取消授权请求。
type RevokeRequest struct {
	PrincipalType model.PrincipalType `json:"principal_type" binding:"required"`
	PrincipalID   int64               `json:"principal_id" binding:"required"`
	// PolicyIDs 要解绑的策略 ID 列表（为空表示解绑全部）。
	PolicyIDs []int64 `json:"policy_ids"`
}

// Revoke 解绑主体的策略。
func (s *GrantLogic) Revoke(ctx context.Context, req *RevokeRequest) error {
	if !req.PrincipalType.Valid() {
		return errors.New("无效的主体类型")
	}

	query := s.db.WithContext(ctx).
		Where("principal_type = ? AND principal_id = ?", req.PrincipalType, req.PrincipalID)
	if len(req.PolicyIDs) > 0 {
		query = query.Where("policy_id IN ?", req.PolicyIDs)
	}
	res := query.Delete(&model.PolicyAttachment{})
	if res.Error != nil {
		return res.Error
	}
	// 日志记录实际删除的授权关系数（PolicyIDs 为空表示解绑全部，此时按 0 计会误导）
	grantLog(ctx, "policy revoked", req.PrincipalType, req.PrincipalID, res.RowsAffected)

	// 授权变更：失效该主体（group 连带组内账号）的权限缓存
	if s.permLogic != nil {
		s.permLogic.InvalidatePermissionCache(ctx, req.PrincipalType, req.PrincipalID)
	}
	return nil
}

// ListByPrincipal 查询某主体已绑定的策略。
func (s *GrantLogic) ListByPrincipal(ctx context.Context, principalType model.PrincipalType, principalID int64) ([]model.Policy, error) {
	if !principalType.Valid() {
		return nil, errors.New("无效的主体类型")
	}

	var policies []model.Policy
	policyTable := model.Policy{}.TableName()
	attachTable := model.PolicyAttachment{}.TableName()
	err := s.db.WithContext(ctx).
		Joins("JOIN "+attachTable+" pa ON pa.policy_id = "+policyTable+".id").
		Where("pa.principal_type = ? AND pa.principal_id = ?", principalType, principalID).
		Order(policyTable + ".id ASC").
		Find(&policies).Error
	return policies, err
}

// ListPrincipals 查询某策略被哪些主体绑定。
func (s *GrantLogic) ListPrincipals(ctx context.Context, policyID int64) ([]model.PolicyAttachment, error) {
	var attachments []model.PolicyAttachment
	err := s.db.WithContext(ctx).
		Where("policy_id = ?", policyID).
		Order("id ASC").
		Find(&attachments).Error
	return attachments, err
}

// ensurePrincipal 校验主体是否存在。
func (s *GrantLogic) ensurePrincipal(ctx context.Context, principalType model.PrincipalType, principalID int64) error {
	var count int64
	switch principalType {
	case model.PrincipalTypeAccount:
		err := s.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", principalID).Count(&count).Error
		if err != nil {
			return err
		}
	case model.PrincipalTypeGroup:
		err := s.db.WithContext(ctx).Model(&model.Group{}).Where("id = ?", principalID).Count(&count).Error
		if err != nil {
			return err
		}
	case model.PrincipalTypeApp:
		err := s.db.WithContext(ctx).Model(&model.App{}).Where("id = ?", principalID).Count(&count).Error
		if err != nil {
			return err
		}
	default:
		return errors.New("无效的主体类型")
	}
	if count == 0 {
		return errors.New("主体不存在")
	}
	return nil
}

// log 辅助：记录授权操作（避免误用，实际在 Grant/Revoke 中调用）。
func grantLog(ctx context.Context, action string, pt model.PrincipalType, pid, policyCount int64) {
	logger.InfoCtx(ctx, action,
		logger.String("principal_type", pt.String()),
		logger.Int64("principal_id", pid),
		logger.Int64("policy_count", policyCount))
}
