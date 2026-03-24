// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
)

// TestGetAllMetadataTransactionRoutes_OperationRoutesPopulated tests that the metadata
// listing also returns operation routes populated.
func TestGetAllMetadataTransactionRoutes_OperationRoutesPopulated(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	trID1 := uuid.New()
	orID1 := uuid.New()
	orID2 := uuid.New()

	mockTRRepo := transactionroute.NewMockRepository(ctrl)
	mockORRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo:    mockTRRepo,
		OperationRouteRepo:      mockORRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		Metadata:  &bson.M{"key": "value"},
	}

	// Metadata with one matching route
	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: trID1.String(),
			Data:     mongodb.JSON{"key": "value"},
		},
	}

	allTransactionRoutes := []*mmodel.TransactionRoute{
		{ID: trID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route1"},
		{ID: uuid.New(), OrganizationID: organizationID, LedgerID: ledgerID, Title: "route2"},
	}

	cursor := libHTTP.CursorPagination{Next: "next", Prev: "prev"}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedMetadata, nil)

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(allTransactionRoutes, cursor, nil)

	// Junction: trID1 -> [orID1, orID2]
	junctionMap := map[uuid.UUID][]uuid.UUID{
		trID1: {orID1, orID2},
	}
	mockTRRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), []uuid.UUID{trID1}).
		Return(junctionMap, nil)

	opRoutes := []*mmodel.OperationRoute{
		{ID: orID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "op1", OperationType: "source"},
		{ID: orID2, OrganizationID: organizationID, LedgerID: ledgerID, Title: "op2", OperationType: "destination"},
	}
	mockORRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(opRoutes, nil)

	result, curResult, err := uc.GetAllMetadataTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	require.NoError(t, err)
	assert.Equal(t, cursor, curResult)
	require.Len(t, result, 1)

	// The filtered route should have operation routes populated
	assert.Len(t, result[0].OperationRoutes, 2)
	assert.Equal(t, orID1, result[0].OperationRoutes[0].ID)
	assert.Equal(t, orID2, result[0].OperationRoutes[1].ID)
	assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
}

// TestGetAllMetadataTransactionRoutes_JunctionQueryError tests that when the junction
// table query fails during metadata-filtered listing, the error propagates correctly.
func TestGetAllMetadataTransactionRoutes_JunctionQueryError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	trID1 := uuid.New()

	mockTRRepo := transactionroute.NewMockRepository(ctrl)
	mockORRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo:    mockTRRepo,
		OperationRouteRepo:      mockORRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		Metadata:  &bson.M{"key": "value"},
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: trID1.String(),
			Data:     mongodb.JSON{"key": "value"},
		},
	}

	allTransactionRoutes := []*mmodel.TransactionRoute{
		{ID: trID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route1"},
	}

	cursor := libHTTP.CursorPagination{Next: "next", Prev: "prev"}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedMetadata, nil)

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(allTransactionRoutes, cursor, nil)

	// Junction table query returns error
	junctionErr := errors.New("junction table connection refused")
	mockTRRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), []uuid.UUID{trID1}).
		Return(nil, junctionErr)

	result, curResult, err := uc.GetAllMetadataTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, curResult)
	require.Error(t, err)
	assert.Equal(t, junctionErr, err)
}

// TestGetAllMetadataTransactionRoutes_EmptyOperationRoutesNotNil tests that metadata-filtered
// transaction routes with no linked operation routes return empty slice, not nil.
func TestGetAllMetadataTransactionRoutes_EmptyOperationRoutesNotNil(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	trID1 := uuid.New()

	mockTRRepo := transactionroute.NewMockRepository(ctrl)
	mockORRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo:    mockTRRepo,
		OperationRouteRepo:      mockORRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		Metadata:  &bson.M{"key": "value"},
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			ID:       primitive.NewObjectID(),
			EntityID: trID1.String(),
			Data:     mongodb.JSON{"key": "value"},
		},
	}

	allTransactionRoutes := []*mmodel.TransactionRoute{
		{ID: trID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route1"},
	}

	cursor := libHTTP.CursorPagination{Next: "next", Prev: "prev"}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedMetadata, nil)

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(allTransactionRoutes, cursor, nil)

	// Junction returns empty map — no linked operation routes
	mockTRRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), []uuid.UUID{trID1}).
		Return(map[uuid.UUID][]uuid.UUID{}, nil)

	result, curResult, err := uc.GetAllMetadataTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	require.NoError(t, err)
	assert.Equal(t, cursor, curResult)
	require.Len(t, result, 1)

	// Must be empty slice, NOT nil
	require.NotNil(t, result[0].OperationRoutes, "operationRoutes must be empty slice, not nil")
	assert.Empty(t, result[0].OperationRoutes)
	assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
}
