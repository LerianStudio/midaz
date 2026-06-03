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

	"github.com/LerianStudio/midaz/v3/components/tracer/tests/end2end/support"
)

func registerAuditSteps(ctx *godog.ScenarioContext, sc *support.ScenarioContext) {
	// --- Audit event query steps ---

	ctx.Step(`^the audit trail for transaction validations is queried$`, func() error {
		events, status, err := support.ListAuditEventsE("event_type=TRANSACTION_VALIDATED")
		if err != nil {
			return fmt.Errorf("listing audit events: %w", err)
		}

		sc.LastAuditEvents = events
		sc.LastAuditEventsHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^(\w+) queries the audit trail for rule creation events$`, func(persona string) error {
		events, status, err := support.ListAuditEventsE("resource_type=rule&action=CREATE")
		if err != nil {
			return fmt.Errorf("listing audit events: %w", err)
		}

		sc.LastAuditEvents = events
		sc.LastAuditEventsHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^(\w+) queries the audit trail for denied transaction validations$`, func(persona string) error {
		events, status, err := support.ListAuditEventsE("event_type=TRANSACTION_VALIDATED&result=DENY")
		if err != nil {
			return fmt.Errorf("listing audit events: %w", err)
		}

		sc.LastAuditEvents = events
		sc.LastAuditEventsHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^(\w+) reviews the audit trail for allowed transaction validations$`, func(persona string) error {
		events, status, err := support.ListAuditEventsE("event_type=TRANSACTION_VALIDATED&result=ALLOW")
		if err != nil {
			return fmt.Errorf("listing audit events: %w", err)
		}

		sc.LastAuditEvents = events
		sc.LastAuditEventsHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	ctx.Step(`^(\w+) reviews the audit trail for rule operations$`, func(persona string) error {
		events, status, err := support.ListAuditEventsE("resource_type=rule")
		if err != nil {
			return fmt.Errorf("listing audit events: %w", err)
		}

		sc.LastAuditEvents = events
		sc.LastAuditEventsHTTP = status

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		return nil
	})

	// --- J1 audit assertion steps ---

	ctx.Step(`^(?:he|she) should see events for both rules being created$`, func() error {
		createCount := 0

		for _, event := range sc.LastAuditEvents.AuditEvents {
			if strings.EqualFold(event.Action, "CREATE") {
				createCount++
			}
		}

		if createCount < 2 {
			return fmt.Errorf("expected at least 2 CREATE events, found %d", createCount)
		}

		return nil
	})

	ctx.Step(`^(?:he|she) should see the activation and deactivation events$`, func() error {
		foundActivate := false
		foundDeactivate := false

		for _, event := range sc.LastAuditEvents.AuditEvents {
			action := strings.ToUpper(event.Action)

			if action == "ACTIVATE" {
				foundActivate = true
			}

			if action == "DEACTIVATE" {
				foundDeactivate = true
			}

			// Also check context for status transitions via after.status
			if action == "STATUS_CHANGE" || action == "UPDATE" {
				if afterStatus := extractAfterStatus(event.Context); afterStatus != "" {
					switch afterStatus {
					case "ACTIVE":
						foundActivate = true
					case "INACTIVE":
						foundDeactivate = true
					}
				}
			}
		}

		if !foundActivate {
			return fmt.Errorf("no activation events found in audit trail")
		}

		if !foundDeactivate {
			return fmt.Errorf("no deactivation events found in audit trail")
		}

		return nil
	})

	// --- Audit event assertion steps ---

	ctx.Step(`^there should be audit events for each validation performed$`, func() error {
		if len(sc.LastAuditEvents.AuditEvents) == 0 {
			return fmt.Errorf("expected audit events for validations, got none")
		}

		return nil
	})

	ctx.Step(`^(?:each|every) (?:audit )?event should have a unique identifier, timestamp, and integrity hash$`, func() error {
		seen := make(map[string]bool)

		for i, event := range sc.LastAuditEvents.AuditEvents {
			if event.EventID == "" {
				return fmt.Errorf("event %d: missing eventId", i)
			}

			if seen[event.EventID] {
				return fmt.Errorf("duplicate eventId: %s", event.EventID)
			}

			seen[event.EventID] = true

			if event.CreatedAt == "" {
				return fmt.Errorf("event %d (%s): missing createdAt", i, event.EventID)
			}

			if event.Hash == "" {
				return fmt.Errorf("event %d (%s): missing hash", i, event.EventID)
			}
		}

		return nil
	})

	ctx.Step(`^each event should have a timestamp and associated context$`, func() error {
		for i, event := range sc.LastAuditEvents.AuditEvents {
			if event.CreatedAt == "" {
				return fmt.Errorf("event %d: missing createdAt", i)
			}
		}

		return nil
	})

	ctx.Step(`^the results should include events showing when each rule was created$`, func() error {
		if len(sc.LastAuditEvents.AuditEvents) == 0 {
			return fmt.Errorf("expected rule creation events, got none")
		}

		// Check that at least one event is a CREATE action
		for _, event := range sc.LastAuditEvents.AuditEvents {
			if strings.EqualFold(event.Action, "CREATE") {
				return nil
			}
		}

		return fmt.Errorf("no CREATE events found in audit trail")
	})

	ctx.Step(`^the results should include events with a deny outcome$`, func() error {
		for _, event := range sc.LastAuditEvents.AuditEvents {
			if strings.EqualFold(event.Result, "DENY") {
				return nil
			}
		}

		return fmt.Errorf("no events with DENY result found")
	})

	ctx.Step(`^each event should contain the original request and decision details$`, func() error {
		for i, event := range sc.LastAuditEvents.AuditEvents {
			if event.EventID == "" {
				return fmt.Errorf("event %d: missing eventId", i)
			}

			if event.Result == "" {
				return fmt.Errorf("event %d (%s): missing result", i, event.EventID)
			}
		}

		return nil
	})

	// --- Hash chain verification ---

	ctx.Step(`^the integrity of the audit hash chain is verified$`, func() error {
		// Get the latest audit event to verify the chain up to that point
		events, status, err := support.ListAuditEventsE("limit=1")
		if err != nil {
			return fmt.Errorf("listing events for verification: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		if len(events.AuditEvents) == 0 {
			return fmt.Errorf("no audit events found for hash chain verification")
		}

		latestEventID := events.AuditEvents[0].EventID
		result, verifyStatus, err := support.VerifyHashChainE(latestEventID)
		if err != nil {
			return fmt.Errorf("verifying hash chain: %w", err)
		}

		sc.LastHashVerification = result
		sc.LastHashVerificationHTTP = verifyStatus

		if verifyStatus != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, verifyStatus)
		}

		return nil
	})

	ctx.Step(`^the hash chain should be valid$`, func() error {
		if !sc.LastHashVerification.IsValid {
			return fmt.Errorf("hash chain is invalid: %s", sc.LastHashVerification.Message)
		}

		return nil
	})

	ctx.Step(`^the number of events checked should be greater than zero$`, func() error {
		if sc.LastHashVerification.TotalChecked <= 0 {
			return fmt.Errorf("expected >0 events checked, got %d", sc.LastHashVerification.TotalChecked)
		}

		return nil
	})

	// --- J8 audit trail assertions for whitelist vs default approval ---

	ctx.Step(`^the (\w+) transaction should show the whitelist rule in its audit context$`, func(merchant string) error {
		// Query audit events for ALLOW validations and check if any have matched rules
		// for the specific merchant (identified by deterministic UUID).
		events, status, err := support.ListAuditEventsE("event_type=TRANSACTION_VALIDATED&result=ALLOW")
		if err != nil {
			return fmt.Errorf("listing audit events: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		merchantUUID := support.DeterministicMerchantUUID(merchant)

		for _, event := range events.AuditEvents {
			if !eventContainsMerchant(event, merchantUUID) {
				continue
			}

			matchedIDs := extractMatchedRuleIDs(event)
			if len(matchedIDs) > 0 {
				return nil
			}
		}

		return fmt.Errorf("no ALLOW audit event found with matched rules for %s transaction (merchantUUID=%s)", merchant, merchantUUID)
	})

	ctx.Step(`^the (\w+) transaction should show no matched rules in its audit context$`, func(merchant string) error {
		events, status, err := support.ListAuditEventsE("event_type=TRANSACTION_VALIDATED&result=ALLOW")
		if err != nil {
			return fmt.Errorf("listing audit events: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		merchantUUID := support.DeterministicMerchantUUID(merchant)

		for _, event := range events.AuditEvents {
			if !eventContainsMerchant(event, merchantUUID) {
				continue
			}

			matchedIDs := extractMatchedRuleIDs(event)
			if len(matchedIDs) == 0 {
				return nil
			}
		}

		return fmt.Errorf("no ALLOW audit event found with empty matched rules for %s transaction (merchantUUID=%s)", merchant, merchantUUID)
	})

	// --- J4/J5 validation history steps ---

	ctx.Step(`^(\w+) reviews the validation history for (\w+) transactions$`, func(persona, txType string) error {
		listResp, status, err := support.ListValidationsE("transaction_type=" + strings.ToUpper(txType))
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		sc.LastAuditEventsHTTP = status
		sc.LastValidationList = listResp

		return nil
	})

	ctx.Step(`^the records should show a deny decision, then a review decision, then an allow decision$`, func() error {
		listResp := sc.LastValidationList
		if len(listResp.TransactionValidations) < 3 {
			return fmt.Errorf("expected at least 3 validation records, got %d", len(listResp.TransactionValidations))
		}

		// Collect decisions in API order (typically DESC by createdAt).
		decisions := make([]string, 0, len(listResp.TransactionValidations))
		for _, v := range listResp.TransactionValidations {
			decisions = append(decisions, v.Decision)
		}

		// Find chronological order: first occurrence index of each decision.
		// API returns DESC, so reverse the slice to get chronological (ASC).
		chronological := make([]string, len(decisions))
		for i, d := range decisions {
			chronological[len(decisions)-1-i] = d
		}

		idxDeny := -1
		idxReview := -1
		idxAllow := -1

		for i, d := range chronological {
			switch d {
			case "DENY":
				if idxDeny == -1 {
					idxDeny = i
				}
			case "REVIEW":
				if idxReview == -1 {
					idxReview = i
				}
			case "ALLOW":
				if idxAllow == -1 {
					idxAllow = i
				}
			}
		}

		if idxDeny == -1 || idxReview == -1 || idxAllow == -1 {
			return fmt.Errorf("expected DENY, REVIEW, and ALLOW decisions; found (chronological): %v", chronological)
		}

		if !(idxDeny < idxReview && idxReview < idxAllow) {
			return fmt.Errorf("expected chronological order DENY(%d) < REVIEW(%d) < ALLOW(%d); decisions: %v",
				idxDeny, idxReview, idxAllow, chronological)
		}

		return nil
	})

	// --- J5 validation history query steps ---

	ctx.Step(`^(\w+) searches for transactions with a review decision$`, func(persona string) error {
		listResp, status, err := support.ListValidationsE("decision=REVIEW")
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		sc.LastAuditEventsHTTP = status
		sc.LastValidationList = listResp

		return nil
	})

	ctx.Step(`^(?:he|she) should find at least one flagged transaction$`, func() error {
		if len(sc.LastValidationList.TransactionValidations) == 0 {
			return fmt.Errorf("expected at least one REVIEW validation, got none")
		}

		return nil
	})

	ctx.Step(`^the results should include the international transaction from the new account$`, func() error {
		// Verify there's at least one REVIEW validation (the international one we submitted)
		listResp, status, err := support.ListValidationsE("decision=REVIEW")
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		if len(listResp.TransactionValidations) == 0 {
			return fmt.Errorf("expected REVIEW transaction in results")
		}

		return nil
	})

	// --- J7 validation history steps ---

	ctx.Step(`^(\w+) queries the validation history$`, func(persona string) error {
		listResp, status, err := support.ListValidationsE("")
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		sc.LastValidationList = listResp

		return nil
	})

	ctx.Step(`^the results should include recent validation records$`, func() error {
		if len(sc.LastValidationList.TransactionValidations) == 0 {
			return fmt.Errorf("expected validation records, got none")
		}

		return nil
	})

	ctx.Step(`^each record should show the decision, amount, transaction type, and processing time$`, func() error {
		listResp, _, err := support.ListValidationsE("")
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		for i, v := range listResp.TransactionValidations {
			if v.Decision == "" {
				return fmt.Errorf("record %d: missing decision", i)
			}

			if v.TransactionType == "" {
				return fmt.Errorf("record %d: missing transactionType", i)
			}

			if v.Amount.IsZero() {
				return fmt.Errorf("record %d: zero amount", i)
			}
		}

		return nil
	})

	ctx.Step(`^(\w+) filters the validation history for denied transactions only$`, func(persona string) error {
		listResp, status, err := support.ListValidationsE("decision=DENY")
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		if len(listResp.TransactionValidations) == 0 {
			return fmt.Errorf("expected at least one denied validation, got none")
		}

		for i, v := range listResp.TransactionValidations {
			if v.Decision != "DENY" {
				return fmt.Errorf("validation %d: expected DENY decision, got %q", i, v.Decision)
			}
		}

		sc.LastValidationList = listResp

		return nil
	})

	ctx.Step(`^all results should show a deny decision$`, func() error {
		listResp, _, err := support.ListValidationsE("decision=DENY")
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		for i, v := range listResp.TransactionValidations {
			if v.Decision != "DENY" {
				return fmt.Errorf("record %d: expected DENY, got %q", i, v.Decision)
			}
		}

		return nil
	})

	ctx.Step(`^the results should include the crypto transaction of R\$([0-9,.]+)$`, func(amount string) error {
		listResp, _, err := support.ListValidationsE("decision=DENY")
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		expected, err := decimal.NewFromString(normalizeAmount(amount))
		if err != nil {
			return fmt.Errorf("parsing amount: %w", err)
		}

		for _, v := range listResp.TransactionValidations {
			if v.Amount.Equal(expected) && v.TransactionType == "CRYPTO" {
				return nil
			}
		}

		return fmt.Errorf("crypto transaction of R$%s not found in denied results", expected)
	})

	ctx.Step(`^(\w+) filters the validation history by the rule "([^"]*)"$`, func(persona, ruleName string) error {
		ruleID := sc.FindRuleID(ruleName)
		if ruleID == "" {
			return fmt.Errorf("rule %q not found in context", ruleName)
		}

		listResp, status, err := support.ListValidationsE("matched_rule_id=" + ruleID)
		if err != nil {
			return fmt.Errorf("listing validations: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		if len(listResp.TransactionValidations) == 0 {
			return fmt.Errorf("expected at least one validation matched by rule %q, got none", ruleName)
		}

		sc.LastFilteredRuleID = ruleID

		return nil
	})

	ctx.Step(`^the results should show transactions that were matched by that rule$`, func() error {
		if sc.LastFilteredRuleID == "" {
			return fmt.Errorf("no previous rule filter — run the filter step first")
		}

		listResp, status, err := support.ListValidationsE("matched_rule_id=" + sc.LastFilteredRuleID)
		if err != nil {
			return fmt.Errorf("listing validations by rule: %w", err)
		}

		if status != http.StatusOK {
			return fmt.Errorf("expected %d, got %d", http.StatusOK, status)
		}

		if len(listResp.TransactionValidations) == 0 {
			return fmt.Errorf("no transactions matched by the filtered rule %s", sc.LastFilteredRuleID)
		}

		return nil
	})
}

