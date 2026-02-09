package search

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/monochromegane/go-gitignore"
)

// Walk traverses the file tree rooted at root and returns a list of files.
// It respects .gitignore if found in the root directory.
func Walk(root string) ([]string, error) {
	var paths []string
	var ignoreMatcher gitignore.IgnoreMatcher

	// Check for .gitignore in root
	gitignorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		ignoreMatcher, _ = gitignore.NewGitIgnore(gitignorePath)
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors (permission denied, etc.) to keep partial results
		}

		// Calculate relative path for matching
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if relPath == "." {
			return nil
		}

		// Default ignores
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir // Skip hidden directories
			}
			if d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
		}

		// Gitignore check
		if ignoreMatcher != nil {
			if ignoreMatcher.Match(path, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Add files only (unless we want dirs too? Spec implies navigating to files, but also "Enter on Dir drills down")
		// The list should probably contain both?
		// "The search engine must not just find the target; it must understand hierarchy."
		// "Enter (on File): Selects... Enter (on Dir): Drills down"
		// implementation_plan says "Returns a slice of all file paths".
		// Let's include Directories too if they are not skipped, so user can navigate? 
		// Actually, standard fuzzy finders usually flatten files. navigation happens by "drilling down" which starts a NEW search at that dir.
		// So this search should return FILES + DIRS?
		// Spec: "Input: arg1 arg2 ... arg1 is ancestor of arg2".
		// "Selects file... Enter (on Dir)..." matches implies Dirs are in the list.
		// Let's add everything.
		
		paths = append(paths, relPath)
		return nil
	})

	return paths, err
}
