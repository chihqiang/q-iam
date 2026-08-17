package logic

import (
	"context"
	"errors"
	"strings"
	"time"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/hash"
	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
)

// AccountCacheInvalidator 账号信息缓存失效接口（由 AuthLogic 实现）。
// AccountLogic 通过该接口在账号变更（禁用/删除/改密）后失效认证中间件的账号缓存，
// 避免缓存旧状态导致已禁用/已删除账号被继续放行。
type AccountCacheInvalidator interface {
	InvalidateAccountCache(ctx context.Context, accountID int64)
}

// AccountLogic 账号管理逻辑：账号 CRUD、修改/重置密码、账号组关联。
type AccountLogic struct {
	db       *gorm.DB
	password *PasswordValidator
	// permLogic 权限逻辑（组关联变更后失效账号权限缓存，nil 表示不失效）。
	permLogic *PermissionLogic
	// cacheInvalidator 账号信息缓存失效器（nil 表示不失效，如未注入）。
	cacheInvalidator AccountCacheInvalidator
}

// NewAccountLogic 创建账号管理逻辑。
func NewAccountLogic(db *gorm.DB, password *PasswordValidator) *AccountLogic {
	return &AccountLogic{db: db, password: password}
}

// SetPermissionLogic 注入权限逻辑（组关联变更后失效权限缓存）。
func (s *AccountLogic) SetPermissionLogic(permLogic *PermissionLogic) {
	s.permLogic = permLogic
}

// SetCacheInvalidator 注入账号信息缓存失效器（认证中间件的账号缓存，见 AuthLogic）。
func (s *AccountLogic) SetCacheInvalidator(inv AccountCacheInvalidator) {
	s.cacheInvalidator = inv
}

// invalidateAccountCache 失效某账号的信息缓存（内部封装，nil 安全）。
func (s *AccountLogic) invalidateAccountCache(ctx context.Context, accountID int64) {
	if s.cacheInvalidator != nil {
		s.cacheInvalidator.InvalidateAccountCache(ctx, accountID)
	}
}

// nullableString 将空字符串转为 nil，避免空字符串触发唯一索引冲突。
func nullableString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// normalizeDuplicateError 将数据库唯一约束冲突转换为友好业务提示。
// infra-go 的 orm 未启用 GORM TranslateError，因此驱动错误不会翻译为
// gorm.ErrDuplicatedKey；这里按主流驱动的唯一冲突错误文本匹配（sqlite/mysql/postgres），
// 把并发下唯一索引兜底的原始 DB 错误转成可读提示。
func normalizeDuplicateError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unique constraint failed"), // sqlite
		strings.Contains(msg, "duplicate entry"),   // mysql
		strings.Contains(msg, "duplicate key"),     // postgres / sqlite 变体
		strings.Contains(msg, "unique constraint"): // postgres 变体
		return errors.New("账号名/邮箱/手机号已存在")
	}
	return err
}

// AccountListRequest 账号列表请求。
type AccountListRequest struct {
	PageRequest
	Status *bool  `form:"status"`
	Key    string `form:"key"`
}

