// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestDecision_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		decision Decision
		want     bool
	}{
		{
			name:     "ALLOW is valid",
			decision: DecisionAllow,
			want:     true,
		},
		{
			name:     "DENY is valid",
			decision: DecisionDeny,
			want:     true,
		},
		{
			name:     "REVIEW is valid",
			decision: DecisionReview,
			want:     true,
		},
		{
			name:     "empty string is invalid",
			decision: Decision(""),
			want:     false,
		},
		{
			name:     "lowercase allow is invalid",
			decision: Decision("allow"),
			want:     false,
		},
		{
			name:     "random string is invalid",
			decision: Decision("INVALID"),
			want:     false,
		},
		{
			name:     "partial match is invalid",
			decision: Decision("ALLO"),
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.decision.IsValid()
			assert.Equal(t, tc.want, result)
		})
	}
}

func TestNewValidationResponse(t *testing.T) {
	tests := []struct {
		name         string
		validationID uuid.UUID
		requestID    uuid.UUID
		decision     Decision
	}{
		{
			name:         "creates response with ALLOW decision",
			validationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			requestID:    uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			decision:     DecisionAllow,
		},
		{
			name:         "creates response with DENY decision",
			validationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440010"),
			requestID:    uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"),
			decision:     DecisionDeny,
		},
		{
			name:         "creates response with REVIEW decision",
			validationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440020"),
			requestID:    uuid.MustParse("550e8400-e29b-41d4-a716-446655440003"),
			decision:     DecisionReview,
		},
		{
			name:         "creates response with invalid decision (documents current behavior)",
			validationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440030"),
			requestID:    uuid.MustParse("550e8400-e29b-41d4-a716-446655440004"),
			decision:     Decision("INVALID"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evaluatedAt := testutil.FixedTime()
			result := NewValidationResponse(tc.validationID, tc.requestID, tc.decision, evaluatedAt)

			require.NotNil(t, result)
			assert.Equal(t, tc.validationID, result.ValidationID)
			assert.Equal(t, tc.requestID, result.RequestID)
			assert.Equal(t, tc.decision, result.Decision)
			assert.Equal(t, evaluatedAt, result.EvaluatedAt, "EvaluatedAt should be set by constructor")

			// Verify slices are initialized (not nil) for proper JSON serialization
			assert.NotNil(t, result.MatchedRuleIDs, "MatchedRuleIDs should be initialized")
			assert.NotNil(t, result.EvaluatedRuleIDs, "EvaluatedRuleIDs should be initialized")
			assert.NotNil(t, result.LimitUsageDetails, "LimitUsageDetails should be initialized")

			// Verify slices are empty
			assert.Empty(t, result.MatchedRuleIDs)
			assert.Empty(t, result.EvaluatedRuleIDs)
			assert.Empty(t, result.LimitUsageDetails)

			// Verify optional fields are zero values
			assert.Empty(t, result.Reason)
			assert.Zero(t, result.ProcessingTimeMs)
		})
	}
}

func TestDecision_String(t *testing.T) {
	tests := []struct {
		name     string
		decision Decision
		want     string
	}{
		{
			name:     "ALLOW returns ALLOW string",
			decision: DecisionAllow,
			want:     "ALLOW",
		},
		{
			name:     "DENY returns DENY string",
			decision: DecisionDeny,
			want:     "DENY",
		},
		{
			name:     "REVIEW returns REVIEW string",
			decision: DecisionReview,
			want:     "REVIEW",
		},
		{
			name:     "invalid value returns raw string",
			decision: Decision("INVALID"),
			want:     "INVALID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.decision.String())
		})
	}
}

