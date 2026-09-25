// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func bridgeOperationRoute(id uuid.UUID) *mmodel.OperationRoute {
	return &mmodel.OperationRoute{
		ID:            id,
		OperationType: constant.OperationRouteTypeBidirectional,
		AccountingEntries: &mmodel.AccountingEntries{CrossLedger: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "1900", Description: "Arriving"},
			Credit: &mmodel.AccountingRubric{Code: "2900", Description: "Leaving"},
		}},
	}
}

func requireInvalidCrossLedgerRoute(t *testing.T, err error) {
	t.Helper()

	var businessErr pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &businessErr)
	assert.Equal(t, constant.ErrInvalidCrossLedgerRoute.Error(), businessErr.Code)
}

func TestValidateOperationRouteTypes_AtMostOneBridgeRoute(t *testing.T) {
	t.Parallel()

	source := &mmodel.OperationRoute{ID: uuid.New(), OperationType: constant.OperationRouteTypeSource}
	destination := &mmodel.OperationRoute{ID: uuid.New(), OperationType: constant.OperationRouteTypeDestination}

	require.NoError(t, validateOperationRouteTypes([]*mmodel.OperationRoute{source, destination}))
	require.NoError(t, validateOperationRouteTypes([]*mmodel.OperationRoute{source, destination, bridgeOperationRoute(uuid.New())}))
	requireInvalidCrossLedgerRoute(t, validateOperationRouteTypes([]*mmodel.OperationRoute{
		source, destination, bridgeOperationRoute(uuid.New()), bridgeOperationRoute(uuid.New()),
	}))
}

// The bridge route is bidirectional, but it only ever classifies the synthetic
// bridge legs, so it cannot be a transaction route's client source or
// destination: a route that saved on the bridge alone would refuse every
// transaction at run time.
func TestValidateOperationRouteTypes_TheBridgeRouteIsNeitherSourceNorDestination(t *testing.T) {
	t.Parallel()

	source := &mmodel.OperationRoute{ID: uuid.New(), OperationType: constant.OperationRouteTypeSource}
	destination := &mmodel.OperationRoute{ID: uuid.New(), OperationType: constant.OperationRouteTypeDestination}
	bridge := bridgeOperationRoute(uuid.New())

	requireBusinessCode := func(t *testing.T, err error, want error) {
		t.Helper()

		var businessErr pkg.ValidationError
		require.ErrorAs(t, err, &businessErr)
		assert.Equal(t, want.Error(), businessErr.Code)
	}

	requireBusinessCode(t, validateOperationRouteTypes([]*mmodel.OperationRoute{source, bridge}), constant.ErrNoDestinationForAction)
	requireBusinessCode(t, validateOperationRouteTypes([]*mmodel.OperationRoute{bridge, destination}), constant.ErrNoSourceForAction)
	requireBusinessCode(t, validateOperationRouteTypes([]*mmodel.OperationRoute{bridge}), constant.ErrNoSourceForAction)
	require.NoError(t, validateOperationRouteTypes([]*mmodel.OperationRoute{source, destination, bridge}))
}

func TestCreateTransactionRoute_RejectsASecondBridgeRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	routes := []*mmodel.OperationRoute{
		{ID: uuid.New(), OperationType: constant.OperationRouteTypeSource},
		{ID: uuid.New(), OperationType: constant.OperationRouteTypeDestination},
		bridgeOperationRoute(uuid.New()),
		bridgeOperationRoute(uuid.New()),
	}
	ids := make([]uuid.UUID, len(routes))
	for i := range routes {
		ids[i] = routes[i].ID
	}

	operationRoutes := operationroute.NewMockRepository(ctrl)
	operationRoutes.EXPECT().FindByIDs(gomock.Any(), organizationID, ids).Return(routes, nil)

	// No TransactionRouteRepo expectation: nothing may be persisted.
	uc := &UseCase{OperationRouteRepo: operationRoutes, TransactionRouteRepo: transactionroute.NewMockRepository(ctrl)}

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, &ledgerID, &mmodel.CreateTransactionRouteInput{
		Title: "Settlement", OperationRoutes: ids,
	})

	assert.Nil(t, result)
	requireInvalidCrossLedgerRoute(t, err)
}

