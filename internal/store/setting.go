package store

import (
	"database/sql"
	"fmt"
)

func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(
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
	_, err := db.Exec(
		"INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}
