package store

import (
	"context"
	"database/sql"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

func GetSetting(db *sql.DB, key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	var value string
	err := db.QueryRowContext(ctx,
		"SELECT value FROM settings WHERE key = ?",
		key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "get setting", err)
	}
	return value, nil
}

func SetSetting(db *sql.DB, key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	_, err := db.ExecContext(ctx,
		"INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)",
		key, value,
	)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "set setting", err)
	}
	return nil
}

// GetEncryptedSetting 读取加密存储的设置值并解密
// 安全实践（基于 B-1.13/B-1.14）：调用 crypto.go 的 decryptWithPrefix 统一解密逻辑
//
// 行为说明：
//   - 如果值不是加密格式（无 "enc:" 前缀），则作为明文返回（兼容旧数据迁移）
//   - 如果值有 "enc:" 前缀但解密失败，返回 ErrDecryptionFailed（密钥不匹配或数据损坏）
//   - 调用方应检查 error，密钥不匹配时不应把密文当作有效值使用
func GetEncryptedSetting(db *sql.DB, key string, encKey []byte) (string, error) {
	value, err := GetSetting(db, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", nil
	}

	plaintext, err := decryptWithPrefix(value, encKey)
	if err != nil {
		log.Warn().Str("key", key).Err(err).Msg("[store] GetEncryptedSetting decryption failed, possible key mismatch")
		return "", ErrDecryptionFailed
	}
	return plaintext, nil
}

// SetEncryptedSetting 加密后存储设置值
// 安全实践（基于 B-1.13/B-1.14）：调用 crypto.go 的 encryptWithPrefix 统一加密逻辑
func SetEncryptedSetting(db *sql.DB, key, value string, encKey []byte) error {
	if value == "" {
		return SetSetting(db, key, "")
	}

	encrypted, err := encryptWithPrefix(value, encKey)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "encrypt setting", err)
	}

	return SetSetting(db, key, encrypted)
}