func TestValidationRequest_ToTransactionScope(t *testing.T) {
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	segmentID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	portfolioID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	merchantID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440004")
	txTypeCard := TransactionTypeCard
	txTypePix := TransactionTypePix
	subType := "debit"

	tests := []struct {
		name    string
		input   ValidationRequest
		want    *Scope
		wantErr bool
	}{
		{
			name: "minimal request with account only",
			input: ValidationRequest{
				TransactionType: TransactionTypePix,
				Account:         AccountContext{ID: accountID},
			},
			want: &Scope{
				AccountID:       &accountID,
				TransactionType: &txTypePix,
			},
		},
		{
			name: "request with all optional fields",
			input: ValidationRequest{
				TransactionType: txTypeCard,
				SubType:         &subType,
				Account:         AccountContext{ID: accountID},
				Segment:         &SegmentContext{ID: segmentID},
				Portfolio:       &PortfolioContext{ID: portfolioID},
				Merchant:        &MerchantContext{ID: merchantID},
			},
			want: &Scope{
				AccountID:       &accountID,
				SegmentID:       &segmentID,
				PortfolioID:     &portfolioID,
				MerchantID:      &merchantID,
				TransactionType: &txTypeCard,
				SubType:         &subType,
			},
		},
		{
			name: "request with segment only",
			input: ValidationRequest{
				TransactionType: txTypeCard,
				Account:         AccountContext{ID: accountID},
				Segment:         &SegmentContext{ID: segmentID},
			},
			want: &Scope{
				AccountID:       &accountID,
				SegmentID:       &segmentID,
				TransactionType: &txTypeCard,
			},
		},
		{
			name: "request with portfolio only",
			input: ValidationRequest{
				TransactionType: txTypeCard,
				Account:         AccountContext{ID: accountID},
				Portfolio:       &PortfolioContext{ID: portfolioID},
			},
			want: &Scope{
				AccountID:       &accountID,
				PortfolioID:     &portfolioID,
				TransactionType: &txTypeCard,
			},
		},
		{
			name: "request with merchant only",
			input: ValidationRequest{
				TransactionType: txTypeCard,
				Account:         AccountContext{ID: accountID},
				Merchant:        &MerchantContext{ID: merchantID},
			},
			want: &Scope{
				AccountID:       &accountID,
				MerchantID:      &merchantID,
				TransactionType: &txTypeCard,
			},
		},
		{
			name: "request with subtype",
			input: ValidationRequest{
				TransactionType: txTypeCard,
				SubType:         &subType,
				Account:         AccountContext{ID: accountID},
			},
			want: &Scope{
				AccountID:       &accountID,
				TransactionType: &txTypeCard,
				SubType:         &subType,
			},
		},
		{
			name: "request with zero account ID",
			input: ValidationRequest{
				TransactionType: TransactionTypePix,
				Account:         AccountContext{ID: uuid.UUID{}},
			},
			want: &Scope{
				AccountID:       func() *uuid.UUID { id := uuid.UUID{}; return &id }(),
				TransactionType: &txTypePix,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.input.ToTransactionScope()

			require.NotNil(t, result, "ToTransactionScope should not return nil")
			assert.Equal(t, tc.want.AccountID, result.AccountID, "AccountID mismatch")
			assert.Equal(t, tc.want.SegmentID, result.SegmentID, "SegmentID mismatch")
			assert.Equal(t, tc.want.PortfolioID, result.PortfolioID, "PortfolioID mismatch")
			assert.Equal(t, tc.want.MerchantID, result.MerchantID, "MerchantID mismatch")
			assert.Equal(t, tc.want.TransactionType, result.TransactionType, "TransactionType mismatch")
			assert.Equal(t, tc.want.SubType, result.SubType, "SubType mismatch")
		})
	}
}

