//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// FindMigrationsPath locates a migrations directory by traversing up from the current directory.
// It looks for the pattern: components/ledger/migrations/{component}
//
// Example:
//
//	path := FindMigrationsPath(t, "onboarding")  // finds components/ledger/migrations/onboarding
//	path := FindMigrationsPath(t, "transaction") // finds components/ledger/migrations/transaction
func FindMigrationsPath(t *testing.T, component string) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err, "failed to get current working directory")

	for {
		candidate := filepath.Join(dir, "components", "ledger", "migrations", component)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find migrations directory for component %q", component)
		}

		dir = parent
	}
}
