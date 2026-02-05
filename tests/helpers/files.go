// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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