func TestNormalizeAndValidate_Atomicity(t *testing.T) {
	// Test that failed validation does NOT mutate the original request (atomicity guarantee)
	t.Run("failed validation does not mutate receiver", func(t *testing.T) {
		subType := "  payment  " // with whitespace to normalize
		originalMetadata := map[string]any{"key1": "value1"}

		req := ValidationRequest{
			RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			TransactionType:      TransactionTypeCard,
			SubType:              &subType,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "invalid", // lowercase - will fail validation
			TransactionTimestamp: testutil.FixedTime(),
			Account:              AccountContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")},
			Metadata:             originalMetadata,
		}

		// Capture original values before call
		originalSubType := *req.SubType
		originalSubTypePtr := req.SubType
		originalCurrency := req.Currency

		// Call NormalizeAndValidate - should fail due to invalid currency
		err := req.NormalizeAndValidate(testutil.FixedTime())

		// Verify validation failed
		assert.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrValidationInvalidCurrency)

		// Verify receiver was NOT mutated (atomicity)
		require.NotNil(t, req.SubType, "SubType should not be nil")
		assert.Equal(t, originalSubType, *req.SubType, "SubType should be unchanged after failed validation")
		assert.Same(t, originalSubTypePtr, req.SubType, "SubType pointer should be unchanged (same reference)")
		assert.Equal(t, originalCurrency, req.Currency, "Currency should be unchanged after failed validation")

		// Verify original metadata map still assigned (not replaced)
		// Add a marker to original to verify it's the same map instance
		originalMetadata["_marker"] = "test"
		assert.Equal(t, "test", req.Metadata["_marker"], "Metadata should still be the same map instance")
	})

	t.Run("successful validation applies normalizations", func(t *testing.T) {
		subType := "  payment  " // with whitespace to normalize
		originalMetadata := map[string]any{"key1": "value1"}

		req := ValidationRequest{
			RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			TransactionType:      TransactionTypeCard,
			SubType:              &subType,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "USD", // valid uppercase
			TransactionTimestamp: testutil.FixedTime(),
			Account:              AccountContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")},
			Metadata:             originalMetadata,
		}

		// Capture original subtype pointer
		originalSubTypePtr := req.SubType

		// Call NormalizeAndValidate - should succeed
		err := req.NormalizeAndValidate(testutil.FixedTime())

		// Verify validation succeeded
		require.NoError(t, err)

		// Verify normalizations were applied
		require.NotNil(t, req.SubType, "SubType should not be nil")
		assert.Equal(t, "payment", *req.SubType, "SubType should be trimmed")
		assert.NotSame(t, originalSubTypePtr, req.SubType, "SubType pointer should be different (new allocation)")

		// Verify metadata was shallow-copied (detached from original)
		originalMetadata["_marker"] = "test"
		_, hasMarker := req.Metadata["_marker"]
		assert.False(t, hasMarker, "Metadata should be detached (shallow copy), marker should not appear")
		assert.Equal(t, "value1", req.Metadata["key1"], "Metadata values should be preserved")
	})

	t.Run("nil subtype and metadata remain unchanged on failure", func(t *testing.T) {
		req := ValidationRequest{
			RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			TransactionType:      TransactionTypeCard,
			SubType:              nil, // no subtype
			Amount:               decimal.RequireFromString("10"),
			Currency:             "invalid", // will fail
			TransactionTimestamp: testutil.FixedTime(),
			Account:              AccountContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")},
			Metadata:             nil, // no metadata
		}

		// Capture original currency before call
		originalCurrency := req.Currency

		err := req.NormalizeAndValidate(testutil.FixedTime())

		assert.Error(t, err)
		assert.Nil(t, req.SubType, "SubType should remain nil")
		assert.Nil(t, req.Metadata, "Metadata should remain nil")
		assert.Equal(t, originalCurrency, req.Currency, "Currency should be unchanged after failed validation")
	})
}

