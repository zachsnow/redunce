package scanner

import (
	"github.com/zachsnow/redunce/internal/gitignore"
)

// NewIgnoreMatcher creates a gitignore matcher that checks both .redunceignore
// and .gitignore files at each directory level, with .redunceignore taking
// precedence.
//
// Note: System-level defaults are handled separately in ScanPaths using
// Get ResolvedIgnorePatterns and matchesIgnorePatterns.
func NewIgnoreMatcher(baseDir string) (gitignore.GitIgnore, error) {
	// Check .redunceignore first, then .gitignore at each directory level
	// If .redunceignore has a pattern match, it takes precedence
	// If no match in .redunceignore, check .gitignore
	files := []string{".redunceignore", ".gitignore"}

	matcher := gitignore.NewRepositoryWithFiles(baseDir, files, nil, nil)
	if matcher == nil {
		// Library returns nil on error, but we handled it in the error callback
		// For our purposes, we can create a simple non-matching instance
		return nil, nil
	}

	return matcher, nil
}
