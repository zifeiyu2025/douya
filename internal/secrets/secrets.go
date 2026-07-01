// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"

	zlog "github.com/rs/zerolog/log"
)

// CipherCache 按 key 哈希缓存 cipher.AEAD 实例，避免每次加密/解密都重新创建。
// 生活类比：像工具箱里预先放好的"加密锁"，同一把钥匙（key）只用造一次锁，
// 之后每次加密/解密直接拿来用，省去重复造锁的开销。
type CipherCache struct {
	m sync.Map // key: hex(key) → cipher.AEAD
}

// defaultCipherCache 是包级默认缓存实例，供 Encrypt/Decrypt/EncryptBatch 共用。
var defaultCipherCache CipherCache

// getAEAD 从缓存获取 AEAD 实例，未命中则创建并缓存。
// 同一 key 只会创建一次 AEAD，后续命中直接返回。
func (c *CipherCache) getAEAD(key []byte) (cipher.AEAD, error) {
	// 用 key 的十六进制表示作为缓存键，避免 key 内容变更后命中旧缓存
	cacheKey := hex.EncodeToString(key)
	if v, ok := c.m.Load(cacheKey); ok {
		return v.(cipher.AEAD), nil
	}
	// 缓存未命中，创建新的 AEAD
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	c.m.Store(cacheKey, aesGCM)
	return aesGCM, nil
}

// Cipher 接口定义加密/解密能力，供 chat.Service 等模块依赖。
// 生活类比：就像一个"加密锁"的标准接口——只要能加密和解密就行，
// 不管你内部用的是 AES 还是别的算法。这样 chat.Service 就不用关心具体实现，
// 也方便测试时替换成假的加密器（mock）。
//
// 说明：本接口只定义 Encrypt/Decrypt 两个方法。需要获取底层 []byte 密钥
// 调用旧 API（如 store 包）的场景，请使用 CipherKey 辅助函数从具体实现中提取。
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// AESCipher 是基于 AES-GCM 的 Cipher 接口实现，持有加密密钥。
// 它额外提供 Key() 方法以暴露底层密钥，便于调用 store 包等仍需 []byte 的旧 API。
type AESCipher struct {
	key []byte
}

// Encrypt 使用持有的密钥加密明文，返回 base64 编码的密文
func (c *AESCipher) Encrypt(plaintext string) (string, error) {
	return Encrypt(plaintext, c.key)
}

// Decrypt 解密 base64 编码的 AES-GCM 密文
func (c *AESCipher) Decrypt(ciphertext string) (string, error) {
	return Decrypt(ciphertext, c.key)
}

// Key 返回底层密钥，供需要 []byte 的旧 API（如 store 包）使用。
// 该方法不归属 Cipher 接口，仅属于具体实现，避免在接口层暴露密钥。
func (c *AESCipher) Key() []byte {
	return c.key
}

// NewCipher 基于已有的 AES 密钥构造一个 Cipher 实例。
// 调用方负责先通过 LoadOrCreateKey 获取密钥，再用本函数包装成 Cipher。
func NewCipher(key []byte) Cipher {
	return &AESCipher{key: key}
}

// CipherKey 从 Cipher 实例中提取底层 []byte 密钥。
// 若该 Cipher 实现未暴露密钥（即不提供 Key() []byte 方法），返回 nil。
// 生活类比：相当于问"加密锁"——"把你用的钥匙给我看一下"，方便传给
// 那些只认钥匙（[]byte）不认锁（Cipher 接口）的旧工具（store 包）。
func CipherKey(c Cipher) []byte {
	type keyHolder interface {
		Key() []byte
	}
	if k, ok := c.(keyHolder); ok {
		return k.Key()
	}
	return nil
}

