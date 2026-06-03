// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package steps

import (
	"fmt"
	"net/http"

	"github.com/cucumber/godog"

	"github.com/LerianStudio/midaz/v3/components/tracer/tests/end2end/support"
)

func assertAuthReachable() error {
	_, status, err := support.ListValidationsE("limit=1")
	if err != nil {
		return fmt.Errorf("authentication check failed: %w", err)
	}

	if status != http.StatusOK {
		return fmt.Errorf("authentication check returned status %d, expected %d", status, http.StatusOK)
	}

	return nil
}

func registerAuthSteps(ctx *godog.ScenarioContext, sc *support.ScenarioContext) {
	// Authentication — these are no-ops because auth is handled at the HTTP client level.
	// They exist so Gherkin Background steps don't produce "undefined step" errors.
	ctx.Step(`^(\w+) is authenticated in the Tracer system$`, func(persona string) error {
		return assertAuthReachable()
	})

	ctx.Step(`^the system is authenticated$`, func() error {
		return assertAuthReachable()
	})

	// Background setup steps — no-ops because test data (accounts, segments)
	// is created by earlier scenarios via the API. The E2E environment is
	// bootstrapped by docker-compose (see mk/tests.mk, target test-e2e).
	ctx.Step(`^the corporate segment has been registered in the system$`, func() error {
		return nil // Created by prior scenarios via POST /segments
	})

	ctx.Step(`^a test account and segment exist$`, func() error {
		return nil // Created by prior scenarios via API calls
	})

	ctx.Step(`^the account for customer "([^"]*)" is registered in the system$`, func(customer string) error {
		return nil // Created by prior scenarios via API calls
	})

	ctx.Step(`^merchants "([^"]*)" and "([^"]*)" have been identified as trusted with zero fraud history$`, func(m1, m2 string) error {
		// Generate deterministic UUIDs for merchant resolution only if not already set.
		for _, name := range []string{m1, m2} {
			if _, exists := sc.MerchantUUIDs[name]; !exists {
				sc.MerchantUUIDs[name] = support.DeterministicMerchantUUID(name)
			}
		}

		return nil
	})
}
