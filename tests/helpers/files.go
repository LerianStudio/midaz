// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains file handling utilities for test data management.
package helpers

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteTextFile creates a directory if it doesn't exist and writes the given
// content to a file, overwriting it if it already exists.
func WriteTextFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	return os.WriteFile(path, []byte(content), 0o644)
}
