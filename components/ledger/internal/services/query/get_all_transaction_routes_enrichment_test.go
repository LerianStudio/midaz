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
	"go.uber.org/mock/gomock"
)

// TestGetAllTransactionRoutes_OperationRoutesPopulated tests that the list endpoint
// returns each transaction route with its operationRoutes[] populated.
func TestGetAllTransactionRoutes_OperationRoutesPopulated(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	trID1 := uuid.New()
	trID2 := uuid.New()
	orID1 := uuid.New()
	orID2 := uuid.New()
	orID3 := uuid.New()

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
		Metadata:  &bson.M{},
	}

	transactionRoutes := []*mmodel.TransactionRoute{
		{ID: trID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route1"},
		{ID: trID2, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route2"},
	}

	cursor := libHTTP.CursorPagination{Next: "next", Prev: "prev"}

	// FindAll returns transaction routes without operation routes
	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(transactionRoutes, cursor, nil)

	// Metadata lookup
	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	// Junction table query: trID1 -> [orID1, orID2], trID2 -> [orID3]
	junctionMap := map[uuid.UUID][]uuid.UUID{
		trID1: {orID1, orID2},
		trID2: {orID3},
	}
	mockTRRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.InAnyOrder([]uuid.UUID{trID1, trID2})).
		Return(junctionMap, nil)

	// Batch fetch operation routes
	opRoutes := []*mmodel.OperationRoute{
		{ID: orID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "op1", OperationType: "source"},
		{ID: orID2, OrganizationID: organizationID, LedgerID: ledgerID, Title: "op2", OperationType: "destination"},
		{ID: orID3, OrganizationID: organizationID, LedgerID: ledgerID, Title: "op3", OperationType: "source"},
	}
	mockORRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(opRoutes, nil)

	result, curResult, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	require.NoError(t, err)
	assert.Equal(t, cursor, curResult)
	require.Len(t, result, 2)

	// trID1 should have 2 operation routes
	assert.Len(t, result[0].OperationRoutes, 2)
	assert.Equal(t, orID1, result[0].OperationRoutes[0].ID)
	assert.Equal(t, orID2, result[0].OperationRoutes[1].ID)

	// trID2 should have 1 operation route
	assert.Len(t, result[1].OperationRoutes, 1)
	assert.Equal(t, orID3, result[1].OperationRoutes[0].ID)
}

// TestGetAllTransactionRoutes_EmptyOperationRoutesNotNil tests that transaction routes
// with no linked operation routes return an empty slice, not nil.
func TestGetAllTransactionRoutes_EmptyOperationRoutesNotNil(t *testing.T) {
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
		Metadata:  &bson.M{},
	}

	transactionRoutes := []*mmodel.TransactionRoute{
		{ID: trID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route1"},
	}

	cursor := libHTTP.CursorPagination{Next: "next", Prev: "prev"}

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(transactionRoutes, cursor, nil)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	// Junction returns empty map — this route has no linked operation routes
	mockTRRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), []uuid.UUID{trID1}).
		Return(map[uuid.UUID][]uuid.UUID{}, nil)

	// FindByIDs should NOT be called when there are no operation route IDs
	// (no expectation set — gomock will fail if it's called)

	result, _, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	require.NoError(t, err)
	require.Len(t, result, 1)

	// Must be empty slice, NOT nil
	require.NotNil(t, result[0].OperationRoutes, "operationRoutes must be empty slice, not nil")
	assert.Empty(t, result[0].OperationRoutes)
}

// TestGetAllTransactionRoutes_EmptyResultNoExtraDBCalls tests that when FindAll returns
// an empty result set, no extra DB calls are made for operation route enrichment.
func TestGetAllTransactionRoutes_EmptyResultNoExtraDBCalls(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockTRRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo:    mockTRRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	cursor := libHTTP.CursorPagination{}

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, cursor, nil)

	// No calls to FindOperationRouteIDsByTransactionRouteIDs or FindByIDs expected
	// gomock will fail if they are called

	result, _, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

// TestGetAllTransactionRoutes_JunctionQueryError tests that when the junction table
// query fails, the enrichment error propagates and the endpoint returns an error.
func TestGetAllTransactionRoutes_JunctionQueryError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	trID1 := uuid.New()

	mockTRRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo:    mockTRRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		Metadata:  &bson.M{},
	}

	transactionRoutes := []*mmodel.TransactionRoute{
		{ID: trID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route1"},
	}

	cursor := libHTTP.CursorPagination{Next: "next", Prev: "prev"}

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(transactionRoutes, cursor, nil)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	// Junction table query returns error
	junctionErr := errors.New("junction table connection refused")
	mockTRRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), []uuid.UUID{trID1}).
		Return(nil, junctionErr)

	result, curResult, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, curResult)
	require.Error(t, err)
	assert.Equal(t, junctionErr, err)
}

