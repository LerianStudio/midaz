// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ToValidationResponse Tests
// =============================================================================

func TestTransactionValidation_ToValidationResponse(t *testing.T) {
	t.Parallel()

	// Fixed times for deterministic testing
	fixedCreatedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	fixedTxTimestamp := time.Date(2024, 1, 15, 10, 29, 0, 0, time.UTC)

	// Deterministic UUIDs
	validationID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	requestID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440100")
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440200")
	matchedRuleID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440010")
	evaluatedRuleID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440011")
	limitID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440030")

	tests := []struct {
		name       string
		validation *TransactionValidation
		wantResp   *ValidationResponse
	}{
		{
			name: "converts minimal entity to response",
			validation: &TransactionValidation{
				ID:        validationID,
				RequestID: requestID,
				EvaluationResult: EvaluationResult{
					Decision:         DecisionAllow,
					Reason:           "No matching rules",
					MatchedRuleIDs:   []uuid.UUID{},
					EvaluatedRuleIDs: []uuid.UUID{},
				},
				LimitUsageDetails: []LimitUsageDetail{},
				ProcessingTimeMs:  42,
				CreatedAt:         fixedCreatedAt,
			},
			wantResp: &ValidationResponse{
				ValidationID: validationID,
				RequestID:    requestID,
				EvaluationResult: EvaluationResult{
					Decision:         DecisionAllow,
					Reason:           "No matching rules",
					MatchedRuleIDs:   []uuid.UUID{},
					EvaluatedRuleIDs: []uuid.UUID{},
				},
				LimitUsageDetails: []LimitUsageDetail{},
				ProcessingTimeMs:  42,
				EvaluatedAt:       fixedCreatedAt,
			},
		},
		{
			name: "converts entity with nil slices to empty slices",
			validation: &TransactionValidation{
				ID:        validationID,
				RequestID: requestID,
				EvaluationResult: EvaluationResult{
					Decision:         DecisionAllow,
					Reason:           "No rules configured",
					MatchedRuleIDs:   nil,
					EvaluatedRuleIDs: nil,
				},
				LimitUsageDetails: nil,
				ProcessingTimeMs:  10,
				CreatedAt:         fixedCreatedAt,
			},
			wantResp: &ValidationResponse{
				ValidationID: validationID,
				RequestID:    requestID,
				EvaluationResult: EvaluationResult{
					Decision:         DecisionAllow,
					Reason:           "No rules configured",
					MatchedRuleIDs:   []uuid.UUID{},
					EvaluatedRuleIDs: []uuid.UUID{},
				},
				LimitUsageDetails: []LimitUsageDetail{},
				ProcessingTimeMs:  10,
				EvaluatedAt:       fixedCreatedAt,
			},
		},
		{
			name: "converts full entity with all fields to response",
			validation: &TransactionValidation{
				ID:                   validationID,
				RequestID:            requestID,
				TransactionType:      TransactionTypeCard,
				Amount:               decimal.RequireFromString("500.00"),
				Currency:             "USD",
				TransactionTimestamp: fixedTxTimestamp,
				Account: AccountContext{
					ID:     accountID,
					Type:   "checking",
					Status: "active",
				},
				EvaluationResult: EvaluationResult{
					Decision:         DecisionDeny,
					Reason:           "High risk transaction",
					MatchedRuleIDs:   []uuid.UUID{matchedRuleID},
					EvaluatedRuleIDs: []uuid.UUID{evaluatedRuleID},
				},
				LimitUsageDetails: []LimitUsageDetail{
					{
						LimitID:      limitID,
						LimitAmount:  decimal.RequireFromString("1000.00"),
						CurrentUsage: decimal.RequireFromString("950.00"),
						Exceeded:     false,
					},
				},
				ProcessingTimeMs: 85,
				CreatedAt:        fixedCreatedAt,
			},
			wantResp: &ValidationResponse{
				ValidationID: validationID,
				RequestID:    requestID,
				EvaluationResult: EvaluationResult{
					Decision:         DecisionDeny,
					Reason:           "High risk transaction",
					MatchedRuleIDs:   []uuid.UUID{matchedRuleID},
					EvaluatedRuleIDs: []uuid.UUID{evaluatedRuleID},
				},
				LimitUsageDetails: []LimitUsageDetail{
					{
						LimitID:      limitID,
						LimitAmount:  decimal.RequireFromString("1000.00"),
						CurrentUsage: decimal.RequireFromString("950.00"),
						Exceeded:     false,
					},
				},
				ProcessingTimeMs: 85,
				EvaluatedAt:      fixedCreatedAt,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := tc.validation.ToValidationResponse()

			require.NotNil(t, result)
			assert.Equal(t, tc.wantResp.ValidationID, result.ValidationID)
			assert.Equal(t, tc.wantResp.RequestID, result.RequestID)
			assert.Equal(t, tc.wantResp.Decision, result.Decision)
			assert.Equal(t, tc.wantResp.Reason, result.Reason)
			assert.Equal(t, tc.wantResp.ProcessingTimeMs, result.ProcessingTimeMs)
			assert.Equal(t, tc.wantResp.EvaluatedAt, result.EvaluatedAt)

			// Verify arrays match - length
			require.Len(t, result.MatchedRuleIDs, len(tc.wantResp.MatchedRuleIDs))
			require.Len(t, result.EvaluatedRuleIDs, len(tc.wantResp.EvaluatedRuleIDs))
			require.Len(t, result.LimitUsageDetails, len(tc.wantResp.LimitUsageDetails))

			// Verify arrays match - content
			require.Equal(t, tc.wantResp.MatchedRuleIDs, result.MatchedRuleIDs, "MatchedRuleIDs content mismatch")
			require.Equal(t, tc.wantResp.EvaluatedRuleIDs, result.EvaluatedRuleIDs, "EvaluatedRuleIDs content mismatch")
			require.Equal(t, tc.wantResp.LimitUsageDetails, result.LimitUsageDetails, "LimitUsageDetails content mismatch")
		})
	}
}