// AccountItem 账号列表项（隐藏敏感字段）。
type AccountItem struct {
	ID           int64  `json:"id"`
	AccountName  string `json:"account_name"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Mobile       string `json:"mobile"`
	Status       bool   `json:"status"`
	AllowConsole bool   `json:"allow_console"`
	Remark       string `json:"remark"`
	CreatedAt    string `json:"created_at"`
}

// List 账号分页列表（按数据权限过滤可见范围）。
// accountID 为数据范围过滤主体：<=0（admin/系统主体）不过滤（全量）；
// 否则按该账号对 iam:account:read 的数据范围（self/group）过滤，防止越权查看。
func (s *AccountLogic) List(ctx context.Context, accountID int64, req *AccountListRequest) (*PageResponse[AccountItem], error) {
	var accounts []model.Account
	var total int64

	query := s.db.WithContext(ctx).Model(&model.Account{})
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}
	if req.Key != "" {
		like := "%" + req.Key + "%"
		query = query.Where("account_name LIKE ? OR display_name LIKE ? OR email LIKE ?", like, like, like)
	}

	// 数据范围过滤（非 admin 账号按 iam:account:read 的数据范围可见性过滤）
	s.applyAccountScope(ctx, query, accountID)

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (req.Page - 1) * req.Size
	if err := query.Order("id ASC").Offset(offset).Limit(req.Size).Find(&accounts).Error; err != nil {
		return nil, err
	}

	items := make([]AccountItem, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, toAccountItem(a))
	}
	return &PageResponse[AccountItem]{Data: items, Total: total}, nil
}

// applyAccountScope 按当前账号对账号资源的数据范围过滤查询。
// accountID<=0（admin/系统主体）或权限逻辑未注入时不过滤（全量）。
// 数据范围加载失败时保守降级为仅本人可见，避免越权。
func (s *AccountLogic) applyAccountScope(ctx context.Context, query *gorm.DB, accountID int64) {
	if accountID <= 0 || s.permLogic == nil {
		return
	}
	acctTable := model.Account{}.TableName() // q_iam_accounts

	scope, err := s.permLogic.AccountDataScope(ctx, accountID)
	if err != nil {
		logger.WarnCtx(ctx, "load account data scope failed, fallback to self",
			logger.Err(err), logger.Int64("account_id", accountID))
		query.Where(acctTable+".id = ?", accountID)
		return
	}
	if scope.All {
		return
	}

	conds := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if scope.SelfOnly {
		conds = append(conds, acctTable+".id = ?")
		args = append(args, accountID)
	}
	if len(scope.GroupIDs) > 0 {
		// 账号组多对多关联表（q_iam_account_groups，见 model.Account 的 many2many 标签）
		conds = append(conds, "EXISTS (SELECT 1 FROM q_iam_account_groups ag WHERE ag.account_id = "+acctTable+".id AND ag.group_id IN ?)")
		args = append(args, scope.GroupIDs)
	}
	if len(conds) == 0 {
		// 无任何可见范围：返回空集
		query.Where("1 = 0")
		return
	}
	query.Where(strings.Join(conds, " OR "), args...)
}

// GetByID 账号详情（含所属账号组）。
func (s *AccountLogic) GetByID(ctx context.Context, id int64) (*model.Account, error) {
	var account model.Account
	if err := s.db.WithContext(ctx).Preload("Groups").First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

// CanViewAccount 判断账号 viewerID 是否有权查看账号 targetID 的详情。
// 委托 PermissionLogic.CanAccessPrincipal 基于 iam:account:read 数据范围判定。
// 供详情接口做数据范围校验，防止「列表已过滤但详情接口按 ID 枚举绕过」越权。
// 权限逻辑未注入时保守拒绝（返回 false）。
func (s *AccountLogic) CanViewAccount(ctx context.Context, viewerID, targetID int64) (bool, error) {
	if s.permLogic == nil {
		return false, nil
	}
	return s.permLogic.CanAccessPrincipal(ctx, viewerID, model.PrincipalTypeAccount, targetID)
}

// AllList 全部启用的账号（用于授权下拉选择）。
// 与 List 不同：不分页、不过滤 AllowConsole，仅按启用状态返回完整列表。
// accountID<=0（admin/系统主体）返回全部；否则按数据权限过滤可见范围，
// 防止下拉选择越权账号。返回的账号含敏感字段（Password 带 json:"-" 不序列化），仅限管理接口使用。
func (s *AccountLogic) AllList(ctx context.Context, accountID int64) ([]model.Account, error) {
	var accounts []model.Account
	query := s.db.WithContext(ctx).Where("status = ?", true).Order("id ASC")
	// 数据范围过滤（同 List）
	s.applyAccountScope(ctx, query, accountID)
	err := query.Find(&accounts).Error
	return accounts, err
}

// AccountCreateRequest 创建账号请求。
type AccountCreateRequest struct {
	AccountName string `json:"account_name" binding:"required"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email" binding:"omitempty,email"`
	Mobile      string `json:"mobile"`
	Password    string `json:"password" binding:"required"`
	// Status 状态（默认启用，nil 视为 true）。
	Status *bool `json:"status"`
	// AllowConsole 是否允许进入管理控制台，默认 true（nil 视为 true）。
	AllowConsole *bool  `json:"allow_console"`
	Remark       string `json:"remark"`
	// GroupIDs 所属账号组 ID 列表。
	GroupIDs []int64 `json:"group_ids"`
}

