// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package secrets

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"douya/internal/pathutil"
)

// TestAESCipherImplementsCipher 验证 AESCipher 实现了 Cipher 接口，
// 并且加密→解密能还原原文。
// 生活类比：检查"加密锁"是否符合标准接口规范——锁上再打开，里面的东西应该原封不动。
func TestAESCipherImplementsCipher(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	// 编译期即可校验 AESCipher 实现了 Cipher 接口
	var c Cipher = NewCipher(key)

	plaintext := "你好，豆芽！这是一条加密测试消息。"
	ciphertext, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt 失败: %v", err)
	}
	if ciphertext == plaintext {
		t.Fatal("密文不应等于明文")
	}

	got, err := c.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt 失败: %v", err)
	}
	if got != plaintext {
		t.Fatalf("解密后得到 %q，期望 %q", got, plaintext)
	}
}

// TestCipherKey 验证 CipherKey 能从 AESCipher 提取底层密钥，
// 且对不暴露 Key() 方法的实现返回 nil。
func TestCipherKey(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	c := NewCipher(key)

	got := CipherKey(c)
	if string(got) != string(key) {
		t.Fatalf("CipherKey 返回 %v，期望 %v", got, key)
	}

	// 不暴露 Key() 方法的实现应返回 nil
	var noop Cipher = noopCipherImpl{}
	if CipherKey(noop) != nil {
		t.Fatal("不暴露 Key() 的实现应返回 nil")
	}

	// nil 接口应安全返回 nil，不 panic
	if CipherKey(nil) != nil {
		t.Fatal("nil Cipher 应返回 nil")
	}
}

// noopCipherImpl 是一个不暴露 Key() 的 Cipher 实现，用于测试 CipherKey 的回退逻辑
type noopCipherImpl struct{}

func (noopCipherImpl) Encrypt(plaintext string) (string, error) { return plaintext, nil }
func (noopCipherImpl) Decrypt(ciphertext string) (string, error) { return ciphertext, nil }

// TestRestrictKeyFileACLWindows_NonWindows 验证非 Windows 平台 pathutil.RestrictACLWindows 的行为。
// Windows 平台跳过该测试（避免实际执行 icacls 影响测试环境）。
// 生活类比：在 Linux 上找不到 Windows 专用的"保险柜锁匠"(icacls)，自然应当报错。
// 注：restrictKeyFileACLWindows 已迁移至 internal/pathutil/pathutil.go 的 RestrictACLWindows（见安全审查 #20）
func TestRestrictKeyFileACLWindows_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 平台跳过 icacls 测试，避免影响测试环境")
	}
	// 非 Windows 平台 icacls 命令不存在，pathutil.RestrictACLWindows 应返回错误
	err := pathutil.RestrictACLWindows("/tmp/nonexistent_douya_key_test")
	if err == nil {
		t.Fatal("非 Windows 平台 icacls 不存在，期望返回错误，实际返回 nil")
	}
}

// TestLoadOrCreateKey_NonWindowsNoICAcls 验证非 Windows 平台 LoadOrCreateKey
// 不依赖 icacls，保持原 0600 权限行为。
// Windows 平台跳过。
func TestLoadOrCreateKey_NonWindowsNoICAcls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 平台跳过此非 Windows 行为验证测试")
	}
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, ".enc_key")

	key, err := LoadOrCreateKey(keyPath)
	if err != nil {
		t.Fatalf("LoadOrCreateKey 失败: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("期望 32 字节密钥，实际 %d 字节", len(key))
	}

	// 非 Windows 平台应保持 0600 权限，不调用 icacls
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("非 Windows 平台期望 0600 权限，实际 %v", info.Mode().Perm())
	}

	// 再次调用应返回相同密钥（文件已存在且长度为 32）
	key2, err := LoadOrCreateKey(keyPath)
	if err != nil {
		t.Fatalf("第二次 LoadOrCreateKey 失败: %v", err)
	}
	if string(key) != string(key2) {
		t.Fatal("两次读取的密钥不一致")
	}
}

// TestLoadOrCreateKey_DamagedFile 验证损坏的密钥文件返回错误而非静默覆盖
func TestLoadOrCreateKey_DamagedFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, ".enc_key")

	// 写入一个长度异常的"损坏"密钥文件
	if err := os.WriteFile(keyPath, []byte("damaged"), 0600); err != nil {
		t.Fatalf("写入损坏文件失败: %v", err)
	}

	_, err := LoadOrCreateKey(keyPath)
	if err == nil {
		t.Fatal("损坏的密钥文件应返回错误，而非静默覆盖")
	}
}


