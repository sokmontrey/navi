package store

import (
	"os"
	"testing"
)

func TestStore(t *testing.T) {
	// Use a temp file for testing
	tmpFile, err := os.CreateTemp("", "navi-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Test 1: Tags
	t.Run("Tags", func(t *testing.T) {
		tagName := "@project"
		path1 := "/home/user/project1"
		path2 := "/home/user/project2"

		if err := AddPathToTag(db, tagName, path1); err != nil {
			t.Fatalf("AddPathToTag failed: %v", err)
		}
		if err := AddPathToTag(db, tagName, path2); err != nil {
			t.Fatalf("AddPathToTag failed: %v", err)
		}

		paths, err := GetPathsForTag(db, tagName)
		if err != nil {
			t.Fatalf("GetPathsForTag failed: %v", err)
		}

		if len(paths) != 2 {
			t.Errorf("expected 2 paths, got %d", len(paths))
		}
		if paths[0] != path1 && paths[0] != path2 { // Order depends on strings, they are sorted in query
			t.Errorf("unexpected path in results: %v", paths)
		}

		// Remove
		if err := RemovePathFromTag(db, tagName, path1); err != nil {
			t.Fatalf("RemovePathFromTag failed: %v", err)
		}
		paths, err = GetPathsForTag(db, tagName)
		if err != nil {
			t.Fatalf("GetPathsForTag failed post-remove: %v", err)
		}
		if len(paths) != 1 {
			t.Errorf("expected 1 path after remove, got %d", len(paths))
		}
		if paths[0] != path2 {
			t.Errorf("expected %s, got %s", path2, paths[0])
		}
	})

	// Test 2: History
	t.Run("History", func(t *testing.T) {
		path := "/home/user/frecency/test"
		
		// First visit
		if err := UpdateFrecency(db, path); err != nil {
			t.Fatalf("UpdateFrecency 1 failed: %v", err)
		}

		history, err := GetHistory(db)
		if err != nil {
			t.Fatalf("GetHistory failed: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("expected 1 history item, got %d", len(history))
		}
		if history[0].Frequency != 1 {
			t.Errorf("expected frequency 1, got %d", history[0].Frequency)
		}

		// Second visit
		if err := UpdateFrecency(db, path); err != nil {
			t.Fatalf("UpdateFrecency 2 failed: %v", err)
		}

		history, err = GetHistory(db)
		if err != nil {
			t.Fatalf("GetHistory 2 failed: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("expected 1 history item, got %d", len(history))
		}
		if history[0].Frequency != 2 {
			t.Errorf("expected frequency 2, got %d", history[0].Frequency)
		}
	})
}
