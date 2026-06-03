// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"tracer/internal/testutil_dbsuite"
)

// TestMain sets up the integration test environment using testcontainers.
// Starts a PostgreSQL container and applies all migrations (functions + schema)
// so that tables like limits and usage_counters are available for repository tests.
func TestMain(m *testing.M) {
	// Locate migrations/ relative to this file (3 levels up: postgres/ -> adapters/ -> internal/ -> project root)
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations")

	os.Exit(testutil_dbsuite.SetupTestDBSuite(m,
		testutil_dbsuite.WithMigrations(migrationsPath),
	))
}