// Create 创建账号。
func (s *AccountLogic) Create(ctx context.Context, req *AccountCreateRequest) (*model.Account, error) {
	req.AccountName = strings.TrimSpace(req.AccountName)
	if req.AccountName == "" {
		return nil, errors.New("账号名不能为空")
	}

	// 账号名唯一性检查
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Account{}).Where("account_name = ?", req.AccountName).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("账号名已存在")
	}

	// 密码强度校验
	if msg := s.password.Validate(req.Password, req.AccountName); msg != "" {
		return nil, errors.New(msg)
	}

	hashed, err := hash.BcryptHashDefault(req.Password)
	if err != nil {
		return nil, err
	}

	// 默认允许进入控制台 / 默认启用（管理员创建），nil 视为 true
	allowConsole := true
	if req.AllowConsole != nil {
		allowConsole = *req.AllowConsole
	}
	status := true
	if req.Status != nil {
		status = *req.Status
	}
	now := time.Now()

	account := model.Account{
		AccountName:       req.AccountName,
		DisplayName:       req.DisplayName,
		Email:             nullableString(req.Email),
		Mobile:            nullableString(req.Mobile),
		Password:          hashed,
		Status:            status,
		AllowConsole:      allowConsole,
		Remark:            req.Remark,
		PasswordChangedAt: &now,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&account).Error; err != nil {
			return err
		}
		// 关联账号组（校验组 ID 均有效，避免静默忽略不存在的组）
		if len(req.GroupIDs) > 0 {
			unique := uniqueIDs(req.GroupIDs)
			var groups []model.Group
			if err := tx.Where("id IN ?", unique).Find(&groups).Error; err != nil {
				return err
			}
			if len(groups) != len(unique) {
				return errors.New("存在无效的账号组")
			}
			if err := tx.Model(&account).Association("Groups").Replace(groups); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// 并发下唯一约束兜底：转成友好提示，避免暴露原始 DB 错误
		return nil, normalizeDuplicateError(err)
	}

	logger.InfoCtx(ctx, "account created", logger.Int64("account_id", account.ID), logger.String("account_name", account.AccountName))
	return s.GetByID(ctx, account.ID)
}

// AccountUpdateRequest 更新账号请求。
type AccountUpdateRequest struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email" binding:"omitempty,email"`
	Mobile      string `json:"mobile"`
	Status      *bool  `json:"status"`
	// AllowConsole 是否允许进入管理控制台（nil 表示不修改）。
	AllowConsole *bool  `json:"allow_console"`
	Remark       string `json:"remark"`
	// GroupIDs 所属账号组 ID 列表（nil 表示不修改，空数组表示清空）。
	GroupIDs []int64 `json:"group_ids"`
}

