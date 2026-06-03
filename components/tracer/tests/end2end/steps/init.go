// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package steps

import (
	"context"

	"github.com/cucumber/godog"

	"github.com/LerianStudio/midaz/v3/components/tracer/tests/end2end/support"
)

// InitializeScenario registers all step definitions for a Godog scenario.
// Called once per scenario — each scenario gets a fresh ScenarioContext.
func InitializeScenario(ctx *godog.ScenarioContext) {
	// Reset package-level UUID maps so each scenario starts from a known state.
	// This prevents cross-scenario pollution from dynamically-added merchant,
	// segment or account names that would shift counter-based UUID assignment.
	support.ResetDeterministicUUIDMaps()

	sc := support.NewScenarioContext()

	// NOTE: Per-scenario rule/limit cleanup is intentionally disabled —
	// E2E scenarios within a Feature are sequential journeys. However,
	// ephemeral scenario-owned resources (fault-injection triggers, the
	// per-scenario DB handle) MUST be torn down so they do not leak into
	// subsequent scenarios. That is what ScenarioTearDown handles below.
	ctx.After(func(goCtx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		sc.ScenarioTearDown()

		return goCtx, nil
	})

	// Register step definitions by category
	registerAuthSteps(ctx, sc)
	registerRuleSteps(ctx, sc)
	registerValidationSteps(ctx, sc)
	registerLimitSteps(ctx, sc)
	registerAuditSteps(ctx, sc)
	registerAuditAtomicitySteps(ctx, sc)
}
