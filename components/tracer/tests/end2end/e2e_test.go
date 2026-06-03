// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package end2end

import (
	"os"
	"testing"

	"github.com/cucumber/godog"

	"github.com/LerianStudio/midaz/v3/components/tracer/tests/end2end/steps"
	"github.com/LerianStudio/midaz/v3/components/tracer/tests/end2end/support"
)

func TestFeatures(t *testing.T) {
	if support.GetBaseURL() == "" {
		t.Fatal("SERVER_ADDRESS not set — cannot run E2E tests (use: make test-e2e)")
	}

	suite := godog.TestSuite{
		ScenarioInitializer: steps.InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Tags:     os.Getenv("GODOG_TAGS"),
			TestingT: t,
		},
	}

	if status := suite.Run(); status != 0 {
		t.Fatalf("non-zero status returned (exit code %d), failing test suite", status)
	}
}
