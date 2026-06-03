// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"os"
	"testing"

	testutil_integration "github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil_integration"
)

// TestMain sets up the integration test environment using testcontainers.
// This runs once before all tests in the package.
func TestMain(m *testing.M) {
	// Check if testcontainers should be disabled
	if os.Getenv("DISABLE_TESTCONTAINERS") == "true" {
		// Use external server (docker-compose)
		os.Exit(m.Run())
	}

	// Default: use testcontainers
	os.Exit(testutil_integration.SetupTestSuite(m))
}
