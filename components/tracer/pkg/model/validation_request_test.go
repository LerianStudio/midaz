// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"time"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationRequest_Validate(t *testing.T) {
	validRequest := func() *ValidationRequest {
		accountID := testutil.MustDeterministicUUID(1)
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(2),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("100"), // $100.00
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID:     accountID,
				Type:   "checking",
				Status: "active",
			},
		}
	}

	tests := []struct {
		name        string
		modify      func(*ValidationRequest)
		expectedErr error
	}{
		{
			name:        "valid request passes validation",
			modify:      func(r *ValidationRequest) {},
			expectedErr: nil,
		},
		{
			name: "missing requestId fails",
			modify: func(r *ValidationRequest) {
				r.RequestID = uuid.Nil
			},
			expectedErr: constant.ErrValidationRequestIDRequired,
		},
		{
			name: "invalid transactionType fails",
			modify: func(r *ValidationRequest) {
				r.TransactionType = TransactionType("INVALID")
			},
			expectedErr: constant.ErrValidationInvalidTransactionType,
		},
		{
			name: "empty transactionType fails",
			modify: func(r *ValidationRequest) {
				r.TransactionType = TransactionType("")
			},
			expectedErr: constant.ErrValidationInvalidTransactionType,
		},
		{
			name: "zero amount fails",
			modify: func(r *ValidationRequest) {
				r.Amount = decimal.RequireFromString("0")
			},
			expectedErr: constant.ErrValidationAmountNonPositive,
		},
		{
			name: "negative amount fails",
			modify: func(r *ValidationRequest) {
				r.Amount = decimal.RequireFromString("-1")
			},
			expectedErr: constant.ErrValidationAmountNonPositive,
		},
		{
			name: "empty currency fails",
			modify: func(r *ValidationRequest) {
				r.Currency = ""
			},
			expectedErr: constant.ErrValidationCurrencyRequired,
		},
		{
			name: "invalid currency format fails",
			modify: func(r *ValidationRequest) {
				r.Currency = "INVALID"
			},
			expectedErr: constant.ErrValidationInvalidCurrency,
		},
		{
			name: "too short currency fails",
			modify: func(r *ValidationRequest) {
				r.Currency = "US"
			},
			expectedErr: constant.ErrValidationInvalidCurrency,
		},
		{
			name: "too long currency fails",
			modify: func(r *ValidationRequest) {
				r.Currency = "USDD"
			},
			expectedErr: constant.ErrValidationInvalidCurrency,
		},
		{
			name: "zero timestamp fails",
			modify: func(r *ValidationRequest) {
				r.TransactionTimestamp = time.Time{}
			},
			expectedErr: constant.ErrValidationTimestampRequired,
		},
		{
			name: "future timestamp fails",
			modify: func(r *ValidationRequest) {
				// Set timestamp 2 minutes in the future (beyond 1 minute clock skew allowance)
				r.TransactionTimestamp = testutil.FixedTime().Add(2 * time.Minute)
			},
			expectedErr: constant.ErrValidationTimestampFuture,
		},
		{
			name: "timestamp within clock skew tolerance passes",
			modify: func(r *ValidationRequest) {
				// Set timestamp 30 seconds in the future (within 1 minute clock skew allowance)
				r.TransactionTimestamp = testutil.FixedTime().Add(30 * time.Second)
			},
			expectedErr: nil,
		},
		{
			name: "missing account ID fails",
			modify: func(r *ValidationRequest) {
				r.Account.ID = uuid.Nil
			},
			expectedErr: constant.ErrValidationAccountRequired,
		},
		{
			name: "segment with nil ID fails",
			modify: func(r *ValidationRequest) {
				r.Segment = &SegmentContext{ID: uuid.Nil}
			},
			expectedErr: constant.ErrValidationSegmentIDRequired,
		},
		{
			name: "portfolio with nil ID fails",
			modify: func(r *ValidationRequest) {
				r.Portfolio = &PortfolioContext{ID: uuid.Nil}
			},
			expectedErr: constant.ErrValidationPortfolioIDRequired,
		},
		{
			name: "valid segment passes",
			modify: func(r *ValidationRequest) {
				r.Segment = &SegmentContext{ID: testutil.MustDeterministicUUID(3), Name: "retail"}
			},
			expectedErr: nil,
		},
		{
			name: "valid portfolio passes",
			modify: func(r *ValidationRequest) {
				r.Portfolio = &PortfolioContext{ID: testutil.MustDeterministicUUID(4), Name: "premium"}
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			tt.modify(req)

			err := req.Validate(testutil.FixedTime())

			if tt.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.expectedErr)
			}
		})
	}
}