// Update 更新账号。
func (s *AccountLogic) Update(ctx context.Context, req *AccountUpdateRequest) (*model.Account, error) {
	var account model.Account
	if err := s.db.WithContext(ctx).First(&account, req.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("账号不存在")
		}
		return nil, err
	}

	updates := map[string]any{
		"display_name": req.DisplayName,
		"email":        nullableString(req.Email),
		"mobile":       nullableString(req.Mobile),
		"remark":       req.Remark,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.AllowConsole != nil {
		updates["allow_console"] = *req.AllowConsole
	}
	// 明确禁用账号：吊销其全部刷新令牌，使已签发会话立即失效
	disable := req.Status != nil && !*req.Status

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&account).Updates(updates).Error; err != nil {
			return err
		}
		if disable {
			if err := revokeAccountTokens(ctx, tx, account.ID, model.RefreshTokenRevokeRevoke); err != nil {
				return err
			}
		}
		// 关联账号组（校验组 ID 均有效，避免静默忽略不存在的组）
		if req.GroupIDs != nil {
			var groups []model.Group
			if len(req.GroupIDs) > 0 {
				unique := uniqueIDs(req.GroupIDs)
				if err := tx.Where("id IN ?", unique).Find(&groups).Error; err != nil {
					return err
				}
				if len(groups) != len(unique) {
					return errors.New("存在无效的账号组")
				}
			}
			if err := tx.Model(&account).Association("Groups").Replace(groups); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 组关联变更：账号权限来源包含所属组的策略，须失效其权限缓存
	if s.permLogic != nil && req.GroupIDs != nil {
		s.permLogic.InvalidatePermissionCache(ctx, model.PrincipalTypeAccount, account.ID)
	}
	// 账号信息（状态/AllowConsole 等）变更：失效认证中间件的账号缓存
	s.invalidateAccountCache(ctx, account.ID)

	logger.InfoCtx(ctx, "account updated", logger.Int64("account_id", account.ID))
	return s.GetByID(ctx, account.ID)
}

// Delete 删除账号。
// 同时清理授权关系（PolicyAttachment）与账号组关联。
func (s *AccountLogic) Delete(ctx context.Context, id int64) error {
	var account model.Account
	if err := s.db.WithContext(ctx).First(&account, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("账号不存在")
		}
		return err
	}

	// 保护内置管理员账号（超级管理员不可删除）
	if account.IsAdmin {
		return errors.New("不能删除内置管理员账号")
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清理授权关系
		if err := tx.Where("principal_type = ? AND principal_id = ?",
			model.PrincipalTypeAccount, id).Delete(&model.PolicyAttachment{}).Error; err != nil {
			return err
		}
		// 清理账号组关联
		if err := tx.Model(&account).Association("Groups").Clear(); err != nil {
			return err
		}
		// 清理刷新令牌记录（账号已删除，记录一并清除）
		if err := tx.Where("account_id = ?", id).Delete(&model.RefreshToken{}).Error; err != nil {
			return err
		}
		// 清理密码历史记录（避免残留孤儿哈希）
		if err := tx.Where("account_id = ?", id).Delete(&model.PasswordHistory{}).Error; err != nil {
			return err
		}
		// 删除账号：物理删除，释放 account_name/email/mobile 唯一索引。
		// （若用软删除，软删记录仍占用唯一索引，删除后无法重建同名账号。）
		return tx.Unscoped().Delete(&account).Error
	})
	if err != nil {
		return err
	}

	// 账号已删除：失效其权限缓存，避免旧缓存键（perm:acct:{id}）命中残留权限
	if s.permLogic != nil {
		s.permLogic.InvalidatePermissionCache(ctx, model.PrincipalTypeAccount, id)
	}
	// 账号已删除：失效认证中间件的账号缓存，避免旧缓存账号被继续放行
	s.invalidateAccountCache(ctx, id)
	return nil
}

