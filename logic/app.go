package logic

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"github.com/chihqiang/infra-go/stringx"
	"gorm.io/gorm"
)

// AppLogic 应用管理逻辑：应用 CRUD 与客户端密钥签发/重置。
type AppLogic struct {
	db     *gorm.DB
	cipher *Cipher
	// permLogic 权限逻辑（应用列表按数据范围过滤，nil 表示不过滤）。
	permLogic *PermissionLogic
}

// NewAppLogic 创建应用管理逻辑。
func NewAppLogic(db *gorm.DB, cipher *Cipher) *AppLogic {
	return &AppLogic{db: db, cipher: cipher}
}

// SetPermissionLogic 注入权限逻辑（应用列表按数据范围过滤）。
func (s *AppLogic) SetPermissionLogic(permLogic *PermissionLogic) {
	s.permLogic = permLogic
}

// AppListRequest 应用列表请求。
type AppListRequest struct {
	PageRequest
	Status *bool  `form:"status"`
	Key    string `form:"key"`
}

// AppItem 应用列表项（隐藏密钥）。
type AppItem struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	AppID          string `json:"app_id"`
	Description    string `json:"description"`
	OwnerAccountID int64  `json:"owner_account_id"`
	// CallbackURL 授权回调地址（authorization_code 模式使用）。
	CallbackURL string `json:"callback_url"`
	GrantType   string `json:"grant_type"`
	Status      bool   `json:"status"`
	CreatedAt   string `json:"created_at"`
}

// List 应用分页列表（按数据权限过滤可见范围）。
// accountID<=0（admin/系统主体）不过滤；否则按该账号对 iam:app:read 的
// 数据范围过滤（self=本人拥有，group=可见组成员拥有）。
func (s *AppLogic) List(ctx context.Context, accountID int64, req *AppListRequest) (*PageResponse[AppItem], error) {
	var apps []model.App
	var total int64

	query := s.db.WithContext(ctx).Model(&model.App{})
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}
	if req.Key != "" {
		like := "%" + req.Key + "%"
		query = query.Where("name LIKE ? OR app_id LIKE ?", like, like)
	}

	// 数据范围过滤（非 admin 账号按 iam:app:read 的数据范围可见性过滤）
	s.applyAppScope(ctx, query, accountID)

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (req.Page - 1) * req.Size
	if err := query.Order("id ASC").Offset(offset).Limit(req.Size).Find(&apps).Error; err != nil {
		return nil, err
	}

	items := make([]AppItem, 0, len(apps))
	for _, a := range apps {
		items = append(items, toAppItem(a))
	}
	return &PageResponse[AppItem]{Data: items, Total: total}, nil
}

// applyAppScope 按当前账号对应用资源（iam:app:read）的数据范围过滤查询。
// accountID<=0 或权限逻辑未注入时不过滤（全量）。数据范围加载失败时保守降级为仅本人拥有。
// 语义：
//   - all/未限定 → 全量；
//   - self → 仅本人拥有的应用（owner_account_id=本人）；
//   - group → 可见组成员拥有的应用（owner 属于指定组）；
//   - attribute → 已降级为 self。
func (s *AppLogic) applyAppScope(ctx context.Context, query *gorm.DB, accountID int64) {
	if accountID <= 0 || s.permLogic == nil {
		return
	}
	appTable := model.App{}.TableName() // q_iam_apps

	scope, err := s.permLogic.DataScopeForAction(ctx, "iam:app:read", accountID)
	if err != nil {
		logger.WarnCtx(ctx, "load app data scope failed, fallback to self",
			logger.Err(err), logger.Int64("account_id", accountID))
		query.Where(appTable+".owner_account_id = ?", accountID)
		return
	}
	if scope.All {
		return
	}

	conds := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if scope.SelfOnly {
		conds = append(conds, appTable+".owner_account_id = ?")
		args = append(args, accountID)
	}
	if len(scope.GroupIDs) > 0 {
		// 应用归属者（owner_account_id）属于可见组的应用可见
		conds = append(conds, "EXISTS (SELECT 1 FROM q_iam_account_groups ag WHERE ag.account_id = "+appTable+".owner_account_id AND ag.group_id IN ?)")
		args = append(args, scope.GroupIDs)
	}
	if len(conds) == 0 {
		query.Where("1 = 0")
		return
	}
	query.Where(strings.Join(conds, " OR "), args...)
}

