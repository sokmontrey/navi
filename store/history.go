package store

import (
	"database/sql"
	"fmt"
	"time"
)

type HistoryItem struct {
	Path        string
	Frequency   int
	LastVisited time.Time
}

// UpdateFrecency updates the frequency and last_visited timestamp for a path.
// It inserts the path if it doesn't exist.
func UpdateFrecency(db *sql.DB, path string) error {
	// Upsert logic: SQLite generic support (ON CONFLICT)
	query := `
		INSERT INTO history (path, frequency, last_visited) 
		VALUES (?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			frequency = frequency + 1,
			last_visited = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, path)
	if err != nil {
		return fmt.Errorf("failed to update frecency: %w", err)
	}
	return nil
}

// GetHistory returns the history items, usually for ranking or debugging.
func GetHistory(db *sql.DB) ([]HistoryItem, error) {
	query := `SELECT path, frequency, last_visited FROM history ORDER BY last_visited DESC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}
	defer rows.Close()

	var items []HistoryItem
	for rows.Next() {
		var item HistoryItem
		
		if err := rows.Scan(&item.Path, &item.Frequency, &item.LastVisited); err != nil {
			// Fallback if direct scan fails (often due to format)
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// GetRecentHistory returns recent history items, limited to the most recent N items.
// This is used for initial load to show recent session history.
func GetRecentHistory(db *sql.DB, limit int) ([]HistoryItem, error) {
	query := `SELECT path, frequency, last_visited FROM history ORDER BY last_visited DESC LIMIT ?`
	rows, err := db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}
	defer rows.Close()

	var items []HistoryItem
	for rows.Next() {
		var item HistoryItem
		
		if err := rows.Scan(&item.Path, &item.Frequency, &item.LastVisited); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
