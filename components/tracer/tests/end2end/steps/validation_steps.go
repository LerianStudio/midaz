// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package steps

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cucumber/godog"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/tests/end2end/support"
)

func registerValidationSteps(ctx *godog.ScenarioContext, sc *support.ScenarioContext) {
	// --- Builder pattern steps (max 2 params per step) ---

	ctx.Step(`^a (\w+) transaction of R\$([0-9,.]+)$`, func(txType, amount string) error {
		amt, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount %q: %w", amount, err)
		}

		sc.InitPendingTransaction(normalizeTransactionType(txType), amt)

		return nil
	})

	ctx.Step(`^the account is (\d+) days old$`, func(days int) error {
		if sc.PendingTransaction == nil {
			return fmt.Errorf("no pending transaction")
		}

		sc.PendingTransaction.Metadata["accountAgeDays"] = days

		return nil
	})

	ctx.Step(`^the customer tier is "([^"]*)"$`, func(tier string) error {
		if sc.PendingTransaction == nil {
			return fmt.Errorf("no pending transaction")
		}

		sc.PendingTransaction.Metadata["customerTier"] = tier

		return nil
	})

	// SubType on a pending transaction (T-2 contract). Builder-pattern step:
	// Example: `And the sub-type is "Sell"`.
	ctx.Step(`^the sub-type is "([^"]*)"$`, func(subType string) error {
		if sc.PendingTransaction == nil {
			return fmt.Errorf("no pending transaction")
		}

		sc.PendingTransaction.SubType = subType

		return nil
	})

	// Merchant scope (T-J8) builder step: `And the merchant has ID "acme-corp"`
	// The alphanumeric name is mapped to a deterministic UUID so subsequent
	// limit-matching assertions share the same merchantId.
	ctx.Step(`^the merchant has ID "([^"]*)"$`, func(name string) error {
		if sc.PendingTransaction == nil {
			return fmt.Errorf("no pending transaction")
		}

		merchantID := sc.MerchantUUIDs[name]
		if merchantID == "" {
			merchantID = support.DeterministicMerchantUUID(name)
			sc.MerchantUUIDs[name] = merchantID
		}

		sc.PendingTransaction.Merchant = &testutil.MerchantContext{
			ID:   merchantID,
			Name: name,
		}

		return nil
	})

	// --- Limit match assertions (T-J8 / T-2) ---

	ctx.Step(`^the limit "([^"]*)" should have been evaluated$`, func(name string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found in scenario context", name)
		}

		for _, detail := range sc.LastValidation.LimitUsageDetails {
			if detail.LimitID == limitID {
				return nil
			}
		}

		return fmt.Errorf("limit %q (%s) was not present in limitUsageDetails (got %d entries)",
			name, limitID, len(sc.LastValidation.LimitUsageDetails))
	})

	ctx.Step(`^the limit "([^"]*)" should NOT have been evaluated$`, func(name string) error {
		limitID := sc.FindLimitID(name)
		if limitID == "" {
			return fmt.Errorf("limit %q not found in scenario context", name)
		}

		for _, detail := range sc.LastValidation.LimitUsageDetails {
			if detail.LimitID == limitID {
				return fmt.Errorf("limit %q (%s) was unexpectedly present in limitUsageDetails",
					name, limitID)
			}
		}

		return nil
	})

	// --- Rule match assertions by name ---

	ctx.Step(`^the rule "([^"]*)" should have matched$`, func(name string) error {
		ruleID := sc.FindRuleID(name)
		if ruleID == "" {
			return fmt.Errorf("rule %q not found in scenario context", name)
		}

		// A matched rule must also have been evaluated. Asserting both
		// defends against a future regression where the scope filter is
		// widened to permissive matching and silently produces a "match"
		// without running the CEL expression.
		evaluated := false

		for _, id := range sc.LastValidation.EvaluatedRuleIDs {
			if id == ruleID {
				evaluated = true
				break
			}
		}

		if !evaluated {
			return fmt.Errorf("rule %q (%s) not present in evaluatedRuleIds (got %v)",
				name, ruleID, sc.LastValidation.EvaluatedRuleIDs)
		}

		for _, matchedID := range sc.LastValidation.MatchedRuleIDs {
			if matchedID == ruleID {
				return nil
			}
		}

		return fmt.Errorf("rule %q (%s) not present in matchedRuleIds (got %v)",
			name, ruleID, sc.LastValidation.MatchedRuleIDs)
	})

	ctx.Step(`^the rule "([^"]*)" should NOT have matched$`, func(name string) error {
		ruleID := sc.FindRuleID(name)
		if ruleID == "" {
			return fmt.Errorf("rule %q not found in scenario context", name)
		}

		// Intentionally NOT checking EvaluatedRuleIDs here: "should NOT
		// have matched" is satisfied both by "evaluated but CEL returned
		// false" and by "scope filter rejected — never evaluated". Both
		// outcomes are valid for scope-mismatch scenarios, and tightening
		// to require one or the other would force scenarios to know which
		// path the engine took.
		for _, matchedID := range sc.LastValidation.MatchedRuleIDs {
			if matchedID == ruleID {
				return fmt.Errorf("rule %q (%s) unexpectedly matched", name, ruleID)
			}
		}

		return nil
	})

	ctx.Step(`^the merchant is "([^"]*)" in (.+)$`, func(name, country string) error {
		if sc.PendingTransaction == nil {
			return fmt.Errorf("no pending transaction")
		}

		countryCode := mapCountryToISO(country)
		merchantID := sc.MerchantUUIDs[name]
		if merchantID == "" {
			merchantID = support.DeterministicMerchantUUID(name)
		}

		sc.PendingTransaction.Merchant = &testutil.MerchantContext{
			ID:      merchantID,
			Name:    name,
			Country: countryCode,
		}

		return nil
	})

	ctx.Step(`^the merchant is "([^"]*)"$`, func(name string) error {
		if sc.PendingTransaction == nil {
			return fmt.Errorf("no pending transaction")
		}

		merchantID := sc.MerchantUUIDs[name]
		if merchantID == "" {
			merchantID = support.DeterministicMerchantUUID(name)
		}

		sc.PendingTransaction.Merchant = &testutil.MerchantContext{
			ID:   merchantID,
			Name: name,
		}

		return nil
	})

	ctx.Step(`^the transaction is submitted$`, func() error {
		req := sc.BuildValidationRequest()
		if req == nil {
			return fmt.Errorf("no pending transaction to submit")
		}

		valResp, status, err := support.CreateValidationE(req)
		if err != nil {
			return fmt.Errorf("submitting validation: %w", err)
		}

		sc.LastValidation = valResp
		sc.LastValidationHTTP = status
		sc.ValidationHistory = append(sc.ValidationHistory, valResp)
		sc.PendingTransaction = nil

		if status != http.StatusCreated {
			return fmt.Errorf("expected %d, got %d", http.StatusCreated, status)
		}

		return nil
	})

	// --- Direct submit steps (≤3 params) ---

	ctx.Step(`^a (\w+) transaction of R\$([0-9,.]+) is submitted$`, func(txType, amount string) error {
		amt, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		req := &testutil.ValidationRequest{
			RequestID:       testutil.MustDeterministicUUID(support.NextRequestID()).String(),
			TransactionType: normalizeTransactionType(txType),
			Amount:          amt,
			Currency:        "BRL",
		}

		valResp, status, err := support.CreateValidationE(req)
		if err != nil {
			return fmt.Errorf("submitting validation: %w", err)
		}

		sc.LastValidation = valResp
		sc.LastValidationHTTP = status
		sc.ValidationHistory = append(sc.ValidationHistory, valResp)

		if status != http.StatusCreated {
			return fmt.Errorf("expected %d, got %d", http.StatusCreated, status)
		}

		return nil
	})

	ctx.Step(`^a (\w+) of R\$([0-9,.]+) is submitted for the (\w+) segment$`, func(txType, amount, segment string) error {
		amt, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		segmentID := support.DeterministicSegmentUUID(segment)

		req := &testutil.ValidationRequest{
			RequestID:       testutil.MustDeterministicUUID(support.NextRequestID()).String(),
			TransactionType: normalizeTransactionType(txType),
			Amount:          amt,
			Currency:        "BRL",
			Segment:         &testutil.SegmentContext{ID: segmentID},
		}

		valResp, status, err := support.CreateValidationE(req)
		if err != nil {
			return fmt.Errorf("submitting validation: %w", err)
		}

		sc.LastValidation = valResp
		sc.LastValidationHTTP = status
		sc.ValidationHistory = append(sc.ValidationHistory, valResp)

		if status != http.StatusCreated {
			return fmt.Errorf("expected %d, got %d", http.StatusCreated, status)
		}

		return nil
	})

	ctx.Step(`^a (\w+) (?:transfer )?of R\$([0-9,.]+) is submitted from the test account$`, func(txType, amount string) error {
		amt, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		accountID := support.TestAccountUUID()

		req := &testutil.ValidationRequest{
			RequestID:       testutil.MustDeterministicUUID(support.NextRequestID()).String(),
			TransactionType: normalizeTransactionType(txType),
			Amount:          amt,
			Currency:        "BRL",
			Account:         &testutil.AccountContext{ID: accountID},
		}

		valResp, status, err := support.CreateValidationE(req)
		if err != nil {
			return fmt.Errorf("submitting validation: %w", err)
		}

		sc.LastValidation = valResp
		sc.LastValidationHTTP = status
		sc.ValidationHistory = append(sc.ValidationHistory, valResp)

		if status != http.StatusCreated {
			return fmt.Errorf("expected %d, got %d", http.StatusCreated, status)
		}

		return nil
	})

	ctx.Step(`^a (\w+) (?:transfer )?of R\$([0-9,.]+) is submitted from ([^']+)'s account$`, func(txType, amount, customer string) error {
		amt, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		accountID := support.DeterministicAccountUUID(customer)

		req := &testutil.ValidationRequest{
			RequestID:       testutil.MustDeterministicUUID(support.NextRequestID()).String(),
			TransactionType: normalizeTransactionType(txType),
			Amount:          amt,
			Currency:        "BRL",
			Account:         &testutil.AccountContext{ID: accountID},
		}

		valResp, status, err := support.CreateValidationE(req)
		if err != nil {
			return fmt.Errorf("submitting validation: %w", err)
		}

		sc.LastValidation = valResp
		sc.LastValidationHTTP = status
		sc.ValidationHistory = append(sc.ValidationHistory, valResp)

		if status != http.StatusCreated {
			return fmt.Errorf("expected %d, got %d", http.StatusCreated, status)
		}

		return nil
	})

	ctx.Step(`^a (\w+) (?:transaction )?of R\$([0-9,.]+) from merchant "([^"]*)" is submitted$`, func(txType, amount, merchant string) error {
		amt, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		merchantID := sc.MerchantUUIDs[merchant]
		if merchantID == "" {
			merchantID = support.DeterministicMerchantUUID(merchant)
		}

		req := &testutil.ValidationRequest{
			RequestID:       testutil.MustDeterministicUUID(support.NextRequestID()).String(),
			TransactionType: normalizeTransactionType(txType),
			Amount:          amt,
			Currency:        "BRL",
			Merchant:        &testutil.MerchantContext{ID: merchantID, Name: merchant},
		}

		valResp, status, err := support.CreateValidationE(req)
		if err != nil {
			return fmt.Errorf("submitting validation: %w", err)
		}

		sc.LastValidation = valResp
		sc.LastValidationHTTP = status
		sc.ValidationHistory = append(sc.ValidationHistory, valResp)

		if status != http.StatusCreated {
			return fmt.Errorf("expected %d, got %d", http.StatusCreated, status)
		}

		return nil
	})

	// --- Assertion steps ---

	ctx.Step(`^the transaction should be allowed$`, func() error {
		if sc.LastValidation.ValidationID == "" {
			return fmt.Errorf("no validation result available")
		}

		if sc.LastValidation.Decision != "ALLOW" {
			return fmt.Errorf("expected ALLOW, got %q (reason: %s)", sc.LastValidation.Decision, sc.LastValidation.Reason)
		}

		return nil
	})

	ctx.Step(`^the transaction should be denied$`, func() error {
		if sc.LastValidation.ValidationID == "" {
			return fmt.Errorf("no validation result available")
		}

		if sc.LastValidation.Decision != "DENY" {
			return fmt.Errorf("expected DENY, got %q (reason: %s)", sc.LastValidation.Decision, sc.LastValidation.Reason)
		}

		return nil
	})

	ctx.Step(`^the transaction should be denied by a rule$`, func() error {
		if sc.LastValidation.ValidationID == "" {
			return fmt.Errorf("no validation result available")
		}

		if sc.LastValidation.Decision != "DENY" {
			return fmt.Errorf("expected DENY, got %q", sc.LastValidation.Decision)
		}

		if len(sc.LastValidation.MatchedRuleIDs) == 0 {
			return fmt.Errorf("expected matched rules, got none")
		}

		return nil
	})

	ctx.Step(`^the transaction should be denied because the limit was exceeded$`, func() error {
		if sc.LastValidation.ValidationID == "" {
			return fmt.Errorf("no validation result available")
		}

		if sc.LastValidation.Decision != "DENY" {
			return fmt.Errorf("expected DENY, got %q", sc.LastValidation.Decision)
		}

		for _, detail := range sc.LastValidation.LimitUsageDetails {
			if detail.Exceeded {
				return nil
			}
		}

		return fmt.Errorf("expected at least one limit to be exceeded")
	})

	ctx.Step(`^the transaction should be flagged for review$`, func() error {
		if sc.LastValidation.ValidationID == "" {
			return fmt.Errorf("no validation result available")
		}

		if sc.LastValidation.Decision != "REVIEW" {
			return fmt.Errorf("expected REVIEW, got %q (reason: %s)", sc.LastValidation.Decision, sc.LastValidation.Reason)
		}

		return nil
	})

	ctx.Step(`^the denial should reference the blocking rule$`, func() error {
		return assertMatchedRule(sc, "block high value pix new accounts")
	})

	ctx.Step(`^the denial should reference the adjusted rule$`, func() error {
		return assertMatchedRule(sc, "block high pix new non-vip")
	})

	ctx.Step(`^a reason for the denial should be provided$`, func() error {
		if sc.LastValidation.Reason == "" {
			return fmt.Errorf("expected a reason, got empty")
		}

		return nil
	})

	ctx.Step(`^the (\w+) rule should be referenced in the decision$`, func(ruleType string) error {
		return assertMatchedRule(sc, ruleType)
	})

	ctx.Step(`^the whitelist rule should be referenced in the decision$`, func() error {
		return assertMatchedRule(sc, "trusted merchant whitelist")
	})

	ctx.Step(`^the review rule should be referenced in the decision$`, func() error {
		return assertMatchedRule(sc, "review")
	})

	ctx.Step(`^no rules should have matched(?: the transaction)?$`, func() error {
		if len(sc.LastValidation.MatchedRuleIDs) > 0 {
			return fmt.Errorf("expected no matched rules, got %v", sc.LastValidation.MatchedRuleIDs)
		}

		return nil
	})

	ctx.Step(`^all three rules should have been evaluated$`, func() error {
		if len(sc.LastValidation.EvaluatedRuleIDs) < 3 {
			return fmt.Errorf("expected at least 3 evaluated rules, got %d", len(sc.LastValidation.EvaluatedRuleIDs))
		}

		return nil
	})

	ctx.Step(`^the deactivated deny rule should not have been evaluated$`, func() error {
		denyRuleID := sc.FindRuleID("Precedence DENY")
		if denyRuleID == "" {
			return fmt.Errorf("DENY rule not found in context")
		}

		for _, id := range sc.LastValidation.EvaluatedRuleIDs {
			if id == denyRuleID {
				return fmt.Errorf("deactivated DENY rule %s should not have been evaluated", denyRuleID)
			}
		}

		return nil
	})

	ctx.Step(`^no limits should have been evaluated$`, func() error {
		if len(sc.LastValidation.LimitUsageDetails) > 0 {
			return fmt.Errorf("expected no limit usage details, got %d", len(sc.LastValidation.LimitUsageDetails))
		}

		return nil
	})

	ctx.Step(`^the limit should not be exceeded$`, func() error {
		for _, detail := range sc.LastValidation.LimitUsageDetails {
			if detail.Exceeded {
				return fmt.Errorf("limit %s was exceeded", detail.LimitID)
			}
		}

		return nil
	})

	ctx.Step(`^the rule "([^"]*)" should be referenced in the decision$`, func(name string) error {
		return assertMatchedRule(sc, name)
	})

	// --- J7 DataTable step ---

	ctx.Step(`^the following transactions are submitted:$`, func(table *godog.Table) error {
		if len(table.Rows) < 2 {
			return fmt.Errorf("expected at least one data row in the table")
		}

		// Parse header to find column indices
		header := table.Rows[0]
		colMap := make(map[string]int)

		for i, cell := range header.Cells {
			colMap[strings.TrimSpace(cell.Value)] = i
		}

		typeCol, ok := colMap["type"]
		if !ok {
			return fmt.Errorf("missing 'type' column")
		}

		amountCol, ok := colMap["amount"]
		if !ok {
			return fmt.Errorf("missing 'amount' column")
		}

		expectedCol, ok := colMap["expected outcome"]
		if !ok {
			return fmt.Errorf("missing 'expected outcome' column")
		}

		// Store expected outcomes for the assertion step
		sc.PendingTransaction = nil // Clear any pending

		// Use a unique account for DataTable submissions (J7) to avoid
		// cross-feature interference with limits scoped to TestAccountUUID (J3).
		analysisAccountID := support.DeterministicAccountUUID("analysis")

		for _, row := range table.Rows[1:] {
			txType := strings.ToUpper(strings.TrimSpace(row.Cells[typeCol].Value))
			amountStr := strings.TrimPrefix(strings.TrimSpace(row.Cells[amountCol].Value), "R$")
			amt, err := decimal.NewFromString(normalizeAmount(amountStr))
			if err != nil {
				return fmt.Errorf("parsing amount %q: %w", amountStr, err)
			}

			expectedOutcome := strings.ToLower(strings.TrimSpace(row.Cells[expectedCol].Value))

			req := &testutil.ValidationRequest{
				RequestID:       testutil.MustDeterministicUUID(support.NextRequestID()).String(),
				TransactionType: txType,
				Amount:          amt,
				Currency:        "BRL",
				Account:         &testutil.AccountContext{ID: analysisAccountID},
			}

			valResp, status, err := support.CreateValidationE(req)
			if err != nil {
				return fmt.Errorf("submitting %s of R$%s: %w", txType, amt, err)
			}

			if status != http.StatusCreated {
				return fmt.Errorf("submitting %s of R$%s: expected %d, got %d", txType, amt, http.StatusCreated, status)
			}

			sc.ValidationHistory = append(sc.ValidationHistory, valResp)

			// Verify outcome matches expected
			var expectedDecision string
			switch expectedOutcome {
			case "denied":
				expectedDecision = "DENY"
			case "allowed":
				expectedDecision = "ALLOW"
			case "flagged":
				expectedDecision = "REVIEW"
			default:
				return fmt.Errorf("unknown expected outcome %q", expectedOutcome)
			}

			if valResp.Decision != expectedDecision {
				return fmt.Errorf("%s of R$%s: expected %s, got %s (reason: %s)",
					txType, amt, expectedDecision, valResp.Decision, valResp.Reason)
			}
		}

		return nil
	})

	ctx.Step(`^each transaction should receive the expected outcome$`, func() error {
		// The DataTable step already verified each outcome inline.
		// This step exists to complete the Gherkin narrative.
		if len(sc.ValidationHistory) == 0 {
			return fmt.Errorf("no validation history — DataTable step may not have run")
		}

		return nil
	})
}

