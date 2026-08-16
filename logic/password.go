package logic

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"chihqiang/q-iam/config"
	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/hash"
	"gorm.io/gorm"
)

// PasswordValidator 密码强度校验器。
// 依据全局安全配置中的密码策略（password_policy）校验密码强度。
type PasswordValidator struct {
	policy config.PasswordPolicyConfig
}

// NewPasswordValidator 创建密码强度校验器。
func NewPasswordValidator(policy config.PasswordPolicyConfig) *PasswordValidator {
	return &PasswordValidator{policy: policy}
}

// Validate 校验密码强度。
// accountName 用于"禁止密码包含账号名"策略。
// 返回空字符串表示通过，否则返回具体错误信息。
func (v *PasswordValidator) Validate(password, accountName string) string {
	p := v.policy

	// 最小/最大长度：按 Unicode 字符数计算（避免多字节 UTF-8 下字节数与字符数偏差）
	length := utf8.RuneCountInString(password)
	if p.MinLength > 0 && length < p.MinLength {
		return fmt.Sprintf("密码长度不能少于 %d 位", p.MinLength)
	}
	if p.MaxLength > 0 && length > p.MaxLength {
		return fmt.Sprintf("密码长度不能超过 %d 位", p.MaxLength)
	}

	hasUpper, hasLower, hasDigit, hasSpecial := classifyPassword(password)

	// 大写字母要求
	if p.RequireUppercase && !hasUpper {
		return "密码必须包含至少一个大写字母"
	}
	// 小写字母要求
	if p.RequireLowercase && !hasLower {
		return "密码必须包含至少一个小写字母"
	}
	// 数字要求
	if p.RequireDigit && !hasDigit {
		return "密码必须包含至少一个数字"
	}
	// 特殊字符要求
	if p.RequireSpecial && !hasSpecial {
		return "密码必须包含至少一个特殊字符"
	}

	// 最少不同字符数
	if p.MinUniqueChars > 0 {
		unique := make(map[rune]bool)
		for _, ch := range password {
			unique[ch] = true
		}
		if len(unique) < p.MinUniqueChars {
			return fmt.Sprintf("密码至少需要 %d 种不同字符", p.MinUniqueChars)
		}
	}

	// 禁止包含账号名
	if p.ForbidAccountName && accountName != "" &&
		strings.Contains(strings.ToLower(password), strings.ToLower(accountName)) {
		return "密码不能包含账号名"
	}

	return ""
}

// IsPasswordExpired 判断密码是否已超过有效期（password_policy.expire_days）。
// expireDays <= 0 表示不限期；changedAt 为空（历史账号升级而来）视为未过期，
// 避免启用策略后把存量账号全部锁死。
func IsPasswordExpired(changedAt *time.Time, expireDays int, now time.Time) bool {
	if expireDays <= 0 || changedAt == nil {
		return false
	}
	return now.Sub(*changedAt) > time.Duration(expireDays)*24*time.Hour
}

// CheckPasswordReuse 校验新密码是否与当前密码或最近历史密码重复。
// historyCount <= 0 表示不启用历史检查。
// 返回 (拒绝原因, error)：msg 非空表示命中历史密码需拒绝；error 为查询失败。
func CheckPasswordReuse(ctx context.Context, db *gorm.DB, accountID int64, newPassword string, historyCount int) (string, error) {
	if historyCount <= 0 {
		return "", nil
	}

	// 当前密码
	var current model.Account
	if err := db.WithContext(ctx).Select("password").First(&current, accountID).Error; err != nil {
		return "", err
	}
	if err := hash.BcryptCompare(current.Password, newPassword); err == nil {
		return "新密码不能与当前密码相同", nil
	}

	// 最近 historyCount 条历史密码
	var hist []model.PasswordHistory
	if err := db.WithContext(ctx).Where("account_id = ?", accountID).
		Order("id DESC").Limit(historyCount).Find(&hist).Error; err != nil {
		return "", err
	}
	for _, h := range hist {
		if err := hash.BcryptCompare(h.PasswordHash, newPassword); err == nil {
			return "新密码不能与最近使用过的密码相同", nil
		}
	}
	return "", nil
}

// RememberPassword 将密码哈希记入历史并只保留最近 historyCount 条。
// 应在密码被替换前调用（记录旧密码）。historyCount <= 0 时不做任何事。
// 调用方需自行保证与密码更新在同一事务内（本函数不开启事务）。
func RememberPassword(ctx context.Context, db *gorm.DB, accountID int64, passwordHash string, historyCount int) error {
	if historyCount <= 0 {
		return nil
	}
	if err := db.WithContext(ctx).Create(&model.PasswordHistory{
		AccountID:    accountID,
		PasswordHash: passwordHash,
	}).Error; err != nil {
		return err
	}

	// 只保留最近 historyCount 条（含刚插入的一条）
	var keepIDs []int64
	if err := db.WithContext(ctx).Model(&model.PasswordHistory{}).
		Where("account_id = ?", accountID).
		Order("id DESC").Limit(historyCount).Pluck("id", &keepIDs).Error; err != nil {
		return err
	}
	if len(keepIDs) == 0 {
		return nil
	}
	return db.WithContext(ctx).Where("account_id = ? AND id NOT IN ?", accountID, keepIDs).
		Delete(&model.PasswordHistory{}).Error
}

// classifyPassword 统计密码中各类字符。
// 返回：是否有大写、小写、数字、特殊字符。
func classifyPassword(s string) (hasUpper, hasLower, hasDigit, hasSpecial bool) {
	for _, ch := range s {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case ch >= 0x21 && ch <= 0x7e: // ASCII 可见标点符号（! ~ / 排除字母数字）
			hasSpecial = true
		}
	}
	return
}
