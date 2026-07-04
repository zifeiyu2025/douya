package store

import (
	"context"
	"database/sql"
	"fmt"

	"douya/internal/secrets"

	"github.com/rs/zerolog/log"
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
		return "", fmt.Errorf("get setting: %w", err)
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
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}

// GetEncryptedSetting 读取加密存储的设置值并解密
// 如果值不是加密格式（无 "enc:" 前缀），则作为明文返回（兼容旧数据迁移）
// 如果值有 "enc:" 前缀但解密失败，返回 ErrDecryptionFailed（密钥不匹配或数据损坏）
// 调用方应检查 error，密钥不匹配时不应把密文当作有效值使用
func GetEncryptedSetting(db *sql.DB, key string, encKey []byte) (string, error) {
	value, err := GetSetting(db, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", nil
	}

	// 兼容旧版明文数据：没有 "enc:" 前缀的直接返回
	if len(value) < 4 || value[:4] != "enc:" {
		return value, nil
	}

	// 解密：失败时返回 error，让调用方明确感知密钥不匹配
	plaintext, err := secrets.Decrypt(value[4:], encKey)
	if err != nil {
		log.Warn().Str("key", key).Err(err).Msg("[store] GetEncryptedSetting decryption failed, possible key mismatch")
		return "", ErrDecryptionFailed
	}
	return plaintext, nil
}

// SetEncryptedSetting 加密后存储设置值
func SetEncryptedSetting(db *sql.DB, key, value string, encKey []byte) error {
	if value == "" {
		return SetSetting(db, key, "")
	}

	encrypted, err := secrets.Encrypt(value, encKey)
	if err != nil {
		return fmt.Errorf("encrypt setting: %w", err)
	}

	return SetSetting(db, key, "enc:"+encrypted)
}
