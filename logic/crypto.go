package logic

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// AES-256-GCM 静态数据加密器，用于加密存储敏感字段（如 AppSecret）。
//
// 密钥版本化：加密始终用当前密钥，密文前缀标记版本：
//   - enc:v2:<base64(nonce||ciphertext)> —— 当前密钥（security.cipher.key，缺省回退 JWT Secret）
//   - enc:v1:<base64(nonce||ciphertext)> —— 存量格式（上一版本密钥 previous_key，未配置则用当前密钥）
//
// 轮换密钥时把旧密钥填 previous_key、新密钥填 key 重启即可平滑过渡，
// 存量 enc:v1 用 previous_key 解、新密文用 key 加密，已存 AppSecret 不会失效。

const (
	encV1Prefix  = "enc:v1:"
	encV2Prefix  = "enc:v2:"
	gcmOverhead  = 16
	versionNonce = 12 // GCM 标准 nonce 长度
)

// Cipher 静态数据加密器。
type Cipher struct {
	aead     cipher.AEAD // 当前版本密钥
	prevAEAD cipher.AEAD // 上一版本密钥（解密 enc:v1 存量），nil 表示无
}

// NewCipher 创建加密器。
// currentKey 为当前密钥；prevKey 为上一版本密钥（可为空）。
// 密钥经 SHA-256 派生为 32 字节 AES 密钥。
func NewCipher(currentKey, prevKey string) (*Cipher, error) {
	aead, err := newAEAD(currentKey)
	if err != nil {
		return nil, err
	}
	var prevAEAD cipher.AEAD
	if prevKey != "" {
		prevAEAD, err = newAEAD(prevKey)
		if err != nil {
			return nil, err
		}
	}
	return &Cipher{aead: aead, prevAEAD: prevAEAD}, nil
}

// newAEAD 从密钥字符串派生 AES-256-GCM。
func newAEAD(key string) (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal 用指定 aead 加密，返回 nonce||ciphertext。
func seal(aead cipher.AEAD, plain string) ([]byte, error) {
	nonce := make([]byte, versionNonce)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

// open 用指定 aead 解密 nonce||ciphertext。
func open(aead cipher.AEAD, raw []byte) (string, error) {
	if len(raw) < versionNonce+gcmOverhead {
		return "", errors.New("cipher: ciphertext too short")
	}
	nonce := raw[:versionNonce]
	body := raw[versionNonce:]
	plain, err := aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("cipher: decrypt failed: %w", err)
	}
	return string(plain), nil
}

// Encrypt 加密字符串，返回带版本前缀的密文（enc:v2）。空字符串直接返回空。
func (c *Cipher) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	ct, err := seal(c.aead, plain)
	if err != nil {
		return "", err
	}
	return encV2Prefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt 解密带版本前缀的密文。空字符串直接返回空。
//   - enc:v2 → 当前密钥；
//   - enc:v1 → 上一版本密钥（未配置则当前密钥，兼容未轮换场景）；
//   - 无法识别前缀时按原始值返回（兼容历史明文数据）。
func (c *Cipher) Decrypt(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	switch {
	case strings.HasPrefix(encrypted, encV2Prefix):
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, encV2Prefix))
		if err != nil {
			return "", fmt.Errorf("cipher: base64 decode failed: %w", err)
		}
		return open(c.aead, raw)
	case strings.HasPrefix(encrypted, encV1Prefix):
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, encV1Prefix))
		if err != nil {
			return "", fmt.Errorf("cipher: base64 decode failed: %w", err)
		}
		if c.prevAEAD != nil {
			return open(c.prevAEAD, raw)
		}
		return open(c.aead, raw)
	default:
		return encrypted, nil
	}
}