func TestValidationRequest_Validate_MerchantID(t *testing.T) {
	validAccountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	validMerchantID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	validRequestID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	fixedTimestamp := testutil.FixedTime()

	baseRequest := func() ValidationRequest {
		return ValidationRequest{
			RequestID:            validRequestID,
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "BRL",
			TransactionTimestamp: fixedTimestamp,
			Account:              AccountContext{ID: validAccountID},
		}
	}

	tests := []struct {
		name    string
		modify  func(*ValidationRequest)
		wantErr error
	}{
		{
			name:    "valid request without merchant",
			modify:  func(r *ValidationRequest) {},
			wantErr: nil,
		},
		{
			name: "valid request with merchant ID",
			modify: func(r *ValidationRequest) {
				r.Merchant = &MerchantContext{ID: validMerchantID, Category: "5411", Country: "BR"}
			},
			wantErr: nil,
		},
		{
			name: "invalid - merchant provided with nil ID",
			modify: func(r *ValidationRequest) {
				r.Merchant = &MerchantContext{ID: uuid.Nil, Category: "5411", Country: "BR"}
			},
			wantErr: constant.ErrValidationMerchantIDRequired,
		},
		{
			name: "invalid - merchant provided with zero UUID",
			modify: func(r *ValidationRequest) {
				r.Merchant = &MerchantContext{Category: "5411"}
			},
			wantErr: constant.ErrValidationMerchantIDRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := baseRequest()
			tc.modify(&req)

			err := req.Validate(fixedTimestamp)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNormalizeAndValidate_NestedMetadataDefensiveCopy(t *testing.T) {
	t.Run("nested context metadata are defensively copied", func(t *testing.T) {
		// Create original metadata maps for nested contexts
		segmentMeta := map[string]any{"segment_key": "segment_value"}
		portfolioMeta := map[string]any{"portfolio_key": "portfolio_value"}
		merchantMeta := map[string]any{"merchant_key": "merchant_value"}

		req := ValidationRequest{
			RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account:              AccountContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")},
			Segment: &SegmentContext{
				ID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440003"),
				Name:     "VIP",
				Metadata: segmentMeta,
			},
			Portfolio: &PortfolioContext{
				ID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440004"),
				Name:     "Premium",
				Metadata: portfolioMeta,
			},
			Merchant: &MerchantContext{
				ID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440005"),
				Name:     "Amazon",
				Category: "5411",
				Country:  "US",
				Metadata: merchantMeta,
			},
		}

		// Call NormalizeAndValidate
		err := req.NormalizeAndValidate(testutil.FixedTime())
		require.NoError(t, err)

		// Mutate original metadata maps
		segmentMeta["_marker"] = "mutated"
		portfolioMeta["_marker"] = "mutated"
		merchantMeta["_marker"] = "mutated"

		// Verify request's nested metadata are detached (defensive copies)
		_, hasSegmentMarker := req.Segment.Metadata["_marker"]
		assert.False(t, hasSegmentMarker, "Segment metadata should be detached, marker should not appear")
		assert.Equal(t, "segment_value", req.Segment.Metadata["segment_key"], "Segment metadata values should be preserved")

		_, hasPortfolioMarker := req.Portfolio.Metadata["_marker"]
		assert.False(t, hasPortfolioMarker, "Portfolio metadata should be detached, marker should not appear")
		assert.Equal(t, "portfolio_value", req.Portfolio.Metadata["portfolio_key"], "Portfolio metadata values should be preserved")

		_, hasMerchantMarker := req.Merchant.Metadata["_marker"]
		assert.False(t, hasMerchantMarker, "Merchant metadata should be detached, marker should not appear")
		assert.Equal(t, "merchant_value", req.Merchant.Metadata["merchant_key"], "Merchant metadata values should be preserved")
	})

	t.Run("nil nested contexts remain unchanged", func(t *testing.T) {
		req := ValidationRequest{
			RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account:              AccountContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")},
			Segment:              nil,
			Portfolio:            nil,
			Merchant:             nil,
		}

		err := req.NormalizeAndValidate(testutil.FixedTime())
		require.NoError(t, err)

		// Verify nil contexts remain nil
		assert.Nil(t, req.Segment)
		assert.Nil(t, req.Portfolio)
		assert.Nil(t, req.Merchant)
	})

	t.Run("nested contexts with nil metadata remain unchanged", func(t *testing.T) {
		req := ValidationRequest{
			RequestID:            uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account:              AccountContext{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")},
			Segment: &SegmentContext{
				ID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440003"),
				Name:     "VIP",
				Metadata: nil,
			},
			Portfolio: &PortfolioContext{
				ID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440004"),
				Name:     "Premium",
				Metadata: nil,
			},
			Merchant: &MerchantContext{
				ID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440005"),
				Name:     "Amazon",
				Category: "5411",
				Country:  "US",
				Metadata: nil,
			},
		}

		err := req.NormalizeAndValidate(testutil.FixedTime())
		require.NoError(t, err)

		// Verify contexts exist but metadata remain nil
		require.NotNil(t, req.Segment)
		assert.Nil(t, req.Segment.Metadata)

		require.NotNil(t, req.Portfolio)
		assert.Nil(t, req.Portfolio.Metadata)

		require.NotNil(t, req.Merchant)
		assert.Nil(t, req.Merchant.Metadata)
	})
}

func TestNewValidationRequest_DefensiveCopyContextMetadata(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()

	// Create contexts with metadata that we'll try to mutate
	segmentMeta := map[string]any{"seg_key": "seg_value"}
	portfolioMeta := map[string]any{"port_key": "port_value"}
	merchantMeta := map[string]any{"merch_key": "merch_value"}

	segment := &SegmentContext{
		ID:       testutil.MustDeterministicUUID(1),
		Name:     "Test Segment",
		Metadata: segmentMeta,
	}

	portfolio := &PortfolioContext{
		ID:       testutil.MustDeterministicUUID(2),
		Name:     "Test Portfolio",
		Metadata: portfolioMeta,
	}

	merchant := &MerchantContext{
		ID:       testutil.MustDeterministicUUID(3),
		Name:     "Test Merchant",
		Category: "5411", // 4-digit MCC code (Grocery Stores)
		Country:  "US",
		Metadata: merchantMeta,
	}

	// Create request
	req, err := NewValidationRequest(
		fixedTime,
		testutil.MustDeterministicUUID(10),
		TransactionTypeCard,
		nil,
		decimal.RequireFromString("10"),
		"USD",
		fixedTime,
		AccountContext{ID: testutil.MustDeterministicUUID(4)},
		segment,
		portfolio,
		merchant,
		nil,
	)
	require.NoError(t, err)

	// Mutate original context metadata maps
	segmentMeta["seg_key"] = "MUTATED"
	segmentMeta["new_key"] = "NEW_VALUE"

	portfolioMeta["port_key"] = "MUTATED"
	portfolioMeta["new_key"] = "NEW_VALUE"

	merchantMeta["merch_key"] = "MUTATED"
	merchantMeta["new_key"] = "NEW_VALUE"

	// Verify request contexts are unaffected (defensive copy worked)
	assert.Equal(t, "seg_value", req.Segment.Metadata["seg_key"], "Segment metadata should not be affected by external mutation")
	assert.NotContains(t, req.Segment.Metadata, "new_key", "Segment metadata should not have new keys from external map")

	assert.Equal(t, "port_value", req.Portfolio.Metadata["port_key"], "Portfolio metadata should not be affected by external mutation")
	assert.NotContains(t, req.Portfolio.Metadata, "new_key", "Portfolio metadata should not have new keys from external map")

	assert.Equal(t, "merch_value", req.Merchant.Metadata["merch_key"], "Merchant metadata should not be affected by external mutation")
	assert.NotContains(t, req.Merchant.Metadata, "new_key", "Merchant metadata should not have new keys from external map")
}

func TestValidationRequest_Validate_PastTimestamp(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// validRequest builds a valid request using deterministic UUIDs from seed range 8100-8109.
	// The timestamp is set to now so it passes all existing checks.
	validRequest := func() *ValidationRequest {
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(8100),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("250.00"),
			Currency:             "USD",
			TransactionTimestamp: now, // will be overridden per test case
			Account: AccountContext{
				ID:     testutil.MustDeterministicUUID(8101),
				Type:   "checking",
				Status: "active",
			},
		}
	}

	tests := []struct {
		name        string
		timestamp   time.Time
		expectedErr error
	}{
		{
			name:        "timestamp 25 hours ago is rejected as too old",
			timestamp:   now.Add(-25 * time.Hour),
			expectedErr: constant.ErrValidationTimestampPast,
		},
		{
			name:        "timestamp 48 hours ago is rejected as too old",
			timestamp:   now.Add(-48 * time.Hour),
			expectedErr: constant.ErrValidationTimestampPast,
		},
		{
			name:        "timestamp 7 days ago is rejected as too old",
			timestamp:   now.Add(-7 * 24 * time.Hour),
			expectedErr: constant.ErrValidationTimestampPast,
		},
		{
			name:        "timestamp 23 hours ago is accepted",
			timestamp:   now.Add(-23 * time.Hour),
			expectedErr: nil,
		},
		{
			name:        "timestamp 1 hour ago is accepted",
			timestamp:   now.Add(-1 * time.Hour),
			expectedErr: nil,
		},
		{
			name:        "timestamp 1 second ago is accepted",
			timestamp:   now.Add(-1 * time.Second),
			expectedErr: nil,
		},
		{
			name:        "current timestamp is accepted",
			timestamp:   now,
			expectedErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := validRequest()
			req.TransactionTimestamp = tc.timestamp

			err := req.Validate(now)

			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.expectedErr)
			}
		})
	}
}

