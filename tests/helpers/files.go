package helpers

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	defaultDirPermissions  = 0o755
	defaultFilePermissions = 0o644
)

// WriteTextFile ensures the directory exists and writes content to path, overwriting any existing file.
func WriteTextFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), defaultDirPermissions); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), defaultFilePermissions); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
