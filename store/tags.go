package store

import (
	"database/sql"
	"fmt"
)

// AddPathToTag adds a path to a specific tag.
func AddPathToTag(db *sql.DB, tagName, path string) error {
	query := `INSERT OR IGNORE INTO tags (name, path) VALUES (?, ?)`
	_, err := db.Exec(query, tagName, path)
	if err != nil {
		return fmt.Errorf("failed to add path to tag: %w", err)
	}
	return nil
}

// GetPathsForTag returns all paths associated with a tag.
func GetPathsForTag(db *sql.DB, tagName string) ([]string, error) {
	query := `SELECT path FROM tags WHERE name = ? ORDER BY path`
	rows, err := db.Query(query, tagName)
	if err != nil {
		return nil, fmt.Errorf("failed to get paths for tag: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// GetAllTaggedPaths returns all unique paths that have at least one tag.
func GetAllTaggedPaths(db *sql.DB) ([]string, error) {
	query := `SELECT DISTINCT path FROM tags`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all tagged paths: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// RemovePathFromTag removes a path from a specific tag.
func RemovePathFromTag(db *sql.DB, tagName, path string) error {
	query := `DELETE FROM tags WHERE name = ? AND path = ?`
	_, err := db.Exec(query, tagName, path)
	if err != nil {
		return fmt.Errorf("failed to remove path from tag: %w", err)
	}
	return nil
}