func TestValidationRequest_Validate_PastTimestamp_Boundary(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// validRequest builds a valid request using deterministic UUIDs from seed range 8110-8119.
	validRequest := func() *ValidationRequest {
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(8110),
			TransactionType:      TransactionTypePix,
			Amount:               decimal.RequireFromString("500.00"),
			Currency:             "BRL",
			TransactionTimestamp: now,
			Account: AccountContext{
				ID:     testutil.MustDeterministicUUID(8111),
				Type:   "savings",
				Status: "active",
			},
		}
	}

	tests := []struct {
		name        string
		timestamp   time.Time
		expectedErr error
	}{
		{
			name:        "exactly 24h ago is rejected (boundary inclusive)",
			timestamp:   now.Add(-24 * time.Hour),
			expectedErr: constant.ErrValidationTimestampPast,
		},
		{
			name:        "24h minus 1 second ago is accepted (just inside window)",
			timestamp:   now.Add(-24*time.Hour + 1*time.Second),
			expectedErr: nil,
		},
		{
			name:        "24h plus 1 second ago is rejected (just outside window)",
			timestamp:   now.Add(-24*time.Hour - 1*time.Second),
			expectedErr: constant.ErrValidationTimestampPast,
		},
		{
			name:        "24h minus 100 milliseconds ago is accepted",
			timestamp:   now.Add(-24*time.Hour + 100*time.Millisecond),
			expectedErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest()
			req.TransactionTimestamp = tc.timestamp

			err := req.Validate(now)

			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.expectedErr)
			}
		})
	}
}

