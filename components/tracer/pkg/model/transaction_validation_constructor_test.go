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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestNewTransactionValidation_Validation(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	validID := testutil.MustDeterministicUUID(1)

	t.Run("Success - creates with ALLOW decision", func(t *testing.T) {
		result, err := NewTransactionValidation(validID, DecisionAllow, fixedTime)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, validID, result.ID)
		assert.Equal(t, DecisionAllow, result.Decision)
		assert.Equal(t, fixedTime, result.CreatedAt)
		assert.NotNil(t, result.MatchedRuleIDs, "MatchedRuleIDs should be initialized")
		assert.NotNil(t, result.EvaluatedRuleIDs, "EvaluatedRuleIDs should be initialized")
		assert.NotNil(t, result.LimitUsageDetails, "LimitUsageDetails should be initialized")
		assert.Empty(t, result.MatchedRuleIDs)
		assert.Empty(t, result.EvaluatedRuleIDs)
		assert.Empty(t, result.LimitUsageDetails)
	})

	t.Run("Success - creates with DENY decision", func(t *testing.T) {
		result, err := NewTransactionValidation(validID, DecisionDeny, fixedTime)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, DecisionDeny, result.Decision)
	})

	t.Run("Success - creates with REVIEW decision", func(t *testing.T) {
		result, err := NewTransactionValidation(validID, DecisionReview, fixedTime)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, DecisionReview, result.Decision)
	})

	t.Run("Error - nil UUID", func(t *testing.T) {
		result, err := NewTransactionValidation(uuid.Nil, DecisionAllow, fixedTime)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, constant.ErrTransactionValidationIDRequired)
	})

	t.Run("Error - invalid decision", func(t *testing.T) {
		result, err := NewTransactionValidation(validID, Decision("INVALID"), fixedTime)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, constant.ErrInvalidDecision)
	})

	t.Run("Error - zero createdAt", func(t *testing.T) {
		result, err := NewTransactionValidation(validID, DecisionAllow, time.Time{})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, constant.ErrTransactionValidationCreatedAtRequired)
	})

	t.Run("Success - all optional fields zero values", func(t *testing.T) {
		result, err := NewTransactionValidation(validID, DecisionAllow, fixedTime)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Reason, "Reason should be empty")
		assert.Zero(t, result.ProcessingTimeMs, "ProcessingTimeMs should be zero")
	})

	t.Run("All decisions are valid", func(t *testing.T) {
		decisions := []Decision{DecisionAllow, DecisionDeny, DecisionReview}

		for _, decision := range decisions {
			t.Run(string(decision), func(t *testing.T) {
				result, err := NewTransactionValidation(validID, decision, fixedTime)

				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, decision, result.Decision)
			})
		}
	})
}