// TestGetAllTransactionRoutes_FindByIDsError tests that when the batch fetch of
// operation routes fails, the enrichment error propagates correctly.
func TestGetAllTransactionRoutes_FindByIDsError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	trID1 := uuid.New()
	orID1 := uuid.New()

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
		Metadata:  &bson.M{},
	}

	transactionRoutes := []*mmodel.TransactionRoute{
		{ID: trID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route1"},
	}

	cursor := libHTTP.CursorPagination{Next: "next", Prev: "prev"}

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(transactionRoutes, cursor, nil)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	// Junction returns a valid mapping
	junctionMap := map[uuid.UUID][]uuid.UUID{
		trID1: {orID1},
	}
	mockTRRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), []uuid.UUID{trID1}).
		Return(junctionMap, nil)

	// FindByIDs returns error
	findByIDsErr := errors.New("operation route batch fetch timeout")
	mockORRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, findByIDsErr)

	result, curResult, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, curResult)
	require.Error(t, err)
	assert.Equal(t, findByIDsErr, err)
}

// TestGetAllTransactionRoutes_MixedLinksAndNoLinks tests that when some transaction
// routes have linked operation routes and others don't, each gets the correct result:
// populated []OperationRoute for those with links, empty []OperationRoute for those without.
func TestGetAllTransactionRoutes_MixedLinksAndNoLinks(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	trID1 := uuid.New()
	trID2 := uuid.New()
	trID3 := uuid.New()
	orID1 := uuid.New()

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
		Metadata:  &bson.M{},
	}

	transactionRoutes := []*mmodel.TransactionRoute{
		{ID: trID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route-with-link"},
		{ID: trID2, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route-without-link"},
		{ID: trID3, OrganizationID: organizationID, LedgerID: ledgerID, Title: "route-empty-link"},
	}

	cursor := libHTTP.CursorPagination{Next: "next", Prev: "prev"}

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(transactionRoutes, cursor, nil)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	// Junction: trID1 -> [orID1], trID2 not in map, trID3 -> empty slice
	junctionMap := map[uuid.UUID][]uuid.UUID{
		trID1: {orID1},
		trID3: {},
	}
	mockTRRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.Any()).
		Return(junctionMap, nil)

	opRoutes := []*mmodel.OperationRoute{
		{ID: orID1, OrganizationID: organizationID, LedgerID: ledgerID, Title: "op1", OperationType: "source"},
	}
	mockORRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(opRoutes, nil)

	result, _, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	require.NoError(t, err)
	require.Len(t, result, 3)

	// trID1: has 1 operation route
	assert.Len(t, result[0].OperationRoutes, 1)
	assert.Equal(t, orID1, result[0].OperationRoutes[0].ID)

	// trID2: not in junction map -> empty slice, not nil
	require.NotNil(t, result[1].OperationRoutes)
	assert.Empty(t, result[1].OperationRoutes)

	// trID3: in junction map with empty slice -> empty slice, not nil
	require.NotNil(t, result[2].OperationRoutes)
	assert.Empty(t, result[2].OperationRoutes)
}

// TestGetAllTransactionRoutes_EmptyTransactionRoutesSlice tests that when FindAll returns
// an empty (non-nil) slice, enrichment handles it without extra DB calls.
// This exercises the len(transactionRoutes) == 0 early-return in enrichTransactionRoutesWithOperationRoutes.
func TestGetAllTransactionRoutes_EmptyTransactionRoutesSlice(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockTRRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo:    mockTRRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
		Metadata:  &bson.M{},
	}

	// Return empty (non-nil) slice — this enters the transactionRoutes != nil branch
	// but len == 0 triggers the early return in enrichTransactionRoutesWithOperationRoutes
	emptySlice := []*mmodel.TransactionRoute{}
	cursor := libHTTP.CursorPagination{}

	mockTRRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(emptySlice, cursor, nil)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	// No calls to FindOperationRouteIDsByTransactionRouteIDs or FindByIDs expected
	// since the enrichment function returns early for an empty slice

	result, _, err := uc.GetAllTransactionRoutes(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Empty(t, result)
}
