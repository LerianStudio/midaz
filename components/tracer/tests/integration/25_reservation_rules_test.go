// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
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

	payload := map[string]any{
		"transactionId":        uuid.New().String(),
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
