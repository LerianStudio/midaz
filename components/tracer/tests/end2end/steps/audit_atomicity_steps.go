// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package steps

import (
	"fmt"

	"github.com/cucumber/godog"

	"github.com/LerianStudio/midaz/v3/components/tracer/tests/end2end/support"
)

// registerAuditAtomicitySteps registers step definitions that exercise the
// SOX/GLBA audit atomicity contract: when the audit-event insert inside a
// lifecycle transaction fails, the repository mutation must be rolled back
// and no audit event must land. The steps drive the negative-path scenarios
// via a fault-injection trigger on audit_events.
func registerAuditAtomicitySteps(ctx *godog.ScenarioContext, sc *support.ScenarioContext) {
	// Install the fault-injection trigger on audit_events for the last
	// rule or limit in context. Sets ExpectActivationFailure so the
	// subsequent `activates the rule/limit` step tolerates a 5xx response.
	ctx.Step(`^the audit event insert is forced to fail for that (rule|limit) on activation$`,
		func(resourceType string) error {
			db, err := sc.DB()
			if err != nil {
				return fmt.Errorf("opening scenario db: %w", err)
			}

			var (
				resourceID string
				eventType  string
			)

			switch resourceType {
			case "rule":
				if sc.LastRule.ID == "" {
					return fmt.Errorf("no rule in context to target with fault injection")
				}

				resourceID = sc.LastRule.ID
				eventType = "RULE_ACTIVATED"
			case "limit":
				if sc.LastLimit.ID == "" {
					return fmt.Errorf("no limit in context to target with fault injection")
				}

				resourceID = sc.LastLimit.ID
				eventType = "LIMIT_ACTIVATED"
			default:
				return fmt.Errorf("unsupported resource type %q", resourceType)
			}

			cleanup, err := support.InstallFailOnAuditEvent(db, resourceID, eventType)
			if err != nil {
				return fmt.Errorf("installing fault trigger: %w", err)
			}

			sc.AddFaultCleanup(cleanup)
			sc.ExpectActivationFailure = true

			return nil
		})

	ctx.Step(`^the activation request returns an HTTP 5xx error$`, func() error {
		if sc.LastActivationHTTP < 500 || sc.LastActivationHTTP >= 600 {
			return fmt.Errorf("expected 5xx status, got %d (body: %s)",
				sc.LastActivationHTTP, string(sc.LastActivationBody))
		}

		return nil
	})

	// Atomicity assertion: the rule row in the DB must still be DRAFT
	// because the activation transaction rolled back. Uses a direct DB
	// query so the assertion is independent of the HTTP error the caller
	// is trying to validate.
	ctx.Step(`^the rule remains in Draft status$`, func() error {
		if sc.LastRule.ID == "" {
			return fmt.Errorf("no rule in context")
		}

		db, err := sc.DB()
		if err != nil {
			return fmt.Errorf("opening scenario db: %w", err)
		}

		status, err := support.FetchRuleStatusDirect(db, sc.LastRule.ID)
		if err != nil {
			return fmt.Errorf("reading rule status: %w", err)
		}

		if status != "DRAFT" {
			return fmt.Errorf("rule %s: expected DRAFT after rollback, got %q",
				sc.LastRule.ID, status)
		}

		return nil
	})

	// Mirror of the rule-side assertion for limits: the DB row must be
	// unchanged (still DRAFT) because the activation transaction rolled
	// back when the audit insert failed.
	ctx.Step(`^the limit remains unchanged$`, func() error {
		if sc.LastLimit.ID == "" {
			return fmt.Errorf("no limit in context")
		}

		db, err := sc.DB()
		if err != nil {
			return fmt.Errorf("opening scenario db: %w", err)
		}

		status, err := support.FetchLimitStatusDirect(db, sc.LastLimit.ID)
		if err != nil {
			return fmt.Errorf("reading limit status: %w", err)
		}

		if status != "DRAFT" {
			return fmt.Errorf("limit %s: expected DRAFT after rollback, got %q",
				sc.LastLimit.ID, status)
		}

		return nil
	})

	ctx.Step(`^no audit event is recorded for that (rule|limit) activation$`, func(resourceType string) error {
		db, err := sc.DB()
		if err != nil {
			return fmt.Errorf("opening scenario db: %w", err)
		}

		var (
			resourceID string
			eventType  string
		)

		switch resourceType {
		case "rule":
			if sc.LastRule.ID == "" {
				return fmt.Errorf("no rule in context")
			}

			resourceID = sc.LastRule.ID
			eventType = "RULE_ACTIVATED"
		case "limit":
			if sc.LastLimit.ID == "" {
				return fmt.Errorf("no limit in context")
			}

			resourceID = sc.LastLimit.ID
			eventType = "LIMIT_ACTIVATED"
		default:
			return fmt.Errorf("unsupported resource type %q", resourceType)
		}

		count, err := support.CountAuditEventsDirect(db, resourceID, eventType)
		if err != nil {
			return fmt.Errorf("counting audit events: %w", err)
		}

		if count != 0 {
			return fmt.Errorf("expected 0 %s events for %s, got %d",
				eventType, resourceID, count)
		}

		return nil
	})
}
