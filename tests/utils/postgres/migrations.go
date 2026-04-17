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
// It looks for the pattern: components/{component}/migrations
//
// Accepts testing.TB so benchmarks can call it too — the signature was widened
// from *testing.T during Batch B to support BenchmarkTransactionsContention_HotBalance.
//
// Example:
//
//	path := FindMigrationsPath(t, "onboarding")  // finds components/onboarding/migrations
//	path := FindMigrationsPath(t, "transaction") // finds components/transaction/migrations
func FindMigrationsPath(tb testing.TB, component string) string {
	tb.Helper()

	dir, err := os.Getwd()
	require.NoError(tb, err, "failed to get current working directory")

	for {
		candidate := filepath.Join(dir, "components", component, "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatalf("could not find migrations directory for component %q", component)
		}

		dir = parent
	}
}
