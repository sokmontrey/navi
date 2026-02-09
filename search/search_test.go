package search

import (
	"testing"
)

func TestGapSearch(t *testing.T) {
	paths := []string{
		"src/foo/bar.go",
		"src/baz/bar.go",
		"src/foo/other.go",
		"README.md",
	}

	tests := []struct {
		name     string
		query    string
		expected []string // Order matters if we implement ranking, but for now just existence
	}{
		{
			name:     "Single term match",
			query:    "foo",
			expected: []string{"src/foo/bar.go", "src/foo/other.go"},
		},
		{
			name:     "Gap match (foo ... bar)",
			query:    "foo bar",
			expected: []string{"src/foo/bar.go"},
		},
		{
			name:     "Gap match (baz ... bar)",
			query:    "baz bar",
			expected: []string{"src/baz/bar.go"},
		},
		{
			name:     "Reverse order (bar ... foo) - Should NOT match",
			query:    "bar foo",
			expected: []string{},
		},
		{
			name:     "Partial characters (s/f/b)",
			query:    "s/f/b",
			expected: []string{"src/foo/bar.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := FuzzyHierarchical(paths, tt.query)
			
			// Extract paths from results
			var resultPaths []string
			for _, res := range results {
				resultPaths = append(resultPaths, res.Path)
			}

			// Check expected count
			if len(resultPaths) != len(tt.expected) {
				t.Errorf("expected %d results, got %d for query '%s'", len(tt.expected), len(resultPaths), tt.query)
				t.Logf("Got: %v", resultPaths)
				return
			}

			// Check containment (order independent for this test)
			for _, exp := range tt.expected {
				found := false
				for _, res := range resultPaths {
					if res == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected result to contain %s, but it was missing. Got: %v", exp, resultPaths)
				}
			}
		})
	}
}
