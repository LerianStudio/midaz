// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Package migrations exposes the production migration set to integration
// tests without making test binaries depend on a source checkout at runtime.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed *.sql
var files embed.FS

// WriteTo materializes the production migrations into destinationRoot for
// runners that require a file:// source.
func WriteTo(destinationRoot string) error {
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		return fmt.Errorf("create embedded migrations directory: %w", err)
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		body, err := fs.ReadFile(files, entry.Name())
		if err != nil {
			return fmt.Errorf("read embedded migration %s: %w", entry.Name(), err)
		}

		if err := os.WriteFile(filepath.Join(destinationRoot, entry.Name()), body, 0o600); err != nil {
			return fmt.Errorf("write embedded migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}
