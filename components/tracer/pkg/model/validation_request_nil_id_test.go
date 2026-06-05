// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// =============================================================================
// NormalizeAndValidate Tests for uuid.Nil Rejection
// Tests that NormalizeAndValidate rejects uuid.Nil as RequestID
// =============================================================================

func TestValidationRequest_NormalizeAndValidate_RequestIDNil(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		requestID   uuid.UUID
		expectedErr error
		description string
	}{
		{
			name:        "rejects uuid.Nil with ErrValidationRequestIDRequired",
			requestID:   uuid.Nil,
			expectedErr: constant.ErrValidationRequestIDRequired,
			description: "uuid.Nil (all zeros) should return ErrValidationRequestIDRequired",
		},
		{
			name:        "accepts valid deterministic UUID",
			requestID:   testutil.MustDeterministicUUID(50),
			expectedErr: nil,
			description: "valid UUIDs should pass validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			accountID := testutil.MustDeterministicUUID(1)
			req := &ValidationRequest{
				RequestID:            tt.requestID,
				TransactionType:      TransactionTypeCard,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				TransactionTimestamp: testutil.FixedTime(),
				Account: AccountContext{
					ID:     accountID,
					Type:   "checking",
					Status: "active",
				},
			}

			err := req.NormalizeAndValidate(testutil.FixedTime())

			if tt.expectedErr == nil {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
				assert.ErrorIs(t, err, tt.expectedErr,
					"expected %v but got %v - %s", tt.expectedErr, err, tt.description)
			}
		})
	}
}
