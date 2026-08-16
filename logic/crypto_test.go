package logic

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestCipherVersionedKeys 验证密钥版本化：
//   - 加密始终用当前密钥（enc:v2），解密可用；
//   - 存量 enc:v1 密文可用上一版本密钥解密；
//   - 轮换后：新密钥加密的 enc:v2 只能用新密钥解，但存量 enc:v1 仍可解。
func TestCipherVersionedKeys(t *testing.T) {
	// 1. 旧密钥加密（模拟存量 enc:v1 数据）
	oldCipher, err := NewCipher("old-key", "")
	if err != nil {
		t.Fatalf("new old cipher: %v", err)
	}
	// 直接构造 enc:v1 密文（用旧密钥 + encV1Prefix），模拟历史数据
	v1 := legacyV1Encrypt(t, oldCipher, "app-secret-abc")

	// 2. 轮换到新密钥，配置 previous_key 为旧密钥
	newCipher, err := NewCipher("new-key", "old-key")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}

	// 新加密产出 enc:v2，且能用新密钥解密
	enc2, err := newCipher.Encrypt("app-secret-xyz")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc2, encV2Prefix) {
		t.Fatalf("expected enc:v2 prefix, got %s", enc2)
	}
	plain2, err := newCipher.Decrypt(enc2)
	if err != nil {
		t.Fatalf("decrypt enc2: %v", err)
	}
	if plain2 != "app-secret-xyz" {
		t.Fatalf("decrypt enc2 mismatch: %q", plain2)
	}

	// 存量 enc:v1 用 previous_key 可解（轮换后不失效）
	plain1, err := newCipher.Decrypt(v1)
	if err != nil {
		t.Fatalf("decrypt legacy v1: %v", err)
	}
	if plain1 != "app-secret-abc" {
		t.Fatalf("decrypt v1 mismatch: %q", plain1)
	}
}

// TestCipherNoPreviousKey 验证未配置 previous_key 时，enc:v1 用当前密钥兜底解密。
func TestCipherNoPreviousKey(t *testing.T) {
	c, err := NewCipher("same-key", "")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	// 用同一密钥构造 enc:v1（模拟未轮换场景的存量数据）
	v1 := legacyV1Encrypt(t, c, "plain-1")

	plain, err := c.Decrypt(v1)
	if err != nil {
		t.Fatalf("decrypt v1 with current key: %v", err)
	}
	if plain != "plain-1" {
		t.Fatalf("decrypt mismatch: %q", plain)
	}

	// 明文透传兼容
	if p, _ := c.Decrypt("legacy-plaintext"); p != "legacy-plaintext" {
		t.Fatalf("plaintext passthrough failed: %q", p)
	}
	// 空串
	if p, _ := c.Decrypt(""); p != "" {
		t.Fatalf("empty should stay empty: %q", p)
	}
}

// legacyV1Encrypt 用旧格式（enc:v1）加密，模拟历史存量数据。
func legacyV1Encrypt(t *testing.T, c *Cipher, plain string) string {
	t.Helper()
	ct, err := seal(c.aead, plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return encV1Prefix + base64.StdEncoding.EncodeToString(ct)
}
