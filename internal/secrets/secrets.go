// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	zlog "github.com/rs/zerolog/log"

	"douya/internal/apperror"
	"douya/internal/pathutil"
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
		return nil, apperror.Wrap(apperror.KindInternal, "create cipher", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "create GCM", err)
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
		return nil, apperror.Newf(apperror.KindInvalidInput, "密钥文件 %s 已损坏（当前大小=%d 字节，期望=32 字节），请备份后手动删除该文件再启动", keyPath, len(data))
	}
	// 文件不存在，创建新密钥
	if !os.IsNotExist(err) {
		// 其他读取错误（权限等）
		return nil, apperror.Wrapf(apperror.KindInternal, "读取密钥文件 %s 失败", err, keyPath)
	}

	// 生成新的 256-bit 密钥
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "生成密钥失败", err)
	}

	// 确保目录存在
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "create key dir", err)
	}

	// 写入密钥文件，权限仅限当前用户
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return nil, apperror.Wrapf(apperror.KindInternal, "写入密钥文件 %s 失败", err, keyPath)
	}

	// Windows 平台：0600 权限位在 Windows 上意义有限（NTFS 不使用 Unix 权限模型），
	// 因此额外调用 icacls 收紧 ACL，移除继承权限并仅授予当前用户读写。
	// 失败仅记录告警，不阻止写入——密钥文件已写入成功，ACL 收紧失败不应导致启动中断。
	// 非 Windows 平台：保持原 0600 行为不变。
	// 安全实践：复用 pathutil.RestrictACLWindows，统一文件权限收紧逻辑（见安全审查 #20）
	if runtime.GOOS == "windows" {
		if err := pathutil.RestrictACLWindows(keyPath); err != nil {
			zlog.Warn().Err(err).Str("key_path", keyPath).Msg("[secrets] restrict key file ACL failed on Windows")
		}
	}

	return key, nil
}

// restrictKeyFileACLWindows 已迁移至 internal/pathutil/pathutil.go 的 RestrictACLWindows，
// 统一文件权限收紧逻辑，供 secrets/config/logger/store 复用（见安全审查 #20）。

// Encrypt 使用 AES-GCM 加密明文，返回 base64 编码的密文
// 改用 CipherCache 缓存 AEAD 实例，避免每次调用都重新创建 cipher
func Encrypt(plaintext string, key []byte) (string, error) {
	aesGCM, err := defaultCipherCache.getAEAD(key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "generate nonce", err)
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
		return "", apperror.Wrap(apperror.KindInternal, "decode base64", err)
	}

	aesGCM, err := defaultCipherCache.getAEAD(key)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", apperror.New(apperror.KindInvalidInput, "ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "decrypt", err)
	}

	return string(plaintext), nil
}
