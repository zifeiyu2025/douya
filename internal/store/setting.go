package store

import (
	"context"
	"database/sql"
	"fmt"

	"douya/internal/secrets"
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

	// 解密
	plaintext, err := secrets.Decrypt(value[4:], encKey)
	if err != nil {
		// 解密失败，可能是旧明文数据恰好以 "enc:" 开头，返回原值
		return value, nil
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
