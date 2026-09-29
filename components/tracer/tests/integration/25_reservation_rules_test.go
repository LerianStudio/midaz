// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	testutil_integration "github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_integration"
)

// reserveRulesResponse is the POST /v1/reservations body as a client reads it.
type reserveRulesResponse struct {
	TransactionID  string   `json:"transactionId"`
	Denied         bool     `json:"denied"`
	Decision       string   `json:"decision"`
	Reason         string   `json:"reason"`
	MatchedRuleIDs []string `json:"matchedRuleIds"`
	ReservationIDs []string `json:"reservationIds"`
}

// postRulesReservation reserves for accountID with a ledger-shaped body: free-form
// account type, flat metadata, no transactionType.
func postRulesReservation(t *testing.T, accountID uuid.UUID, amount string) (reserveRulesResponse, []byte) {
	t.Helper()

	return postRulesReservationFor(t, uuid.New(), accountID, amount, false)
}

// postRulesReservationFor is postRulesReservation with an explicit ledger
// transaction id and revert flag.
func postRulesReservationFor(t *testing.T, transactionID, accountID uuid.UUID, amount string, revert bool) (reserveRulesResponse, []byte) {
	t.Helper()

	payload := map[string]any{
		"transactionId":        transactionID.String(),
		"requestId":            uuid.New().String(),
		"amount":               amount,
		"asset":                "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": accountID.String(),
			"type":      "deposit",
		},
		"metadata": map[string]any{"channel": "app"},
	}

	if revert {
		payload["revert"] = true
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, testutil.GetBaseURL()+"/v1/reservations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", testutil.GetAPIKey())
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "reserve must answer 201, got body: %s", string(respBody))

	var out reserveRulesResponse
	require.NoError(t, json.Unmarshal(respBody, &out))

	return out, respBody
}

// createScopedReserveRule creates and activates a rule scoped to one account so it
// cannot leak into any other test's requests.
func createScopedReserveRule(t *testing.T, name, expression, action string, accountID uuid.UUID) string {
	t.Helper()

	scopedAccount := accountID.String()
	ruleID := testutil.CreateRuleWithScope(t, name, expression, action,
		[]testutil.ScopeInput{{AccountID: &scopedAccount}})
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	return ruleID
}

// TestIntegration_Reservation_RulesDecide proves the reserve path evaluates the
// CEL rules before limits: a rule DENY or REVIEW refuses the reserve with the
// matched rule ids and holds no capacity, while a request no rule refuses carries
// decision ALLOW. The rule expressions read the ledger-shaped account type and
// metadata, so they only match when both reach the CEL context.
func TestIntegration_Reservation_RulesDecide(t *testing.T) {
	t.Run("DENY rule refuses the reserve", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(96101)
		ruleID := createScopedReserveRule(t, "Reserve rules DENY deposit via app",
			`account.type == "deposit" && metadata["channel"] == "app"`, "DENY", accountID)

		got, raw := postRulesReservation(t, accountID, "100.00")

		assert.True(t, got.Denied)
		assert.Equal(t, "DENY", got.Decision)
		assert.Equal(t, []string{ruleID}, got.MatchedRuleIDs)
		assert.Empty(t, got.ReservationIDs)
		assert.Contains(t, string(raw), `"reservationIds":[]`)
	})

	t.Run("REVIEW rule refuses the reserve", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(96102)
		ruleID := createScopedReserveRule(t, "Reserve rules REVIEW deposit",
			`account.type == "deposit"`, "REVIEW", accountID)

		got, _ := postRulesReservation(t, accountID, "100.00")

		assert.True(t, got.Denied)
		assert.Equal(t, "REVIEW", got.Decision)
		assert.Equal(t, []string{ruleID}, got.MatchedRuleIDs)
		assert.Empty(t, got.ReservationIDs)
	})

	t.Run("no refusing rule allows the reserve", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(96103)
		createScopedReserveRule(t, "Reserve rules DENY other channel",
			`metadata["channel"] == "branch"`, "DENY", accountID)

		got, raw := postRulesReservation(t, accountID, "100.00")

		assert.False(t, got.Denied)
		assert.Equal(t, "ALLOW", got.Decision)
		assert.Empty(t, got.MatchedRuleIDs)
		assert.Contains(t, string(raw), `"matchedRuleIds":[]`)
	})
}

// createActiveAccountLimit creates and activates a DAILY BRL limit scoped to
// accountID, so a reserve for that account resolves exactly one counter-backed
// limit.
func createActiveAccountLimit(t *testing.T, accountID uuid.UUID, maxAmount string) string {
	t.Helper()

	limitID := testutil.CreateLimitWithAccountScope(t, accountID.String(), maxAmount)
	testutil.ActivateLimit(t, limitID)

	t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

	return limitID
}

// limitUsage returns the current plus reserved usage the limit's counters hold.
func limitUsage(t *testing.T, db *sql.DB, limitID string) decimal.Decimal {
	t.Helper()

	var usage decimal.Decimal

	err := db.QueryRow(
		`SELECT COALESCE(SUM(current_usage + reserved_usage), 0) FROM usage_counters WHERE limit_id = $1`,
		limitID,
	).Scan(&usage)
	require.NoError(t, err)

	return usage
}

// reservationRowCount returns how many reservation rows a ledger transaction holds.
func reservationRowCount(t *testing.T, db *sql.DB, transactionID uuid.UUID) int {
	t.Helper()

	var count int

	err := db.QueryRow(`SELECT COUNT(*) FROM usage_reservations WHERE transaction_id = $1`, transactionID).Scan(&count)
	require.NoError(t, err)

	return count
}