// TestValidationRequest_ValidateForReserve locks the reserve-path relaxation:
// transactionType and account are OPTIONAL on reserve (the ledger has no card
// rail and may reserve for an external-only source), while requestId, amount,
// currency and the timestamp window stay mandatory. This is the contract that
// closed the F3 enforce gap; tightening it back re-breaks the ledger reserve.
func TestValidationRequest_ValidateForReserve(t *testing.T) {
	validRequest := func() *ValidationRequest {
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(2),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("100"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: testutil.MustDeterministicUUID(1),
			},
		}
	}

	tests := []struct {
		name        string
		modify      func(*ValidationRequest)
		expectedErr error
	}{
		{
			name:        "fully-populated ledger-style request passes",
			modify:      func(r *ValidationRequest) {},
			expectedErr: nil,
		},
		{
			name: "empty transactionType is ACCEPTED on reserve (relaxed)",
			modify: func(r *ValidationRequest) {
				r.TransactionType = ""
			},
			expectedErr: nil,
		},
		{
			name: "missing account is ACCEPTED on reserve (relaxed)",
			modify: func(r *ValidationRequest) {
				r.Account = AccountContext{}
			},
			expectedErr: nil,
		},
		{
			name: "ledger-shaped request (no account, no transactionType) passes",
			modify: func(r *ValidationRequest) {
				r.Account = AccountContext{}
				r.TransactionType = ""
			},
			expectedErr: nil,
		},
		{
			name: "INVALID (non-empty, non-enum) transactionType still fails",
			modify: func(r *ValidationRequest) {
				r.TransactionType = TransactionType("PIXIE")
			},
			expectedErr: constant.ErrValidationInvalidTransactionType,
		},
		{
			name: "missing requestId still fails",
			modify: func(r *ValidationRequest) {
				r.RequestID = uuid.Nil
			},
			expectedErr: constant.ErrValidationRequestIDRequired,
		},
		{
			name: "zero amount still fails",
			modify: func(r *ValidationRequest) {
				r.Amount = decimal.RequireFromString("0")
			},
			expectedErr: constant.ErrValidationAmountNonPositive,
		},
		{
			name: "invalid currency still fails",
			modify: func(r *ValidationRequest) {
				r.Currency = "usd"
			},
			expectedErr: constant.ErrValidationInvalidCurrency,
		},
		{
			name: "zero timestamp still fails",
			modify: func(r *ValidationRequest) {
				r.TransactionTimestamp = time.Time{}
			},
			expectedErr: constant.ErrValidationTimestampRequired,
		},
		{
			name: "future timestamp still fails",
			modify: func(r *ValidationRequest) {
				r.TransactionTimestamp = testutil.FixedTime().Add(2 * time.Minute)
			},
			expectedErr: constant.ErrValidationTimestampFuture,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			tt.modify(req)

			err := req.ValidateForReserve(testutil.FixedTime())

			if tt.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.expectedErr)
			}
		})
	}
}

