package search

import (
	"strings"

	"github.com/sahilm/fuzzy"
)

type Result struct {
	Path    string
	Score   int
	Matches []int // Indices of matched characters
}

// FuzzyHierarchical performs a fuzzy search on the provided paths.
// It respects the order of query parts (ancestor matching).
func FuzzyHierarchical(paths []string, query string) []Result {
	if query == "" {
		// Return all or empty? Returning all for "navigator" style
		results := make([]Result, len(paths))
		for i, p := range paths {
			results[i] = Result{Path: p}
		}
		return results
	}

	// Use sahilm/fuzzy
	// Remove spaces to support "gap" matching (e.g. "foo bar" -> "foobar")
	// This preserves order ("foo" must appear before "bar") but allows matching
	// across directory separators without requiring the space character in the path.
	cleanQuery := strings.ReplaceAll(query, " ", "")
	
	matches := fuzzy.Find(cleanQuery, paths)

	var results []Result
	for _, match := range matches {
		results = append(results, Result{
			Path:    match.Str,
			Score:   match.Score,
			Matches: match.MatchedIndexes,
		})
	}

	// Ranking happens implicitly by fuzzy.Find sorting? 
	// sahlim/fuzzy returns matches sorted by score.
	
	// We might want to add Frecency boosting here later (Phase 2 integration).
	// For now, simple return.
	
	return results
}

// Helper to manually partial sort if we add custom scoring later
type ByScore []Result

func (a ByScore) Len() int           { return len(a) }
func (a ByScore) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByScore) Less(i, j int) bool { return a[i].Score > a[j].Score } // Higher score first? fuzzy pkg uses different metric. 
// fuzzy score: 0 is best? No, it's usually match length/distance. 
// sahilm/fuzzy: "Matches are sorted by score (descending?)" -> Check docs if needed. 
// actually fuzzy.Find returns Matches which is already sorted.
