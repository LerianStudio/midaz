// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// =============================================================================
// Limit merchant-scope enforcement test
//
// Contract under test: a limit scoped to a specific merchantId must be
// enforced only when the incoming validation request carries a merchant
// context with a matching merchantId. Other transactions (no merchant
// context, or a different merchantId) must not see the limit in
// limitUsageDetails.
//
// End-to-end path exercised:
//
//	ValidationRequest.Merchant.ID
//	    -> ValidationRequest.ToCheckLimitsInput() propagates MerchantID
//	    -> buildTransactionScope() copies MerchantID into the tx Scope
//	    -> Scope.Matches() compares the limit's scope against the tx scope
//	    -> scopeMatchesLimit() returns true only when merchantIds align
//	    -> response.limitUsageDetails includes / excludes the limit accordingly
// =============================================================================

// TestLimitMerchantScope_Integration verifies that a limit scoped to a
// specific merchantId is enforced on transactions for that merchant and not
// enforced on transactions for a different merchant.
func TestLimitMerchantScope_Integration(t *testing.T) {
	targetMerchantID := testutil.MustDeterministicUUID(100100).String()
	otherMerchantID := testutil.MustDeterministicUUID(100101).String()
	accountID := testutil.MustDeterministicUUID(100102).String()

	// ----- Setup: create + activate a DAILY limit scoped to targetMerchantID.
	// Derived deterministic suffix from a fixed seed to keep failures
	// reproducible locally while still avoiding name collisions with other
	// tests in this suite.
	limitID := testutil.CreateLimitWithScope(
		t,
		"merchant-scope-limit-"+testutil.MustDeterministicUUID(100110).String()[:8],
		"1000",
		[]testutil.ScopeInput{{MerchantID: &targetMerchantID}},
	)
	testutil.ActivateLimit(t, limitID)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// ----- Case 1: request for the matching merchant should see the limit.
	matchingReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(100110).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Merchant: &testutil.MerchantContext{
			ID:   targetMerchantID,
			Name: "Target Merchant",
		},
	}

	matchingResp, matchingBody := testutil.CreateValidation(t, matchingReq)
	defer matchingResp.Body.Close()
	require.Equal(t, http.StatusCreated, matchingResp.StatusCode,
		"expected 201 for matching-merchant validation: %s", string(matchingBody))

	var matchingResult testutil.ValidationResponse
	require.NoError(t, json.Unmarshal(matchingBody, &matchingResult))

	foundForMatching := false

	for _, detail := range matchingResult.LimitUsageDetails {
		if detail.LimitID == limitID {
			foundForMatching = true

			// Defensive sanity-checks on the enforced limit details.
			assert.False(t, detail.Exceeded,
				"first matching transaction (amount=100, cap=1000) must not exceed limit")
			assert.True(t, decimal.RequireFromString("100").Equal(detail.AttemptedAmount),
				"attemptedAmount should mirror the request amount")

			break
		}
	}

	assert.True(t, foundForMatching,
		"limitUsageDetails must contain the merchant-scoped limit when request merchantId matches")

	// ----- Case 2: request for a different merchant must not see the limit.
	otherReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(100111).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
		Merchant: &testutil.MerchantContext{
			ID:   otherMerchantID,
			Name: "Other Merchant",
		},
	}

	otherResp, otherBody := testutil.CreateValidation(t, otherReq)
	defer otherResp.Body.Close()
	require.Equal(t, http.StatusCreated, otherResp.StatusCode,
		"expected 201 for other-merchant validation: %s", string(otherBody))

	var otherResult testutil.ValidationResponse
	require.NoError(t, json.Unmarshal(otherBody, &otherResult))

	for _, detail := range otherResult.LimitUsageDetails {
		assert.NotEqual(t, limitID, detail.LimitID,
			"limit scoped to merchantId=%s must NOT be evaluated for merchantId=%s",
			targetMerchantID, otherMerchantID)
	}

	// ----- Case 3: request with no merchant context must not see the limit.
	noMerchantReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(100112).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	noMerchantResp, noMerchantBody := testutil.CreateValidation(t, noMerchantReq)
	defer noMerchantResp.Body.Close()
	require.Equal(t, http.StatusCreated, noMerchantResp.StatusCode,
		"expected 201 for no-merchant validation: %s", string(noMerchantBody))

	var noMerchantResult testutil.ValidationResponse
	require.NoError(t, json.Unmarshal(noMerchantBody, &noMerchantResult))

	for _, detail := range noMerchantResult.LimitUsageDetails {
		assert.NotEqual(t, limitID, detail.LimitID,
			"limit scoped to merchantId=%s must NOT be evaluated when merchant context is absent",
			targetMerchantID)
	}
}
