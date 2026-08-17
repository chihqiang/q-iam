package db

import (
	"time"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/hash"
	"gorm.io/gorm"
)

// Migrate 执行数据库迁移与种子数据初始化。
// 由配置控制两个阶段（均默认开启）：
//   - AutoMigrate：是否自动迁移表结构（建表/加列等）；关闭时需自行保证表结构已存在（如由外部迁移工具管理）；
//   - SeedData：是否初始化基础数据（内置 admin 账号、系统内置策略等）。
func Migrate(db *gorm.DB, cfg config.MigrationConfig) error {
	if cfg.AutoMigrate {
		if err := db.AutoMigrate(
			&model.Account{},
			&model.Group{},
			&model.Policy{},
			&model.PolicyStatement{},
			&model.DataScope{},
			&model.PolicyAttachment{},
			&model.App{},
			&model.AuditLog{},
			&model.PasswordHistory{},
			&model.RefreshToken{},
			&model.KeyStoreItem{},
		); err != nil {
			return err
		}
	}
	// 基础数据初始化依赖表结构已存在；关闭 AutoMigrate 时需自行建表。
	if !cfg.SeedData {
		return nil
	}
	if err := seed(db); err != nil {
		return err
	}
	return seedConsolePolicy(db)
}

// ptrString 返回字符串指针。
func ptrString(s string) *string {
	return &s
}

// seed 初始化默认数据：超级管理员账号、系统内置策略及其授权。
func seed(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.Account{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	password, err := hash.BcryptHashDefault("admin123")
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// 内置超级管理员账号（初始密码 admin123，请及时修改），ID 由数据库自增生成
		now := time.Now()
		admin := model.Account{
			AccountName:       model.AdminAccountName,
			DisplayName:       "超级管理员",
			Email:             ptrString("admin@local"),
			Password:          password,
			Status:            true,
			AllowConsole:      true,
			Remark:            "内置超级管理员",
			PasswordChangedAt: &now,
		}
		if err := tx.Create(&admin).Error; err != nil {
			return err
		}

		// 系统内置策略：管理员全权限（system 类型，不可删除），规则拆分到明细表
		adminPolicy := model.Policy{
			Name:        "AdministratorAccess",
			Description: "系统内置策略，允许管理全部资源",
			Type:        model.PolicyTypeSystem,
			Status:      true,
			Statements: []model.PolicyStatement{
				{Effect: "Allow", Action: "*"},
			},
		}
		if err := tx.Create(&adminPolicy).Error; err != nil {
			return err
		}

		// 将系统策略授权给超级管理员（account 主体）
		attachment := model.PolicyAttachment{
			PrincipalType: model.PrincipalTypeAccount,
			PrincipalID:   admin.ID,
			PolicyID:      adminPolicy.ID,
		}
		return tx.Create(&attachment).Error
	})
}

// seedConsolePolicy 初始化系统内置策略：管理控制台访问（ConsoleAccess）。
//
// 将当前系统（管理控制台）实际使用的全部动作录入为一条 system 内置策略，
// 普通账号/账号组/应用授权该策略即可访问控制台对应功能，无需手动逐个创建策略。
// 幂等：按策略名查重，不存在才创建。
func seedConsolePolicy(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.Policy{}).Where("name = ?", "ConsoleAccess").Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	policy := model.Policy{
		Name:        "ConsoleAccess",
		Description: "系统内置策略，允许访问管理控制台各模块（账号/账号组/权限策略/授权/应用/审计/数据清理）",
		Type:        model.PolicyTypeSystem,
		Status:      true,
		Statements: []model.PolicyStatement{
			// 身份管理
			{Description: "账号管理", Effect: "Allow", Action: "iam:account:read,iam:account:write", Sort: 1},
			{Description: "账号组管理", Effect: "Allow", Action: "iam:group:read,iam:group:write", Sort: 2},
			// 权限管理
			{Description: "权限策略管理", Effect: "Allow", Action: "iam:policy:read,iam:policy:write", Sort: 3},
			{Description: "授权管理", Effect: "Allow", Action: "iam:grant", Sort: 4},
			// 集成管理
			{Description: "应用管理", Effect: "Allow", Action: "iam:app:read,iam:app:write", Sort: 5},
			// 安全审计
			{Description: "操作审计", Effect: "Allow", Action: "iam:audit:read", Sort: 6},
			// 系统管理
			{Description: "历史数据清理", Effect: "Allow", Action: "iam:system:cleanup", Sort: 7},
		},
	}
	return db.Create(&policy).Error
}
