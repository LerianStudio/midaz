// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

func TestFeeOperationRouteIDsRoundTripIndependentlyFromLegacyLabels(t *testing.T) {
	t.Parallel()

	legacyFrom := uuid.NewString()
	legacyTo := uuid.NewString()
	operationFrom := uuid.NewString()
	operationTo := uuid.NewString()
	deductible := false

	databaseFees, err := FromEntityFeeMap(map[string]model.Fee{"routed": {
		FeeLabel: "routed fee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: model.FlatFee,
			Calculations:    []model.Calculation{{Type: model.Flat, Value: "1"}},
		},
		ReferenceAmount:      model.OriginalAmount,
		Priority:             1,
		IsDeductibleFrom:     &deductible,
		CreditAccount:        "@fee-revenue",
		RouteFrom:            &legacyFrom,
		RouteTo:              &legacyTo,
		OperationRouteFromID: &operationFrom,
		OperationRouteToID:   &operationTo,
	}})
	require.NoError(t, err)

	persisted := databaseFees["routed"]
	assert.Equal(t, legacyFrom, *persisted.RouteFrom)
	assert.Equal(t, legacyTo, *persisted.RouteTo)
	assert.Equal(t, operationFrom, *persisted.OperationRouteFromID)
	assert.Equal(t, operationTo, *persisted.OperationRouteToID)

	roundTrip := ToEntityFeeMap(databaseFees)["routed"]
	assert.Equal(t, legacyFrom, *roundTrip.RouteFrom)
	assert.Equal(t, legacyTo, *roundTrip.RouteTo)
	assert.Equal(t, operationFrom, *roundTrip.OperationRouteFromID)
	assert.Equal(t, operationTo, *roundTrip.OperationRouteToID)
}
