// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package cache_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil_dbsuite"
)

// TestMain sets up the integration test environment using testcontainers.
// Starts a PostgreSQL container and applies all migrations so the rules table
// and idx_rules_updated_at index are available, then exports the DB_* env vars
// that testutil.GetTestDSN reads.
func TestMain(m *testing.M) {
	// Locate migrations/ relative to this file (3 levels up: cache/ -> services/ -> internal/ -> project root)
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations")

	os.Exit(testutil_dbsuite.SetupTestDBSuite(m,
		testutil_dbsuite.WithMigrations(migrationsPath),
	))
}
