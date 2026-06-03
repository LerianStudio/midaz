// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package migration

import (
	"os"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil_dbsuite"
)

// TestMain sets up the integration test environment using testcontainers.
// Starts a PostgreSQL container with NO pre-applied migrations — the
// TestMigratorIntegration test creates its own temp migrations to verify
// the FunctionMigrator works from a blank database.
func TestMain(m *testing.M) {
	os.Exit(testutil_dbsuite.SetupTestDBSuite(m))
}