// TestTransactionValidation_ToValidationResponse_DefensiveCopy verifies that mutating the
// response does not affect the original entity. Without this test, the defensive copy logic
// in ToValidationResponse could be removed and all other tests would still pass.
func TestTransactionValidation_ToValidationResponse_DefensiveCopy(t *testing.T) {
	t.Parallel()

	originalRuleID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440010")
	originalLimitID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440030")

	tv := &TransactionValidation{
		ID:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
		RequestID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440100"),
		EvaluationResult: EvaluationResult{
			Decision:         DecisionDeny,
			Reason:           "High risk",
			MatchedRuleIDs:   []uuid.UUID{originalRuleID},
			EvaluatedRuleIDs: []uuid.UUID{originalRuleID},
		},
		LimitUsageDetails: []LimitUsageDetail{
			{
				LimitID:      originalLimitID,
				LimitAmount:  decimal.RequireFromString("1000.00"),
				CurrentUsage: decimal.RequireFromString("500.00"),
				Exceeded:     false,
			},
		},
		ProcessingTimeMs: 50,
		CreatedAt:        time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	resp := tv.ToValidationResponse()

	// Mutate all response slices
	resp.MatchedRuleIDs[0] = uuid.Nil
	resp.EvaluatedRuleIDs[0] = uuid.Nil
	resp.LimitUsageDetails[0].Exceeded = true

	// Original entity must remain unchanged
	assert.Equal(t, originalRuleID, tv.MatchedRuleIDs[0], "original MatchedRuleIDs mutated via response")
	assert.Equal(t, originalRuleID, tv.EvaluatedRuleIDs[0], "original EvaluatedRuleIDs mutated via response")
	assert.False(t, tv.LimitUsageDetails[0].Exceeded, "original LimitUsageDetails mutated via response")
}

// TestTransactionValidation_ToValidationResponse_DefensiveCopy_Scopes verifies that
// mutating the nested Scopes slice in the response does not affect the original entity.
func TestTransactionValidation_ToValidationResponse_DefensiveCopy_Scopes(t *testing.T) {
	t.Parallel()

	originalAccountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	originalLimitID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440030")

	tv := &TransactionValidation{
		ID:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440010"),
		RequestID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440020"),
		EvaluationResult: EvaluationResult{
			Decision: DecisionAllow,
		},
		LimitUsageDetails: []LimitUsageDetail{
			{
				LimitID: originalLimitID,
				Scopes: []Scope{
					{AccountID: &originalAccountID},
				},
			},
		},
		CreatedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	resp := tv.ToValidationResponse()

	// Mutate response's Scopes slice (append)
	resp.LimitUsageDetails[0].Scopes = append(resp.LimitUsageDetails[0].Scopes, Scope{})

	// Mutate the pointee (not the pointer) to detect shared pointer targets
	require.NotNil(t, resp.LimitUsageDetails[0].Scopes[0].AccountID,
		"copied AccountID pointer should not be nil")
	*resp.LimitUsageDetails[0].Scopes[0].AccountID = uuid.MustParse("99999999-9999-9999-9999-999999999999")

	// Original must remain unchanged
	require.Len(t, tv.LimitUsageDetails[0].Scopes, 1,
		"original Scopes length mutated via response append")
	assert.Equal(t, originalAccountID, *tv.LimitUsageDetails[0].Scopes[0].AccountID,
		"original Scopes content mutated via response")
}

// TestTransactionValidation_ToValidationResponse_NilReceiver verifies nil-safety.
// This is critical because FindByRequestID returns (nil, nil) for not-found cases,
// and callers might chain .ToValidationResponse() on the result.
func TestTransactionValidation_ToValidationResponse_NilReceiver(t *testing.T) {
	t.Parallel()

	var nilValidation *TransactionValidation = nil

	result := nilValidation.ToValidationResponse()

	assert.Nil(t, result, "ToValidationResponse on nil receiver should return nil")
}
