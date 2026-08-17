// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFeeOperationRouteIDsUpdateAndJSONContract(t *testing.T) {
	t.Parallel()

	fromID := uuid.NewString()
	toID := uuid.NewString()
	fee := &Fee{
		OperationRouteFromID: &fromID,
		OperationRouteToID:   &toID,
	}
	fields := bson.M{}
	unsetFields := bson.M{}

	assert.True(t, fee.updateOperationRouteFromID("service", fields, unsetFields))
	assert.True(t, fee.updateOperationRouteToID("service", fields, unsetFields))
	assert.Equal(t, &fromID, fields["fees.service.operation_route_from_id"])
	assert.Equal(t, &toID, fields["fees.service.operation_route_to_id"])
	assert.Empty(t, unsetFields)
	assert.False(t, fee.ValidateIfFeeIsNil(), "an operation-route-only patch must not be mistaken for fee removal")

	raw, err := json.Marshal(fee)
	require.NoError(t, err)
	assert.JSONEq(t, `{"feeLabel":"","calculationModel":null,"referenceAmount":"","isDeductibleFrom":null,"creditAccount":"","operationRouteFromId":"`+fromID+`","operationRouteToId":"`+toID+`"}`, string(raw))
}

func TestFeeOperationRouteIDPatchDistinguishesOmittedNullAndValue(t *testing.T) {
	t.Parallel()

	validID := uuid.NewString()
	tests := []struct {
		name        string
		raw         string
		wantRemoval bool
		wantRouteID *string
		wantNull    bool
	}{
		{
			name:        "omitted preserves the existing field and empty fee means removal",
			raw:         `{}`,
			wantRemoval: true,
		},
		{
			name:        "null clears only the field",
			raw:         `{"operationRouteFromId":null}`,
			wantRemoval: false,
			wantNull:    true,
		},
		{
			name:        "string updates the field",
			raw:         `{"operationRouteFromId":"` + validID + `"}`,
			wantRemoval: false,
			wantRouteID: &validID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var fee Fee
			require.NoError(t, json.Unmarshal([]byte(tt.raw), &fee))
			assert.Equal(t, tt.wantRemoval, fee.ValidateIfFeeIsNil())
			assert.Equal(t, tt.wantRouteID, fee.OperationRouteFromID)

			marshaled, err := json.Marshal(fee)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(marshaled, &fields))
			if tt.wantNull {
				value, exists := fields["operationRouteFromId"]
				assert.True(t, exists)
				assert.Nil(t, value)
			} else if tt.wantRouteID == nil {
				assert.NotContains(t, fields, "operationRouteFromId")
			}
		})
	}
}