func TestValidationRequest_ToCheckLimitsInput(t *testing.T) {
	t.Run("converts required fields correctly", func(t *testing.T) {
		subType := "Credit"
		accountID := testutil.MustDeterministicUUID(30)
		segmentID := testutil.MustDeterministicUUID(31)
		portfolioID := testutil.MustDeterministicUUID(32)
		timestamp := testutil.FixedTime()
		req := &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(33),
			TransactionType:      TransactionTypeCard,
			SubType:              &subType,
			Amount:               decimal.RequireFromString("500"),
			Currency:             "USD",
			TransactionTimestamp: timestamp,
			Account: AccountContext{
				ID: accountID,
			},
			Segment:   &SegmentContext{ID: segmentID},
			Portfolio: &PortfolioContext{ID: portfolioID},
		}

		input := req.ToCheckLimitsInput()

		require.NotNil(t, input)
		assert.Equal(t, req.Amount, input.Amount)
		assert.Equal(t, req.Currency, input.Currency)
		assert.Equal(t, accountID, input.AccountID)
		assert.Equal(t, &segmentID, input.SegmentID)
		assert.Equal(t, &portfolioID, input.PortfolioID)
		assert.Equal(t, timestamp, input.TransactionTimestamp)
	})

	t.Run("handles nil segment and portfolio", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(40)
		req := &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(41),
			TransactionType:      TransactionTypePix,
			SubType:              nil,
			Amount:               decimal.RequireFromString("100"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: accountID,
			},
			Segment:   nil,
			Portfolio: nil,
		}

		input := req.ToCheckLimitsInput()

		require.NotNil(t, input)
		assert.Equal(t, accountID, input.AccountID)
		assert.Nil(t, input.SegmentID)
		assert.Nil(t, input.PortfolioID)
	})

	// MerchantID must be propagated from ValidationRequest.Merchant.ID to
	// CheckLimitsInput.MerchantID so merchant-scoped limits are actually
	// enforced downstream by the limit checker. A nil Merchant must yield a
	// nil MerchantID (no spurious scoping).
	t.Run("propagates merchant id when merchant is present", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(50)
		merchantID := testutil.MustDeterministicUUID(51)
		req := &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(52),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("250"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: accountID,
			},
			Merchant: &MerchantContext{
				ID:       merchantID,
				Name:     "Deterministic Merchant",
				Category: "5411",
				Country:  "BR",
			},
		}

		input := req.ToCheckLimitsInput()

		require.NotNil(t, input)
		require.NotNil(t, input.MerchantID, "MerchantID must be set when request has a Merchant")
		require.Equal(t, merchantID, *input.MerchantID,
			"MerchantID must equal the ValidationRequest.Merchant.ID")
		// Cross-contamination guard: a typo such as `input.SegmentID = &r.Merchant.ID`
		// would pass the MerchantID assertion above silently. Pin the sibling
		// optional pointers to nil here to catch field-mis-assignment regressions
		// at the MerchantID propagation seam. Mirrors the sibling subtest
		// "handles nil segment and portfolio" style.
		assert.Nil(t, input.SegmentID, "MerchantID propagation must not leak into SegmentID")
		assert.Nil(t, input.PortfolioID, "MerchantID propagation must not leak into PortfolioID")
	})

	t.Run("normalizes subtype to lowercase after NormalizeAndValidate", func(t *testing.T) {
		subType := "  SELL  "
		accountID := testutil.MustDeterministicUUID(70)
		req := &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(71),
			TransactionType:      TransactionTypeCard,
			SubType:              &subType,
			Amount:               decimal.RequireFromString("50"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: accountID,
			},
		}

		err := req.NormalizeAndValidate(testutil.FixedTime())
		require.NoError(t, err)

		input := req.ToCheckLimitsInput()

		require.NotNil(t, input)
		require.NotNil(t, input.SubType)
		require.Equal(t, "sell", *input.SubType)
	})

	t.Run("NewValidationRequest normalizes subtype to lowercase", func(t *testing.T) {
		subType := "  SELL  "
		accountID := testutil.MustDeterministicUUID(80)

		req, err := NewValidationRequest(
			testutil.FixedTime(),
			testutil.MustDeterministicUUID(81),
			TransactionTypeCard,
			&subType,
			decimal.RequireFromString("50"),
			"USD",
			testutil.FixedTime(),
			AccountContext{ID: accountID},
			nil,
			nil,
			nil,
			nil,
		)

		require.NoError(t, err)
		require.NotNil(t, req.SubType)
		require.Equal(t, "sell", *req.SubType)
	})

	t.Run("leaves merchant id nil when merchant is nil", func(t *testing.T) {
		accountID := testutil.MustDeterministicUUID(60)
		req := &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(61),
			TransactionType:      TransactionTypePix,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "BRL",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: accountID,
			},
			Merchant: nil,
		}

		input := req.ToCheckLimitsInput()

		require.NotNil(t, input)
		require.Nil(t, input.MerchantID,
			"MerchantID must be nil when ValidationRequest.Merchant is nil (no spurious scoping)")
		// Sibling optional pointers must stay nil too: a bug that accidentally wrote
		// into SegmentID or PortfolioID when Merchant is nil would violate the
		// "only propagate what was provided" contract. Mirrors the positive subtest
		// above which pins the same invariant from the opposite direction.
		assert.Nil(t, input.SegmentID,
			"SegmentID must remain nil when ValidationRequest carries no Segment")
		assert.Nil(t, input.PortfolioID,
			"PortfolioID must remain nil when ValidationRequest carries no Portfolio")
	})
}
