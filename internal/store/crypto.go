// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"fmt"

	"douya/internal/apperror"
	"douya/internal/secrets"
)

// encPrefix 是加密字段的统一前缀，用于区分密文与旧版明文数据
const encPrefix = "enc:"

// ErrDecryptionFailed 解密失败错误
// 用于区分"密钥不匹配"与"明文兼容"两种场景，调用方据此决定如何处理
// 安全实践（基于 B-1.13/B-1.14）：统一 store 包的加密/解密错误，消除重复定义
var ErrDecryptionFailed = fmt.Errorf("decryption failed: key mismatch or corrupted ciphertext")

// encryptWithPrefix 使用 AES-GCM 加密字符串，返回带 "enc:" 前缀的密文
// 安全实践（基于 B-1.13/B-1.14）：统一 setting.go 和 message.go 的加密逻辑
//
// 行为约定：
//   - encKey 为 nil 时跳过加密直接返回明文（用于禁用加密的场景）
//   - plaintext 为空时直接返回空字符串（不产生无意义的密文）
//   - 加密失败时返回 error，调用方应决定如何处理而非静默回退为明文
func encryptWithPrefix(plaintext string, encKey []byte) (string, error) {
	if encKey == nil || plaintext == "" {
		return plaintext, nil
	}
	encrypted, err := secrets.Encrypt(plaintext, encKey)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "encrypt failed", err)
	}
	return encPrefix + encrypted, nil
}

// decryptWithPrefix 解密带 "enc:" 前缀的密文，兼容旧版明文数据
// 安全实践（基于 B-1.13/B-1.14）：统一 setting.go 和 message.go 的解密逻辑
//
// 行为约定：
//   - encKey 为 nil 时跳过解密直接返回原值（用于禁用加密的场景）
//   - ciphertext 为空时直接返回空字符串
//   - 没有 "enc:" 前缀的值视为旧版明文数据，直接返回（兼容旧数据迁移）
//   - 有 "enc:" 前缀但解密失败时返回 ErrDecryptionFailed（密钥不匹配或数据损坏）
//     调用方应检查 error，密钥不匹配时不应把密文当作有效值使用
func decryptWithPrefix(ciphertext string, encKey []byte) (string, error) {
	if encKey == nil || ciphertext == "" {
		return ciphertext, nil
	}
	// 兼容旧版明文数据：没有 "enc:" 前缀的直接返回
	if len(ciphertext) < len(encPrefix) || ciphertext[:len(encPrefix)] != encPrefix {
		return ciphertext, nil
	}
	plaintext, err := secrets.Decrypt(ciphertext[len(encPrefix):], encKey)
	if err != nil {
		// 解密失败：密钥不匹配或密文损坏
		// 用 apperror 包装保留类型信息，ErrDecryptionFailed 作为 Cause 保留在错误链中
		// 上层既可用 errors.Is(err, ErrDecryptionFailed) 判断，也可用 apperror.KindOf(err) 获取类型
		return "", apperror.Wrap(apperror.KindInternal, "decrypt failed", ErrDecryptionFailed)
	}
	return plaintext, nil
}
