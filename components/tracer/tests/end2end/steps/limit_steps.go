// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package steps

import (
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/cucumber/godog"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/tests/end2end/support"
)

func registerLimitSteps(ctx *godog.ScenarioContext, sc *support.ScenarioContext) {
	// --- Builder pattern steps (J2, J6 persona style) ---

	ctx.Step(`^(\w+) creates a daily limit called "([^"]*)"$`, func(persona, name string) error {
		sc.InitPendingLimit(name, "DAILY")
		return nil
	})

	// Compact one-line daily-limit creator with a subType scope. Used by
	// the subType case-insensitive scenario. Preserves the raw input (with
	// whitespace / casing) so the server-side normalization is exercised
	// end-to-end. Example:
	// `Maria creates a daily limit of 1000 USD called "Crypto Sweep" with sub-type "  Buy  "`
	ctx.Step(`^(\w+) creates a daily limit of ([0-9,.]+) (\w+) called "([^"]*)" with sub-type "([^"]*)"$`,
		func(persona, amount, currency, name, subType string) error {
			amt, err := decimal.NewFromString(normalizeAmount(amount))
			if err != nil {
				return fmt.Errorf("parsing amount %q: %w", amount, err)
			}

			sc.InitPendingLimit(name, "DAILY")
			sc.PendingLimit.MaxAmount = amt
			sc.PendingLimit.Currency = strings.ToUpper(currency)

			accountID := support.TestAccountUUID()
			sc.PendingLimit.Scopes = []testutil.ScopeInput{
				{AccountID: testutil.Ptr(accountID), SubType: testutil.Ptr(subType)},
			}

			return nil
		})

	// Compact limit creator with a merchant scope. Used by the merchant
	// scope enforcement scenario. The merchant name is mapped to a
	// deterministic UUID so subsequent validation steps (`the merchant
	// has ID "X"`) target the same merchantId. Example:
	// `Maria creates a daily limit of 1000 USD called "Acme Daily Cap" for merchant "acme-corp"`
	ctx.Step(`^(\w+) creates a daily limit of ([0-9,.]+) (\w+) called "([^"]*)" for merchant "([^"]*)"$`,
		func(persona, amount, currency, name, merchant string) error {
			amt, err := decimal.NewFromString(normalizeAmount(amount))
			if err != nil {
				return fmt.Errorf("parsing amount %q: %w", amount, err)
			}

			merchantID := sc.MerchantUUIDs[merchant]
			if merchantID == "" {
				merchantID = support.DeterministicMerchantUUID(merchant)
				sc.MerchantUUIDs[merchant] = merchantID
			}

			sc.InitPendingLimit(name, "DAILY")
			sc.PendingLimit.MaxAmount = amt
			sc.PendingLimit.Currency = strings.ToUpper(currency)
			sc.PendingLimit.Scopes = []testutil.ScopeInput{
				{MerchantID: testutil.Ptr(merchantID)},
			}

			return nil
		})

	// Direct GET + sub-type canonical-form assertion (mirrors the rule
	// assertion). Reads the limit via the API and compares scope[0].subType
	// to the expected canonical (trimmed+lowercased) value.
	ctx.Step(`^the stored limit "([^"]*)" should have sub-type "([^"]*)"$`,
		func(name, expected string) error {
			limitID := sc.FindLimitID(name)
			if limitID == "" {
				return fmt.Errorf("limit %q not found in scenario context", name)
			}

			limit, status, err := support.GetLimitE(limitID)
			if err != nil {
				return fmt.Errorf("getting limit: %w", err)
			}

			if status != http.StatusOK {
				return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
			}

			if len(limit.Scopes) == 0 {
				return fmt.Errorf("limit %q has no scopes", name)
			}

			got := ""
			if limit.Scopes[0].SubType != nil {
				got = *limit.Scopes[0].SubType
			}

			if got != expected {
				return fmt.Errorf("limit %q: expected stored sub-type %q, got %q",
					name, expected, got)
			}

			sc.LastLimit = limit

			return nil
		})

	// Compact one-line daily-limit creator. Example:
	// `Maria creates a daily spending limit of 500 USD called "Daily Cap"`
	// Leaves Scopes empty so the caller (or the scenario Background) can
	// add scope attributes via subsequent `And ...` steps; otherwise the
	// limit is created unscoped and the `the limit should be created
	// successfully in Draft status` assertion posts the request.
	ctx.Step(`^(\w+) creates a daily spending limit of ([0-9,.]+) (\w+) called "([^"]*)"$`,
		func(persona, amount, currency, name string) error {
			amt, err := decimal.NewFromString(normalizeAmount(amount))
			if err != nil {
				return fmt.Errorf("parsing amount %q: %w", amount, err)
			}

			sc.InitPendingLimit(name, "DAILY")
			sc.PendingLimit.MaxAmount = amt
			sc.PendingLimit.Currency = strings.ToUpper(currency)
			// Default to a deterministic account scope so the API accepts
			// the limit (at least one scope attribute required).
			accountID := support.TestAccountUUID()
			sc.PendingLimit.Scopes = []testutil.ScopeInput{
				{AccountID: testutil.Ptr(accountID)},
			}

			return nil
		})

	ctx.Step(`^(?:he|she) sets the maximum amount to R\$([0-9,.]+) in (\w+)$`, func(amount, currency string) error {
		if sc.PendingLimit == nil {
			return fmt.Errorf("no pending limit — call 'creates a daily limit' first")
		}

		amt, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount %q: %w", amount, err)
		}

		sc.PendingLimit.MaxAmount = amt
		sc.PendingLimit.Currency = strings.ToUpper(currency)

		return nil
	})

	ctx.Step(`^(?:he|she) applies it to payment transactions in the corporate segment$`, func() error {
		if sc.PendingLimit == nil {
			return fmt.Errorf("no pending limit")
		}

		segmentID := support.DeterministicSegmentUUID("corporate")
		sc.PendingLimit.Scopes = []testutil.ScopeInput{
			{SegmentID: testutil.Ptr(segmentID), TransactionType: testutil.Ptr("CARD")},
		}

		return nil
	})

	ctx.Step(`^(?:he|she) applies it to the (.+?) account$`, func(customer string) error {
		if sc.PendingLimit == nil {
			return fmt.Errorf("no pending limit")
		}

		customer = strings.TrimSpace(customer)
		accountID := support.DeterministicAccountUUID(customer)
		sc.PendingLimit.Scopes = []testutil.ScopeInput{
			{AccountID: testutil.Ptr(accountID)},
		}

		return nil
	})

	// Flush pending limit → POST /v1/limits
	ctx.Step(`^the limit should be created successfully in Draft status$`, func() error {
		if sc.PendingLimit != nil && sc.PendingLimit.Name != "" {
			req := &support.LimitRequest{
				Name:      sc.PendingLimit.Name,
				LimitType: sc.PendingLimit.LimitType,
				MaxAmount: sc.PendingLimit.MaxAmount,
				Currency:  sc.PendingLimit.Currency,
				Scopes:    sc.PendingLimit.Scopes,
			}

			limit, status, err := support.CreateLimitE(req)
			if err != nil {
				return fmt.Errorf("creating limit: %w", err)
			}

			sc.LastLimit = limit
			sc.LastLimitHTTP = status
			sc.PendingLimit = nil

			if status != http.StatusCreated {
				return fmt.Errorf("expected 201 Created, got %d", status)
			}

			sc.RegisterLimit(limit.Name, limit.ID)
		}

		if sc.LastLimit.ID == "" {
			return fmt.Errorf("no limit available: neither created from pending nor previously set")
		}

		if sc.LastLimit.Status != "DRAFT" {
			return fmt.Errorf("expected DRAFT status, got %q", sc.LastLimit.Status)
		}

		return nil
	})

	// --- Limit status management ---

	ctx.Step(`^the limit "([^"]*)" is in Draft status$`, func(name string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found in scenario context", name)
		}

		limit, status, err := support.GetLimitE(limitID)
		if err != nil {
			return fmt.Errorf("getting limit: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		if limit.Status != "DRAFT" {
			return fmt.Errorf("expected DRAFT, got %q", limit.Status)
		}

		sc.LastLimit = limit

		return nil
	})

	ctx.Step(`^the limit "([^"]*)" is Active(?: with a maximum of R\$([0-9,.]+))?$`, func(name, maxAmount string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found in scenario context", name)
		}

		limit, status, err := support.GetLimitE(limitID)
		if err != nil {
			return fmt.Errorf("getting limit: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		if limit.Status != "ACTIVE" {
			return fmt.Errorf("expected ACTIVE, got %q", limit.Status)
		}

		if maxAmount != "" {
			expected, err := decimal.NewFromString(normalizeAmount(maxAmount))
			if err != nil {
				return fmt.Errorf("parsing expected amount: %w", err)
			}

			if !limit.MaxAmount.Equal(expected) {
				return fmt.Errorf("expected maxAmount %s, got %s", expected, limit.MaxAmount)
			}
		}

		sc.LastLimit = limit

		return nil
	})

	ctx.Step(`^(\w+) activates the limit$`, func(persona string) error {
		if sc.LastLimit.ID == "" {
			return fmt.Errorf("no limit in context to activate")
		}

		// Atomicity scenarios pre-install a fault trigger and set
		// ExpectActivationFailure. In that mode we must NOT fail the step
		// on a 5xx — the scenario's Then assertions verify the rollback.
		if sc.ExpectActivationFailure {
			status, body, err := support.ActivateLimitRawE(sc.LastLimit.ID)
			if err != nil {
				return fmt.Errorf("activating limit: %w", err)
			}

			sc.LastActivationHTTP = status
			sc.LastActivationBody = body
			sc.LastLimitHTTP = status

			return nil
		}

		limit, status, err := support.ActivateLimitE(sc.LastLimit.ID)
		if err != nil {
			return fmt.Errorf("activating limit: %w", err)
		}

		sc.LastLimit = limit
		sc.LastLimitHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^(\w+) activates the limit "([^"]*)"$`, func(persona, name string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found", name)
		}

		limit, status, err := support.ActivateLimitE(limitID)
		if err != nil {
			return fmt.Errorf("activating limit: %w", err)
		}

		sc.LastLimit = limit
		sc.LastLimitHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^(\w+) deactivates the limit "([^"]*)"$`, func(persona, name string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found", name)
		}

		limit, status, err := support.DeactivateLimitE(limitID)
		if err != nil {
			return fmt.Errorf("deactivating limit: %w", err)
		}

		sc.LastLimit = limit
		sc.LastLimitHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d deactivating limit %q, got %d", http.StatusOK, name, status)
		}

		return nil
	})

	ctx.Step(`^the limit should become Active$`, func() error {
		if sc.LastLimit.Status != "ACTIVE" {
			return fmt.Errorf("expected ACTIVE, got %q", sc.LastLimit.Status)
		}

		return nil
	})

	ctx.Step(`^the limit should be Active with a maximum of R\$([0-9,.]+)$`, func(amount string) error {
		if sc.LastLimit.Status != "ACTIVE" {
			return fmt.Errorf("expected ACTIVE, got %q", sc.LastLimit.Status)
		}

		expected, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing expected amount: %w", err)
		}

		if !sc.LastLimit.MaxAmount.Equal(expected) {
			return fmt.Errorf("expected maxAmount %s, got %s", expected, sc.LastLimit.MaxAmount)
		}

		return nil
	})

	// --- Builder pattern steps (J3 style, max 2 params per step) ---

	ctx.Step(`^a daily limit called "([^"]*)"$`, func(name string) error {
		sc.InitPendingLimit(name, "DAILY")
		return nil
	})

	ctx.Step(`^the maximum amount is R\$([0-9,.]+) in (\w+)$`, func(amount, currency string) error {
		if sc.PendingLimit == nil {
			return fmt.Errorf("no pending limit")
		}

		amt, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		sc.PendingLimit.MaxAmount = amt
		sc.PendingLimit.Currency = strings.ToUpper(currency)

		return nil
	})

	ctx.Step(`^it applies to the test account on (\w+) transfers$`, func(txType string) error {
		if sc.PendingLimit == nil {
			return fmt.Errorf("no pending limit")
		}

		accountID := support.TestAccountUUID()
		sc.PendingLimit.Scopes = []testutil.ScopeInput{
			{AccountID: testutil.Ptr(accountID), TransactionType: testutil.Ptr(strings.ToUpper(txType))},
		}

		return nil
	})

	ctx.Step(`^the limit is created$`, func() error {
		if sc.PendingLimit == nil {
			return fmt.Errorf("no pending limit to create")
		}

		req := &support.LimitRequest{
			Name:      sc.PendingLimit.Name,
			LimitType: sc.PendingLimit.LimitType,
			MaxAmount: sc.PendingLimit.MaxAmount,
			Currency:  sc.PendingLimit.Currency,
			Scopes:    sc.PendingLimit.Scopes,
		}

		limit, status, err := support.CreateLimitE(req)
		if err != nil {
			return fmt.Errorf("creating limit: %w", err)
		}

		sc.LastLimit = limit
		sc.LastLimitHTTP = status
		sc.PendingLimit = nil

		if status != http.StatusCreated {
			return fmt.Errorf("expected 201, got %d", status)
		}

		sc.RegisterLimit(limit.Name, limit.ID)

		return nil
	})

	ctx.Step(`^the limit is activated$`, func() error {
		if sc.LastLimit.ID == "" {
			return fmt.Errorf("no limit in context to activate")
		}

		limit, status, err := support.ActivateLimitE(sc.LastLimit.ID)
		if err != nil {
			return fmt.Errorf("activating limit: %w", err)
		}

		sc.LastLimit = limit
		sc.LastLimitHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^the limit should be Active$`, func() error {
		if sc.LastLimit.Status != "ACTIVE" {
			return fmt.Errorf("expected ACTIVE, got %q", sc.LastLimit.Status)
		}

		return nil
	})

	// --- Usage check steps ---

	ctx.Step(`^(\w+) checks the usage of "([^"]*)"$`, func(persona, name string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found in scenario context", name)
		}

		usage, status, err := support.GetLimitUsageE(limitID)
		if err != nil {
			return fmt.Errorf("getting usage: %w", err)
		}

		sc.LastUsage = usage
		sc.LastUsageHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^the usage of "([^"]*)" is checked$`, func(name string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found", name)
		}

		usage, status, err := support.GetLimitUsageE(limitID)
		if err != nil {
			return fmt.Errorf("getting usage: %w", err)
		}

		sc.LastUsage = usage
		sc.LastUsageHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^the usage of "([^"]*)" is R\$([0-9,.]+)$`, func(name, amount string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found", name)
		}

		usage, status, err := support.GetLimitUsageE(limitID)
		if err != nil {
			return fmt.Errorf("getting usage: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		expected, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing expected amount: %w", err)
		}

		if !usage.CurrentUsage.Equal(expected) {
			return fmt.Errorf("expected usage R$%s, got R$%s", expected, usage.CurrentUsage)
		}

		sc.LastUsage = usage

		return nil
	})

	ctx.Step(`^the current usage should be R\$([0-9,.]+)$`, func(amount string) error {
		expected, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		if !sc.LastUsage.CurrentUsage.Equal(expected) {
			return fmt.Errorf("expected usage R$%s, got R$%s", expected, sc.LastUsage.CurrentUsage)
		}

		return nil
	})

	ctx.Step(`^the current usage should be zero$`, func() error {
		if !sc.LastUsage.CurrentUsage.IsZero() {
			return fmt.Errorf("expected zero usage, got R$%s", sc.LastUsage.CurrentUsage)
		}

		return nil
	})

	ctx.Step(`^the maximum amount should be R\$([0-9,.]+)$`, func(amount string) error {
		expected, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		if !sc.LastUsage.LimitAmount.Equal(expected) {
			return fmt.Errorf("expected limit amount R$%s, got R$%s", expected, sc.LastUsage.LimitAmount)
		}

		return nil
	})

	ctx.Step(`^the utilization should be (\d+)%$`, func(percent int) error {
		expected := float64(percent)
		actual := sc.LastUsage.UtilizationPercent

		// Allow small floating-point tolerance
		diff := math.Abs(actual - expected)

		if diff > 0.5 {
			return fmt.Errorf("expected utilization %.0f%%, got %.1f%%", expected, actual)
		}

		return nil
	})

	ctx.Step(`^the limit should be flagged as near its threshold$`, func() error {
		if !sc.LastUsage.NearLimit {
			return fmt.Errorf("expected nearLimit=true, got false (utilization: %.1f%%)", sc.LastUsage.UtilizationPercent)
		}

		return nil
	})

	ctx.Step(`^the limit should not be flagged as near its threshold$`, func() error {
		if sc.LastUsage.NearLimit {
			return fmt.Errorf("expected nearLimit=false, got true (utilization: %.1f%%)", sc.LastUsage.UtilizationPercent)
		}

		return nil
	})

	// --- Limit update steps (J6) ---

	ctx.Step(`^(?:he|she) updates the maximum amount to R\$([0-9,.]+)$`, func(amount string) error {
		if sc.LastLimit.ID == "" {
			return fmt.Errorf("no limit in context to update")
		}

		newMax, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		limit, status, err := support.UpdateLimitE(sc.LastLimit.ID, &support.UpdateLimitRequest{
			MaxAmount: &newMax,
		})
		if err != nil {
			return fmt.Errorf("updating limit: %w", err)
		}

		sc.LastLimit = limit
		sc.LastLimitHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^(?:he|she) reactivates the limit$`, func() error {
		if sc.LastLimit.ID == "" {
			return fmt.Errorf("no limit in context to reactivate")
		}

		limit, status, err := support.ActivateLimitE(sc.LastLimit.ID)
		if err != nil {
			return fmt.Errorf("reactivating limit %s: %w", sc.LastLimit.ID, err)
		}

		sc.LastLimit = limit
		sc.LastLimitHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("reactivating limit %s: expected %d, got %d", sc.LastLimit.ID, http.StatusOK, status)
		}

		return nil
	})
}
