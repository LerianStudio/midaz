// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

func decodeOperationRoutePatch(t *testing.T, raw string) *model.UpdatePackageInput {
	t.Helper()

	var input model.UpdatePackageInput
	require.NoError(t, json.Unmarshal([]byte(raw), &input))

	return &input
}

func TestUpdatePackageByID_TwoOperationRouteOnlyFeesPreserveEffectivePrioritiesAndWriteAtomically(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packageID := uuid.New()
	firstRouteID := uuid.NewString()
	secondRouteID := uuid.NewString()
	existing := map[string]model.Fee{
		"firstFee":  {Priority: 1},
		"secondFee": {Priority: 2},
	}
	input := decodeOperationRoutePatch(t, `{"fees":{"firstFee":{"operationRouteFromId":"`+firstRouteID+`"},"secondFee":{"operationRouteToId":"`+secondRouteID+`"}}}`)
	require.NoError(t, input.ValidateFees())

	repo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), orgID, packageID).
		Return(&model.AmountData{
			MinAmount: decimal.NewFromInt(1),
			MaxAmount: decimal.NewFromInt(100),
			Fees:      existing,
			LedgerID:  ledgerID,
		}, nil)

	var captured *bson.M
	repo.EXPECT().
		Update(gomock.Any(), packageID, orgID, uuid.Nil, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update *bson.M) (*pack.Package, error) {
			captured = update

			return &pack.Package{ID: packageID, LedgerID: ledgerID}, nil
		}).
		Times(1)

	svc := &UseCase{packageRepo: repo}
	require.NoError(t, svc.UpdatePackageByID(context.Background(), packageID, orgID, uuid.Nil, input))
	require.NotNil(t, captured)

	setFields, ok := (*captured)["$set"].(bson.M)
	require.True(t, ok)
	assert.Equal(t, &firstRouteID, setFields["fees.firstFee.operation_route_from_id"])
	assert.Equal(t, &secondRouteID, setFields["fees.secondFee.operation_route_to_id"])
	assert.Contains(t, setFields, "updated_at")
	assert.Len(t, setFields, 3, "partial PATCH must not replace either complete fee")
	assert.NotContains(t, *captured, "$unset")
}

func TestUpdatePackageByID_NullClearsOneOperationRouteWithoutRemovingFee(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jsonField string
		mongoPath string
	}{
		{
			name:      "clear source operation route",
			jsonField: "operationRouteFromId",
			mongoPath: "fees.serviceFee.operation_route_from_id",
		},
		{
			name:      "clear destination operation route",
			jsonField: "operationRouteToId",
			mongoPath: "fees.serviceFee.operation_route_to_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := pack.NewMockRepository(ctrl)
			orgID := uuid.New()
			ledgerID := uuid.New()
			packageID := uuid.New()
			fromID := uuid.NewString()
			toID := uuid.NewString()
			input := decodeOperationRoutePatch(t, `{"fees":{"serviceFee":{"`+tt.jsonField+`":null}}}`)
			require.NoError(t, input.ValidateFees())

			repo.EXPECT().
				FindFeesAndAmountDataByPackageID(gomock.Any(), orgID, packageID).
				Return(&model.AmountData{
					MinAmount: decimal.NewFromInt(1),
					MaxAmount: decimal.NewFromInt(100),
					Fees: map[string]model.Fee{"serviceFee": {
						Priority:             1,
						OperationRouteFromID: &fromID,
						OperationRouteToID:   &toID,
					}},
					LedgerID: ledgerID,
				}, nil)

			var captured *bson.M
			repo.EXPECT().
				Update(gomock.Any(), packageID, orgID, uuid.Nil, gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update *bson.M) (*pack.Package, error) {
					captured = update

					return &pack.Package{ID: packageID, LedgerID: ledgerID}, nil
				}).
				Times(1)

			svc := &UseCase{packageRepo: repo}
			require.NoError(t, svc.UpdatePackageByID(context.Background(), packageID, orgID, uuid.Nil, input))
			require.NotNil(t, captured)

			unsetFields, ok := (*captured)["$unset"].(bson.M)
			require.True(t, ok)
			assert.Equal(t, "", unsetFields[tt.mongoPath])
			assert.NotContains(t, unsetFields, "fees.serviceFee", "null must clear the field, not remove the fee")
			assert.Len(t, unsetFields, 1)

			setFields, ok := (*captured)["$set"].(bson.M)
			require.True(t, ok)
			assert.Contains(t, setFields, "updated_at")
			assert.Len(t, setFields, 1)
		})
	}
}

func TestUpdatePackageByID_EmptyFeeObjectStillRemovesTheFee(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packageID := uuid.New()
	input := decodeOperationRoutePatch(t, `{"fees":{"serviceFee":{}}}`)
	require.NoError(t, input.ValidateFees())

	repo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), orgID, packageID).
		Return(&model.AmountData{
			MinAmount: decimal.NewFromInt(1),
			MaxAmount: decimal.NewFromInt(100),
			Fees:      map[string]model.Fee{"serviceFee": {Priority: 1}},
			LedgerID:  ledgerID,
		}, nil)

	var captured *bson.M
	repo.EXPECT().
		Update(gomock.Any(), packageID, orgID, uuid.Nil, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update *bson.M) (*pack.Package, error) {
			captured = update

			return &pack.Package{ID: packageID, LedgerID: ledgerID}, nil
		})

	svc := &UseCase{packageRepo: repo}
	require.NoError(t, svc.UpdatePackageByID(context.Background(), packageID, orgID, uuid.Nil, input))
	require.NotNil(t, captured)

	unsetFields, ok := (*captured)["$unset"].(bson.M)
	require.True(t, ok)
	assert.Equal(t, "", unsetFields["fees.serviceFee"])
	assert.Len(t, unsetFields, 1)
}
