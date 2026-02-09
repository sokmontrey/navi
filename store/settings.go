package store

import (
	"database/sql"
	"fmt"
)

// GetSetting retrieves a setting value by key.
func GetSetting(db *sql.DB, key string) (string, error) {
	query := `SELECT value FROM settings WHERE key = ?`
	var value string
	err := db.QueryRow(query, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get setting %q: %w", key, err)
	}
	return value, nil
}

// SetSetting sets a setting value by key.
func SetSetting(db *sql.DB, key, value string) error {
	query := `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`
	_, err := db.Exec(query, key, value)
	if err != nil {
		return fmt.Errorf("failed to set setting %q: %w", key, err)
	}
	return nil
}