// ChangePasswordRequest 修改密码请求。
type ChangePasswordRequest struct {
	ID          int64  `json:"id"`
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword 修改密码（需校验旧密码）。
func (s *AccountLogic) ChangePassword(ctx context.Context, req *ChangePasswordRequest) error {
	var account model.Account
	if err := s.db.WithContext(ctx).First(&account, req.ID).Error; err != nil {
		return errors.New("账号不存在")
	}

	// 校验旧密码
	if err := hash.BcryptCompare(account.Password, req.OldPassword); err != nil {
		return errors.New("旧密码错误")
	}

	// 新密码强度校验
	if msg := s.password.Validate(req.NewPassword, account.AccountName); msg != "" {
		return errors.New(msg)
	}

	// 密码历史检查（防止重复使用最近用过的密码）
	historyCount := s.password.policy.HistoryCount
	if msg, err := CheckPasswordReuse(ctx, s.db, account.ID, req.NewPassword, historyCount); err != nil {
		return err
	} else if msg != "" {
		return errors.New(msg)
	}

	hashed, err := hash.BcryptHashDefault(req.NewPassword)
	if err != nil {
		return err
	}

	// 记录旧密码到历史 + 更新新密码与修改时间 + 吊销该账号全部刷新令牌（同一事务）。
	// 密码已变更，旧会话的刷新令牌应全部失效。
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := RememberPassword(ctx, tx, account.ID, account.Password, historyCount); err != nil {
			return err
		}
		if err := tx.Model(&account).Updates(map[string]any{
			"password":            hashed,
			"password_changed_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		return revokeAccountTokens(ctx, tx, account.ID, model.RefreshTokenRevokeRevoke)
	}); err != nil {
		return err
	}

	// 改密后账号信息（password_changed_at）已变更：失效认证中间件的账号缓存
	s.invalidateAccountCache(ctx, account.ID)
	return nil
}

// ResetPasswordRequest 重置密码请求。
type ResetPasswordRequest struct {
	ID       int64  `json:"id"`
	Password string `json:"password" binding:"required"`
}

// ResetPassword 重置密码（管理员操作，不校验旧密码）。
// 同时清除登录失败计数与锁定状态。
func (s *AccountLogic) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
	var account model.Account
	if err := s.db.WithContext(ctx).First(&account, req.ID).Error; err != nil {
		return errors.New("账号不存在")
	}

	// 新密码强度校验
	if msg := s.password.Validate(req.Password, account.AccountName); msg != "" {
		return errors.New(msg)
	}

	// 密码历史检查（防止重复使用最近用过的密码）
	historyCount := s.password.policy.HistoryCount
	if msg, err := CheckPasswordReuse(ctx, s.db, account.ID, req.Password, historyCount); err != nil {
		return err
	} else if msg != "" {
		return errors.New(msg)
	}

	hashed, err := hash.BcryptHashDefault(req.Password)
	if err != nil {
		return err
	}

	// 记录旧密码到历史 + 更新新密码、修改时间，清除锁定状态，并吊销全部刷新令牌（同一事务）。
	// 密码已变更，旧会话的刷新令牌应全部失效。
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := RememberPassword(ctx, tx, account.ID, account.Password, historyCount); err != nil {
			return err
		}
		if err := tx.Model(&account).Updates(map[string]any{
			"password":            hashed,
			"login_fail_count":    0,
			"locked_until":        nil,
			"password_changed_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		return revokeAccountTokens(ctx, tx, account.ID, model.RefreshTokenRevokeRevoke)
	}); err != nil {
		return err
	}

	// 重置密码后账号信息（锁定状态/password_changed_at）已变更：失效认证中间件的账号缓存
	s.invalidateAccountCache(ctx, account.ID)
	return nil
}

// toAccountItem 转换为列表项。
func toAccountItem(a model.Account) AccountItem {
	return AccountItem{
		ID:           a.ID,
		AccountName:  a.AccountName,
		DisplayName:  a.DisplayName,
		Email:        derefString(a.Email),
		Mobile:       derefString(a.Mobile),
		Status:       a.Status,
		AllowConsole: a.AllowConsole,
		Remark:       a.Remark,
		CreatedAt:    a.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// derefString 解引用字符串指针，nil 返回空字符串。
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