// AllList 返回全部启用的应用（用于下拉选择）。
// accountID<=0 返回全部；否则按数据范围过滤，防止下拉选择越权应用。
func (s *AppLogic) AllList(ctx context.Context, accountID int64) ([]model.App, error) {
	var apps []model.App
	query := s.db.WithContext(ctx).Where("status = ?", true).Order("id ASC")
	s.applyAppScope(ctx, query, accountID)
	err := query.Find(&apps).Error
	return apps, err
}

// CanViewApp 判断账号 viewerID 是否有权查看应用 targetID 的详情。
// 委托 PermissionLogic.CanAccessPrincipal 基于 iam:app:read 数据范围判定。
// 供详情接口做数据范围校验，防止「列表已过滤但详情接口按 ID 枚举绕过」越权。
// 权限逻辑未注入时保守拒绝（返回 false）。
func (s *AppLogic) CanViewApp(ctx context.Context, viewerID, targetID int64) (bool, error) {
	if s.permLogic == nil {
		return false, nil
	}
	return s.permLogic.CanAccessPrincipal(ctx, viewerID, model.PrincipalTypeApp, targetID)
}

// GetByID 应用详情（隐藏密钥）。
func (s *AppLogic) GetByID(ctx context.Context, id int64) (*model.App, error) {
	var app model.App
	if err := s.db.WithContext(ctx).First(&app, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("应用不存在")
		}
		return nil, err
	}
	// 清空密钥，避免泄露
	app.AppSecret = ""
	return &app, nil
}

// AppCreateRequest 创建应用请求。
type AppCreateRequest struct {
	Name           string `json:"name" binding:"required"`
	Description    string `json:"description"`
	OwnerAccountID int64  `json:"owner_account_id"`
	CallbackURL    string `json:"callback_url"`
	GrantType      string `json:"grant_type" binding:"omitempty,oneof=client_credentials authorization_code"`
	// Status 状态（默认启用，nil 视为 true；与账号/账号组/策略创建语义一致）。
	Status *bool `json:"status"`
}

// AppCreateResponse 创建应用响应（含明文密钥，仅此一次返回）。
type AppCreateResponse struct {
	model.App
	AppSecret string `json:"app_secret"`
}

// Create 创建应用（自动签发 AppID + AppSecret，密钥加密存储）。
func (s *AppLogic) Create(ctx context.Context, req *AppCreateRequest) (*AppCreateResponse, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, errors.New("应用名称不能为空")
	}

	// 签发客户端 ID 与密钥
	appID := "app-" + stringx.RandId()
	appSecret := stringx.RandId() + stringx.RandId()

	encryptedSecret, err := s.cipher.Encrypt(appSecret)
	if err != nil {
		return nil, err
	}

	grantType := req.GrantType
	if grantType == "" {
		grantType = model.AppGrantTypeClientCredentials
	}

	// 默认启用（与账号/账号组/策略创建语义一致），nil 视为 true
	status := true
	if req.Status != nil {
		status = *req.Status
	}

	app := model.App{
		Name:           req.Name,
		AppID:          appID,
		AppSecret:      encryptedSecret,
		Description:    req.Description,
		OwnerAccountID: req.OwnerAccountID,
		CallbackURL:    req.CallbackURL,
		GrantType:      grantType,
		Status:         status,
	}

	if err := s.db.WithContext(ctx).Create(&app).Error; err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "app created", logger.Int64("app_id", app.ID), logger.String("name", app.Name))
	return &AppCreateResponse{App: app, AppSecret: appSecret}, nil
}

