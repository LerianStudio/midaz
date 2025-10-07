// Package helpers provides test utilities and helper functions for integration tests.
// This file contains file handling utilities for test data management.
package helpers

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteTextFile ensures the directory exists and writes content to path, overwriting any existing file.
func WriteTextFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
