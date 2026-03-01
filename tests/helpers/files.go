// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package helpers

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirPermissions  = 0o755 // rwxr-xr-x
	filePermissions = 0o644 // rw-r--r--
)

// WriteTextFile ensures the directory exists and writes content to path, overwriting any existing file.
func WriteTextFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPermissions); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), filePermissions); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