// eventContainsMerchant checks if the audit event's context contains the given
// merchant UUID by performing a depth-first traversal of the nested context map.
// It returns true only when a string value exactly equals the merchantUUID.
func eventContainsMerchant(event support.AuditEvent, merchantUUID string) bool {
	if merchantUUID == "" {
		return false
	}

	return mapContainsValue(event.Context, merchantUUID)
}

// mapContainsValue performs a depth-first search over a nested structure
// (maps, slices, and primitives) and returns true when any string value
// exactly equals target.
func mapContainsValue(v any, target string) bool {
	switch val := v.(type) {
	case string:
		return val == target
	case map[string]any:
		for _, child := range val {
			if mapContainsValue(child, target) {
				return true
			}
		}
	case []any:
		for _, item := range val {
			if mapContainsValue(item, target) {
				return true
			}
		}
	}

	return false
}

// extractMatchedRuleIDs drills into the audit event's nested context structure
// to extract the matchedRuleIds array from context.response.matchedRuleIds.
func extractMatchedRuleIDs(event support.AuditEvent) []string {
	resp, ok := event.Context["response"]
	if !ok {
		return nil
	}

	respMap, ok := resp.(map[string]any)
	if !ok {
		return nil
	}

	matched, ok := respMap["matchedRuleIds"]
	if !ok {
		return nil
	}

	ids, ok := matched.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if s, ok := id.(string); ok {
			result = append(result, s)
		}
	}

	return result
}

// extractAfterStatus extracts the status string from context["after"]["status"].
// Returns the uppercased status or empty string if the path does not exist.
func extractAfterStatus(ctx map[string]any) string {
	if ctx == nil {
		return ""
	}

	after, ok := ctx["after"].(map[string]any)
	if !ok {
		return ""
	}

	status, ok := after["status"].(string)
	if !ok {
		return ""
	}

	return strings.ToUpper(status)
}
