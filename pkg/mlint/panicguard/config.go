package panicguard

import (
	"path/filepath"
	"strings"
)

// PathMatcher determines if a file path should be excluded from analysis.
type PathMatcher struct {
	// Patterns are glob patterns to match against file paths.
	// Supports: *, **, specific directory names
	patterns []string
}

// NewPathMatcher creates a PathMatcher with the given patterns.
func NewPathMatcher(patterns []string) *PathMatcher {
	return &PathMatcher{patterns: patterns}
}

// ShouldExclude returns true if the given file path matches any exclusion pattern.
func (pm *PathMatcher) ShouldExclude(filePath string) bool {
	// Normalize path separators
	normalizedPath := filepath.ToSlash(filePath)

	for _, pattern := range pm.patterns {
		if pm.matchPattern(normalizedPath, pattern) {
			return true
		}
	}

	return false
}

// matchPattern checks if the path matches the given pattern.
func (pm *PathMatcher) matchPattern(path, pattern string) bool {
	// Handle suffix patterns like *_test.go
	if strings.HasPrefix(pattern, "*") && !strings.Contains(pattern, "/") {
		suffix := pattern[1:] // Remove leading *
		return strings.HasSuffix(path, suffix)
	}

	// Handle prefix patterns like mock_*
	if strings.HasSuffix(pattern, "*") && !strings.Contains(pattern, "/") {
		prefix := pattern[:len(pattern)-1]
		base := filepath.Base(path)

		return strings.HasPrefix(base, prefix)
	}

	// Handle directory patterns like /pkg/mruntime/
	if strings.HasPrefix(pattern, "/") {
		// Remove leading slash for contains check
		dirPattern := pattern[1:]
		if strings.HasSuffix(dirPattern, "/") {
			// Directory pattern - check if path contains this directory
			return strings.Contains(path, dirPattern) ||
				strings.Contains(path, strings.TrimSuffix(dirPattern, "/"))
		}

		return strings.Contains(path, dirPattern)
	}

	// Handle glob patterns with **
	if strings.Contains(pattern, "**") {
		// Convert ** glob to simple contains check
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			suffix := strings.TrimPrefix(parts[1], "/")
			if suffix != "" {
				return strings.HasSuffix(path, suffix) ||
					strings.Contains(path, suffix)
			}
		}
	}

	// Handle directory name patterns like mocks/
	if strings.HasSuffix(pattern, "/") {
		dirName := strings.TrimSuffix(pattern, "/")

		return strings.Contains(path, "/"+dirName+"/") ||
			strings.HasSuffix(path, "/"+dirName)
	}

	// Direct substring match as fallback
	return strings.Contains(path, pattern)
}

// Default exclusion patterns for different rule types.
var (
	// CommonExclusions are patterns excluded from all rules.
	CommonExclusions = []string{
		"*_test.go",
		"*.pb.go",
		"*_mock.go",
		"mock_*.go",
		"mocks/",
	}

	// MRuntimeExclusions excludes the mruntime package itself.
	MRuntimeExclusions = []string{
		"/pkg/mruntime/",
	}

	// BoundaryPackageExclusions are packages allowed to use recover().
	BoundaryPackageExclusions = []string{
		"/pkg/mruntime/",
		"/internal/adapters/http/",
		"/internal/adapters/grpc/",
		"/internal/adapters/rabbitmq/",
		"/internal/bootstrap/",
	}

	// PanicAllowedExclusions are packages where panic() is acceptable.
	PanicAllowedExclusions = []string{
		"/pkg/mruntime/",
		"/pkg/assert/",
	}
)