func TestUpdateTransactionRoute_RejectsASecondBridgeRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	organizationID := uuid.New()
	transactionRouteID := uuid.New()
	routes := []*mmodel.OperationRoute{
		{ID: uuid.New(), OperationType: constant.OperationRouteTypeSource},
		{ID: uuid.New(), OperationType: constant.OperationRouteTypeDestination},
		bridgeOperationRoute(uuid.New()),
		bridgeOperationRoute(uuid.New()),
	}
	ids := make([]uuid.UUID, len(routes))
	for i := range routes {
		ids[i] = routes[i].ID
	}

	transactionRoutes := transactionroute.NewMockRepository(ctrl)
	transactionRoutes.EXPECT().FindByID(gomock.Any(), organizationID, transactionRouteID).
		Return(&mmodel.TransactionRoute{ID: transactionRouteID}, nil)

	operationRoutes := operationroute.NewMockRepository(ctrl)
	operationRoutes.EXPECT().FindByIDs(gomock.Any(), organizationID, ids).Return(routes, nil)

	uc := &UseCase{TransactionRouteRepo: transactionRoutes, OperationRouteRepo: operationRoutes}

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, transactionRouteID, &mmodel.UpdateTransactionRouteInput{
		OperationRoutes: &ids,
	})

	assert.Nil(t, result)
	requireInvalidCrossLedgerRoute(t, err)
}

func TestUpdateOperationRoute_BridgeEntryKeepsOneBridgePerTransactionRoute(t *testing.T) {
	organizationID := uuid.New()
	operationRouteID := uuid.New()
	transactionRouteID := uuid.New()
	otherRouteID := uuid.New()
	patch := &mmodel.UpdateOperationRouteInput{AccountingEntries: bridgeOperationRoute(operationRouteID).AccountingEntries}

	t.Run("adding it where a linked transaction route already has a bridge route is refused", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		operationRoutes := operationroute.NewMockRepository(ctrl)
		transactionRoutes := transactionroute.NewMockRepository(ctrl)

		operationRoutes.EXPECT().FindTransactionRouteIDs(gomock.Any(), operationRouteID).Return([]uuid.UUID{transactionRouteID}, nil)
		transactionRoutes.EXPECT().FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), []uuid.UUID{transactionRouteID}).
			Return(map[uuid.UUID][]uuid.UUID{transactionRouteID: {operationRouteID, otherRouteID}}, nil)
		operationRoutes.EXPECT().FindByIDs(gomock.Any(), organizationID, []uuid.UUID{otherRouteID}).
			Return([]*mmodel.OperationRoute{bridgeOperationRoute(otherRouteID)}, nil)

		uc := &UseCase{OperationRouteRepo: operationRoutes, TransactionRouteRepo: transactionRoutes}

		result, err := uc.UpdateOperationRoute(context.Background(), organizationID, operationRouteID, patch)

		assert.Nil(t, result)
		requireInvalidCrossLedgerRoute(t, err)
	})

	t.Run("adding it where no linked transaction route has a bridge route is saved", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		operationRoutes := operationroute.NewMockRepository(ctrl)
		transactionRoutes := transactionroute.NewMockRepository(ctrl)
		metadata := mongodb.NewMockRepository(ctrl)

		operationRoutes.EXPECT().FindTransactionRouteIDs(gomock.Any(), operationRouteID).Return([]uuid.UUID{transactionRouteID}, nil)
		transactionRoutes.EXPECT().FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), []uuid.UUID{transactionRouteID}).
			Return(map[uuid.UUID][]uuid.UUID{transactionRouteID: {operationRouteID, otherRouteID}}, nil)
		operationRoutes.EXPECT().FindByIDs(gomock.Any(), organizationID, []uuid.UUID{otherRouteID}).
			Return([]*mmodel.OperationRoute{{ID: otherRouteID, OperationType: constant.OperationRouteTypeSource}}, nil)
		operationRoutes.EXPECT().Update(gomock.Any(), organizationID, operationRouteID, gomock.Any()).
			Return(bridgeOperationRoute(operationRouteID), nil)
		metadata.EXPECT().Update(gomock.Any(), constant.EntityOperationRoute, operationRouteID.String(), gomock.Any()).Return(nil)

		uc := &UseCase{OperationRouteRepo: operationRoutes, TransactionRouteRepo: transactionRoutes, TransactionMetadataRepo: metadata}

		result, err := uc.UpdateOperationRoute(context.Background(), organizationID, operationRouteID, patch)

		require.NoError(t, err)
		assert.Equal(t, operationRouteID, result.ID)
	})
}