// AppUpdateRequest 更新应用请求。
type AppUpdateRequest struct {
	ID          int64  `json:"id"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	CallbackURL string `json:"callback_url"`
	GrantType   string `json:"grant_type" binding:"omitempty,oneof=client_credentials authorization_code"`
	Status      *bool  `json:"status"`
}

// Update 更新应用。
func (s *AppLogic) Update(ctx context.Context, req *AppUpdateRequest) (*model.App, error) {
	var app model.App
	if err := s.db.WithContext(ctx).First(&app, req.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("应用不存在")
		}
		return nil, err
	}

	updates := map[string]any{
		"name":         req.Name,
		"description":  req.Description,
		"callback_url": req.CallbackURL,
	}
	// grant_type 为空表示不修改，避免未传值时覆盖已有授权类型
	if req.GrantType != "" {
		updates["grant_type"] = req.GrantType
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if err := s.db.WithContext(ctx).Model(&app).Updates(updates).Error; err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "app updated", logger.Int64("app_id", app.ID))
	return s.GetByID(ctx, app.ID)
}

// Delete 删除应用（级联清理授权关系）。
func (s *AppLogic) Delete(ctx context.Context, id int64) error {
	var app model.App
	if err := s.db.WithContext(ctx).First(&app, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("应用不存在")
		}
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清理授权关系
		if err := tx.Where("principal_type = ? AND principal_id = ?",
			model.PrincipalTypeApp, id).Delete(&model.PolicyAttachment{}).Error; err != nil {
			return err
		}
		// 物理删除：释放 app_id 唯一索引，避免删除后重建冲突
		return tx.Unscoped().Delete(&app).Error
	})
}

// ResetSecret 重置应用客户端密钥（返回明文，仅此一次）。
func (s *AppLogic) ResetSecret(ctx context.Context, id int64) (string, error) {
	var app model.App
	if err := s.db.WithContext(ctx).First(&app, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("应用不存在")
		}
		return "", err
	}

	appSecret := stringx.RandId() + stringx.RandId()
	encryptedSecret, err := s.cipher.Encrypt(appSecret)
	if err != nil {
		return "", err
	}

	if err := s.db.WithContext(ctx).Model(&app).Update("app_secret", encryptedSecret).Error; err != nil {
		return "", err
	}

	logger.InfoCtx(ctx, "app secret reset", logger.Int64("app_id", app.ID))
	return appSecret, nil
}

// VerifyCredential 校验应用凭证（client_id + client_secret），用于应用换取 Token。
func (s *AppLogic) VerifyCredential(ctx context.Context, appID, appSecret string) (*model.App, error) {
	var app model.App
	if err := s.db.WithContext(ctx).Where("app_id = ?", appID).First(&app).Error; err != nil {
		return nil, errors.New("应用凭证无效")
	}
	if !app.Status {
		return nil, errors.New("应用已被禁用")
	}

	decrypted, err := s.cipher.Decrypt(app.AppSecret)
	if err != nil {
		return nil, errors.New("应用凭证无效")
	}
	// 常量时间比较，避免时序侧信道
	if subtle.ConstantTimeCompare([]byte(decrypted), []byte(appSecret)) != 1 {
		return nil, errors.New("应用凭证无效")
	}
	return &app, nil
}

// toAppItem 转换为列表项。
func toAppItem(a model.App) AppItem {
	return AppItem{
		ID:             a.ID,
		Name:           a.Name,
		AppID:          a.AppID,
		Description:    a.Description,
		OwnerAccountID: a.OwnerAccountID,
		CallbackURL:    a.CallbackURL,
		GrantType:      a.GrantType,
		Status:         a.Status,
		CreatedAt:      a.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
