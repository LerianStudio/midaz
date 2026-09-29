// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchedRuleIDsOrEmpty(t *testing.T) {
	ruleID := uuid.MustParse("00000000-0000-0000-0000-000000000005")

	assert.Equal(t, []uuid.UUID{}, matchedRuleIDsOrEmpty(nil))
	assert.Equal(t, []uuid.UUID{ruleID}, matchedRuleIDsOrEmpty([]uuid.UUID{ruleID}))
}

func TestReserveResponse_JSONShape(t *testing.T) {
	t.Run("allow serializes decision and empty arrays, omits reason", func(t *testing.T) {
		body, err := json.Marshal(ReserveResponse{
			TransactionID:  uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			Decision:       "ALLOW",
			ReservationIDs: reservationIDsOrEmpty(nil),
			MatchedRuleIDs: matchedRuleIDsOrEmpty(nil),
		})
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal(body, &got))

		assert.Equal(t, "ALLOW", got["decision"])
		assert.Equal(t, []any{}, got["reservationIds"])
		assert.Equal(t, []any{}, got["matchedRuleIds"])
		assert.NotContains(t, got, "reason")
		assert.Len(t, got, 5)
	})

	t.Run("review carries reason and matched rule ids", func(t *testing.T) {
		ruleID := uuid.MustParse("00000000-0000-0000-0000-000000000005")

		body, err := json.Marshal(ReserveResponse{
			Denied:         true,
			Decision:       "REVIEW",
			Reason:         "manual review required",
			ReservationIDs: reservationIDsOrEmpty(nil),
			MatchedRuleIDs: matchedRuleIDsOrEmpty([]uuid.UUID{ruleID}),
		})
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal(body, &got))

		assert.Equal(t, "REVIEW", got["decision"])
		assert.Equal(t, "manual review required", got["reason"])
		assert.Equal(t, []any{ruleID.String()}, got["matchedRuleIds"])
		assert.Len(t, got, 6)
	})
}
