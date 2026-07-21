// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTransactionValidation(t *testing.T) {
	t.Parallel()

	// Fixed time for deterministic testing
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		id        uuid.UUID
		decision  Decision
		createdAt time.Time
		expectErr bool
	}{
		{
			name:      "creates audit with ALLOW decision",
			id:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			decision:  DecisionAllow,
			createdAt: fixedTime,
			expectErr: false,
		},
		{
			name:      "creates audit with DENY decision",
			id:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"),
			decision:  DecisionDeny,
			createdAt: fixedTime,
			expectErr: false,
		},
		{
			name:      "creates audit with REVIEW decision",
			id:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440003"),
			decision:  DecisionReview,
			createdAt: fixedTime,
			expectErr: false,
		},
		{
			name:      "rejects invalid decision",
			id:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440004"),
			decision:  Decision("INVALID"),
			createdAt: fixedTime,
			expectErr: true,
		},
		{
			name:      "rejects zero UUID",
			id:        uuid.Nil,
			decision:  DecisionAllow,
			createdAt: fixedTime,
			expectErr: true,
		},
		{
			name:      "rejects zero createdAt",
			id:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440005"),
			decision:  DecisionAllow,
			createdAt: time.Time{},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewTransactionValidation(tc.id, tc.decision, tc.createdAt)

			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tc.id, result.ID)
			assert.Equal(t, tc.decision, result.Decision)

			// Verify slices are initialized (not nil) for proper JSON serialization
			assert.NotNil(t, result.MatchedRuleIDs, "MatchedRuleIDs should be initialized")
			assert.NotNil(t, result.EvaluatedRuleIDs, "EvaluatedRuleIDs should be initialized")
			assert.NotNil(t, result.LimitUsageDetails, "LimitUsageDetails should be initialized")

			// Verify slices are empty
			assert.Empty(t, result.MatchedRuleIDs)
			assert.Empty(t, result.EvaluatedRuleIDs)
			assert.Empty(t, result.LimitUsageDetails)

			// Verify CreatedAt matches the provided time (deterministic)
			assert.Equal(t, tc.createdAt, result.CreatedAt, "CreatedAt should match provided time")

			// Verify optional fields are zero values
			assert.Empty(t, result.Reason)
			assert.Zero(t, result.ProcessingTimeMs)
		})
	}
}