// TestIntegration_Reservation_RulesGuardLimitCapacity proves a rule refusal
// holds no limit capacity while an admitted reserve holds exactly one
// reservation for an account-scoped limit, and covers the reserve-only rule
// policy: an unevaluable rule routes to REVIEW, and a revert skips the rules.
func TestIntegration_Reservation_RulesGuardLimitCapacity(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	for i, action := range []string{"DENY", "REVIEW"} {
		t.Run(action+" rule holds no capacity on an account-scoped limit", func(t *testing.T) {
			accountID := testutil.MustDeterministicUUID(int64(96201 + i))
			limitID := createActiveAccountLimit(t, accountID, "1000")
			ruleID := createScopedReserveRule(t, "Reserve capacity "+action+" deposit",
				`account.type == "deposit"`, action, accountID)

			transactionID := uuid.New()
			got, _ := postRulesReservationFor(t, transactionID, accountID, "100.00", false)

			assert.True(t, got.Denied)
			assert.Equal(t, action, got.Decision)
			assert.Equal(t, []string{ruleID}, got.MatchedRuleIDs)
			assert.Empty(t, got.ReservationIDs)
			assert.True(t, limitUsage(t, db, limitID).IsZero(), "a refused reserve must not move the counter")
			assert.Zero(t, reservationRowCount(t, db, transactionID), "a refused reserve must not write a reservation row")
		})
	}

	t.Run("ALLOW holds exactly one reservation on an account-scoped limit", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(96203)
		limitID := createActiveAccountLimit(t, accountID, "1000")
		createScopedReserveRule(t, "Reserve capacity DENY other channel",
			`metadata["channel"] == "branch"`, "DENY", accountID)

		transactionID := uuid.New()
		got, _ := postRulesReservationFor(t, transactionID, accountID, "100.00", false)

		assert.False(t, got.Denied)
		assert.Equal(t, "ALLOW", got.Decision)
		require.Len(t, got.ReservationIDs, 1)
		assert.Equal(t, 1, reservationRowCount(t, db, transactionID))
		assert.True(t, limitUsage(t, db, limitID).IsPositive(), "the admitted reserve must hold its amount")
	})

	t.Run("rule evaluation error routes the reserve to REVIEW without capacity", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(96204)
		limitID := createActiveAccountLimit(t, accountID, "1000")
		// Compiles (metadata values are dyn) but fails at runtime: string + int
		// has no overload.
		ruleID := createScopedReserveRule(t, "Reserve capacity unevaluable rule",
			`metadata["channel"] + 1 > 0`, "DENY", accountID)

		transactionID := uuid.New()
		got, _ := postRulesReservationFor(t, transactionID, accountID, "100.00", false)

		assert.True(t, got.Denied)
		assert.Equal(t, "REVIEW", got.Decision)
		assert.Equal(t, "rule_evaluation_error", got.Reason)
		assert.Equal(t, []string{ruleID}, got.MatchedRuleIDs)
		assert.Empty(t, got.ReservationIDs)
		assert.True(t, limitUsage(t, db, limitID).IsZero())
		assert.Zero(t, reservationRowCount(t, db, transactionID))
	})

	t.Run("revert skips a matching DENY rule and still reserves the limit", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(96205)
		limitID := createActiveAccountLimit(t, accountID, "1000")
		createScopedReserveRule(t, "Reserve capacity DENY deposit on revert",
			`account.type == "deposit"`, "DENY", accountID)

		transactionID := uuid.New()
		got, _ := postRulesReservationFor(t, transactionID, accountID, "100.00", true)

		assert.False(t, got.Denied)
		assert.Equal(t, "ALLOW", got.Decision)
		assert.Empty(t, got.MatchedRuleIDs)
		require.Len(t, got.ReservationIDs, 1)
		assert.Equal(t, 1, reservationRowCount(t, db, transactionID))
		assert.True(t, limitUsage(t, db, limitID).IsPositive())
	})
}

// TestIntegration_Reservation_NoMatchDefaultDoesNotRefuse proves that on the
// reserve path only a matched rule refuses: with DEFAULT_DECISION_WHEN_NO_MATCH
// set to DENY, a reserve no rule matches is still admitted and reserves its
// limit.
//
// It restarts the server, so it cannot run in parallel with other restarting
// tests.
func TestIntegration_Reservation_NoMatchDefaultDoesNotRefuse(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"DEFAULT_DECISION_WHEN_NO_MATCH": "DENY",
	})
	require.NoError(t, err, "restart server with DENY no-match default")

	t.Cleanup(func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Errorf("restore server config: %v", cleanupErr)
		}
	})

	db := testutil.SetupIntegrationDB(t)

	accountID := testutil.MustDeterministicUUID(96301)
	limitID := createActiveAccountLimit(t, accountID, "1000")
	createScopedReserveRule(t, "Reserve no-match default DENY other channel",
		`metadata["channel"] == "branch"`, "DENY", accountID)

	transactionID := uuid.New()
	got, _ := postRulesReservationFor(t, transactionID, accountID, "100.00", false)

	assert.False(t, got.Denied)
	assert.Equal(t, "ALLOW", got.Decision)
	assert.Empty(t, got.MatchedRuleIDs)
	require.Len(t, got.ReservationIDs, 1)
	assert.Equal(t, 1, reservationRowCount(t, db, transactionID))
	assert.True(t, limitUsage(t, db, limitID).IsPositive())
}