// LoadOrCreateKey 加载或创建加密密钥文件
// 密钥文件存储在 keyPath 路径，受 OS 文件权限保护
//
// 行为说明：
//   - 文件不存在：生成新密钥并写入，返回（正常首次启动场景）
//   - 文件存在且长度为 32：直接返回已有密钥
//   - 文件存在但长度异常：返回错误，绝不静默覆盖——否则用旧密钥加密的数据将永久无法解密
func LoadOrCreateKey(keyPath string) ([]byte, error) {
	data, err := os.ReadFile(keyPath)
	if err == nil {
		// 文件存在
		if len(data) == 32 {
			return data, nil
		}
		// 文件存在但长度异常——可能是损坏/截断，不能静默覆盖，否则旧加密数据永久无法解密
		return nil, fmt.Errorf("密钥文件 %s 已损坏（当前大小=%d 字节，期望=32 字节），请备份后手动删除该文件再启动", keyPath, len(data))
	}
	// 文件不存在，创建新密钥
	if !os.IsNotExist(err) {
		// 其他读取错误（权限等）
		return nil, fmt.Errorf("读取密钥文件 %s 失败: %w", keyPath, err)
	}

	// 生成新的 256-bit 密钥
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("生成密钥失败: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}

	// 写入密钥文件，权限仅限当前用户
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("写入密钥文件 %s 失败: %w", keyPath, err)
	}

	// Windows 平台：0600 权限位在 Windows 上意义有限（NTFS 不使用 Unix 权限模型），
	// 因此额外调用 icacls 收紧 ACL，移除继承权限并仅授予当前用户读写。
	// 失败仅记录告警，不阻止写入——密钥文件已写入成功，ACL 收紧失败不应导致启动中断。
	// 非 Windows 平台：保持原 0600 行为不变。
	if runtime.GOOS == "windows" {
		if err := restrictKeyFileACLWindows(keyPath); err != nil {
			zlog.Warn().Err(err).Str("key_path", keyPath).Msg("[secrets] restrict key file ACL failed on Windows")
		}
	}

	return key, nil
}

// restrictKeyFileACLWindows 在 Windows 上通过 icacls 命令限制密钥文件的访问权限，
// 移除继承的权限并仅授予当前用户读写权限。
// 生活类比：就像给保险柜换一把只有你能开的锁，并把其他人以前配的钥匙全部作废——
// 即使有人能物理访问到这个文件，没有你的用户身份也读不了、改不了。
//
// 命令格式：icacls "<keyPath>" /inheritance:r /grant:r "<username>:(R,W)"
//   - /inheritance:r  移除从父目录继承的所有权限
//   - /grant:r         替换式授予（覆盖而非追加）指定用户的权限
//   - (R,W)            仅授予读(Read)和写(Write)权限
func restrictKeyFileACLWindows(keyPath string) error {
	// 获取当前用户名，用于在 ACL 中授予该用户读写权限
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("获取当前用户失败: %w", err)
	}
	username := u.Username

	// 拼接并执行 icacls 命令
	// 注意：exec.Command 会自动处理参数转义，无需手动加引号
	cmd := exec.Command("icacls", keyPath, "/inheritance:r", "/grant:r", fmt.Sprintf("%s:(R,W)", username))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls 限制密钥文件权限失败: %w, 命令输出: %s", err, string(output))
	}
	return nil
}

// Encrypt 使用 AES-GCM 加密明文，返回 base64 编码的密文
// 改用 CipherCache 缓存 AEAD 实例，避免每次调用都重新创建 cipher
func Encrypt(plaintext string, key []byte) (string, error) {
	aesGCM, err := defaultCipherCache.getAEAD(key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// nonce 附加在密文前面
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 base64 编码的 AES-GCM 密文
// 改用 CipherCache 缓存 AEAD 实例，避免每次调用都重新创建 cipher
func Decrypt(encoded string, key []byte) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	aesGCM, err := defaultCipherCache.getAEAD(key)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// EncryptBatch 批量加密多个明文，复用同一 AEAD 实例，避免重复创建 cipher 的开销。
// 适用于一次性加密多个字段（如消息的多个加密字段）的场景。
func EncryptBatch(plaintexts []string, key []byte) ([]string, error) {
	aesGCM, err := defaultCipherCache.getAEAD(key)
	if err != nil {
		return nil, err
	}
	results := make([]string, len(plaintexts))
	for i, pt := range plaintexts {
		nonce := make([]byte, aesGCM.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return nil, fmt.Errorf("generate nonce: %w", err)
		}
		// nonce 附加在密文前面
		ciphertext := aesGCM.Seal(nonce, nonce, []byte(pt), nil)
		results[i] = base64.StdEncoding.EncodeToString(ciphertext)
	}
	return results, nil
}
