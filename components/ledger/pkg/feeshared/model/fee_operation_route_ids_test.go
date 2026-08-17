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

	assert.True(t, fee.updateOperationRouteFromID("service", fields))
	assert.True(t, fee.updateOperationRouteToID("service", fields))
	assert.Equal(t, &fromID, fields["fees.service.operation_route_from_id"])
	assert.Equal(t, &toID, fields["fees.service.operation_route_to_id"])
	assert.False(t, fee.ValidateIfFeeIsNil(), "an operation-route-only patch must not be mistaken for fee removal")

	raw, err := json.Marshal(fee)
	require.NoError(t, err)
	assert.JSONEq(t, `{"feeLabel":"","calculationModel":null,"referenceAmount":"","isDeductibleFrom":null,"creditAccount":"","operationRouteFromId":"`+fromID+`","operationRouteToId":"`+toID+`"}`, string(raw))
}
