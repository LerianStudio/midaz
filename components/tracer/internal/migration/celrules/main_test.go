// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package celrules

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_dbsuite"
)

// TestMain starts a PostgreSQL testcontainer and applies all tracer migrations
// so the rules table (and its enums) exist for the stored-rule migration tests.
func TestMain(m *testing.M) {
	// celrules/ -> migration/ -> internal/ -> components/tracer (three levels up).
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations")

	os.Exit(testutil_dbsuite.SetupTestDBSuite(
		m,
		testutil_dbsuite.WithMigrations(migrationsPath),
	))
}