func TestValidationRequest_Validate_PastTimestamp_CustomMaxAge(t *testing.T) {
	// No t.Parallel() — this test mutates the global MaxTimestampAge variable
	original := MaxTimestampAge
	MaxTimestampAge = 1 * time.Hour

	defer func() { MaxTimestampAge = original }()

	now := time.Now()

	// validRequest builds a valid request using deterministic UUIDs from seed range 8120-8129.
	validRequest := func() *ValidationRequest {
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(8120),
			TransactionType:      TransactionTypeWire,
			Amount:               decimal.RequireFromString("100.00"),
			Currency:             "EUR",
			TransactionTimestamp: now,
			Account: AccountContext{
				ID:     testutil.MustDeterministicUUID(8121),
				Type:   "checking",
				Status: "active",
			},
		}
	}

	tests := []struct {
		name        string
		timestamp   time.Time
		expectedErr error
	}{
		{
			name:        "2h ago rejected with 1h max age",
			timestamp:   now.Add(-2 * time.Hour),
			expectedErr: constant.ErrValidationTimestampPast,
		},
		{
			name:        "30min ago accepted with 1h max age",
			timestamp:   now.Add(-30 * time.Minute),
			expectedErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest()
			req.TransactionTimestamp = tc.timestamp

			err := req.Validate(now)

			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.expectedErr)
			}
		})
	}
}