// assertMatchedRule checks if any matched rule ID corresponds to a rule with the given name fragment.
// It first checks the local sc.Rules map, then falls back to fetching each matched rule by ID from the API.
func assertMatchedRule(sc *support.ScenarioContext, nameFragment string) error {
	fragment := strings.ToLower(nameFragment)

	// First: check local rules map
	for storedName, storedID := range sc.Rules {
		if strings.Contains(strings.ToLower(storedName), fragment) {
			for _, matchedID := range sc.LastValidation.MatchedRuleIDs {
				if matchedID == storedID {
					return nil
				}
			}
		}
	}

	// Fallback: fetch each matched rule by ID and check its name.
	// This handles cross-scenario cases where sc.Rules is empty.
	for _, matchedID := range sc.LastValidation.MatchedRuleIDs {
		rule, status, err := support.GetRuleE(matchedID)
		if err != nil || status != http.StatusOK {
			continue
		}

		if strings.Contains(strings.ToLower(rule.Name), fragment) {
			sc.Rules[rule.Name] = rule.ID

			return nil
		}
	}

	return fmt.Errorf("no matched rule contains %q (matched: %v, known rules: %v)",
		nameFragment, sc.LastValidation.MatchedRuleIDs, sc.Rules)
}

// normalizeTransactionType maps business-language transaction types to valid Tracer API types.
// Valid API types: CARD, WIRE, PIX, CRYPTO.
func normalizeTransactionType(s string) string {
	upper := strings.ToUpper(strings.TrimSpace(s))

	switch upper {
	case "PAYMENT", "CARD":
		return "CARD"
	default:
		return upper
	}
}

// mapCountryToISO maps human-readable country names to ISO 3166-1 alpha-2 codes.
// The mapping is intentionally limited to the countries used in BDD test
// scenarios. Extend the map only when a new feature file introduces a country
// not yet covered.
func mapCountryToISO(country string) string {
	countryMap := map[string]string{
		"brazil":            "BR",
		"the united states": "US",
		"united states":     "US",
		"us":                "US",
		"usa":               "US",
	}

	lower := strings.ToLower(strings.TrimSpace(country))
	if code, ok := countryMap[lower]; ok {
		return code
	}

	// If already an ISO code, return as-is
	if len(country) == 2 {
		return strings.ToUpper(country)
	}

	return country
}
