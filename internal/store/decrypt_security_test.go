// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"crypto/rand"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetEncryptedSetting_KeyMismatch 验证：密钥不匹配时必须返回 error，而不是静默返回密文
// 这是安全与数据一致性的关键保障：
//   - 密钥丢失/更换后，用户应看到明确的错误提示，而不是看到 enc:xxxx 形式的乱码
//   - 静默返回密文会让用户误以为数据损坏，且无法区分"密钥问题"与"数据损坏"
func TestGetEncryptedSetting_KeyMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// 用密钥 A 加密
	encKeyA := make([]byte, 32)
	if _, err := rand.Read(encKeyA); err != nil {
		t.Fatalf("生成密钥A失败: %v", err)
	}

	db, err := Init(dbPath, encKeyA)
	if err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	defer db.Close()

	// 用密钥 A 写入加密设置
	key := "secret_key"
	value := "secret_value_to_protect"
	if err := SetEncryptedSetting(db, key, value, encKeyA); err != nil {
		t.Fatalf("写入加密设置失败: %v", err)
	}

	// 用密钥 B 读取，应该返回 error（密钥不匹配）
	encKeyB := make([]byte, 32)
	if _, err := rand.Read(encKeyB); err != nil {
		t.Fatalf("生成密钥B失败: %v", err)
	}

	got, err := GetEncryptedSetting(db, key, encKeyB)
	if err == nil {
		t.Errorf("密钥不匹配时应返回 error，实际返回 nil, 值=%q", got)
	}
	// 错误信息不应泄漏明文值
	if err != nil && strings.Contains(err.Error(), value) {
		t.Errorf("错误信息不应泄漏明文值: %s", err.Error())
	}
	// 返回值不应是明文（不能让调用方误以为解密成功）
	if got == value {
		t.Errorf("密钥不匹配时不应返回明文值")
	}
}

// TestDecryptField_KeyMismatch 验证 decryptField 在密钥不匹配时返回 error
func TestDecryptField_KeyMismatch(t *testing.T) {
	encKeyA := make([]byte, 32)
	if _, err := rand.Read(encKeyA); err != nil {
		t.Fatalf("生成密钥A失败: %v", err)
	}
	encKeyB := make([]byte, 32)
	if _, err := rand.Read(encKeyB); err != nil {
		t.Fatalf("生成密钥B失败: %v", err)
	}

	// 用密钥 A 加密
	encrypted, err := encryptField("secret_plaintext", encKeyA)
	if err != nil {
		t.Fatalf("encryptField 失败: %v", err)
	}

	// 用密钥 B 解密，应返回 error
	plaintext, err := decryptField(encrypted, encKeyB)
	if err == nil {
		t.Errorf("密钥不匹配时应返回 error，实际返回 nil, 值=%q", plaintext)
	}
}

// TestDecryptField_PlaintextCompatibility 验证旧版明文数据（无 enc: 前缀）仍能正常返回
// 这是向后兼容的关键：不能因为加强安全就破坏旧数据的可读性
func TestDecryptField_PlaintextCompatibility(t *testing.T) {
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}

	// 旧版明文数据（无 enc: 前缀）
	plaintext := "legacy_plain_value_without_enc_prefix"
	got, err := decryptField(plaintext, encKey)
	if err != nil {
		t.Errorf("旧版明文数据应能正常返回，不应报错: %v", err)
	}
	if got != plaintext {
		t.Errorf("旧版明文值不匹配: 期望 %q, 实际 %q", plaintext, got)
	}
}

// TestDecryptField_EmptyString 验证空字符串处理
func TestDecryptField_EmptyString(t *testing.T) {
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}

	got, err := decryptField("", encKey)
	if err != nil {
		t.Errorf("空字符串不应报错: %v", err)
	}
	if got != "" {
		t.Errorf("空字符串应返回空字符串, 实际 %q", got)
	}
}

// TestDecryptField_NilKey 验证 encKey 为 nil 时跳过解密（用于禁用加密的场景）
func TestDecryptField_NilKey(t *testing.T) {
	plaintext := "some_value"
	got, err := decryptField(plaintext, nil)
	if err != nil {
		t.Errorf("nil encKey 时不应报错: %v", err)
	}
	if got != plaintext {
		t.Errorf("nil encKey 时应返回原值, 期望 %q, 实际 %q", plaintext, got)
	}
}
