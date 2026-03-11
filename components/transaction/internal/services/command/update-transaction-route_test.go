// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateTransactionRouteSuccess tests successfully updating a transaction route with metadata
func TestUpdateTransactionRouteSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	input := &mmodel.UpdateTransactionRouteInput{
		Title:       "Updated Title",
		Description: "Updated Description",
		Metadata:    map[string]any{"key": "updated_value"},
	}

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          input.Title,
		Description:    input.Description,
	}

	expectedMetadata := map[string]any{"key": "updated_value"}

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledgerID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []mmodel.OperationRouteActionInput) (*mmodel.TransactionRoute, error) {
			assert.Equal(t, input.Title, tr.Title)
			assert.Equal(t, input.Description, tr.Description)
			assert.Empty(t, toAdd)
			assert.Empty(t, toRemove)
			return expectedTransactionRoute, nil
		}).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String(), expectedMetadata).
		Return(nil).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedTransactionRoute.ID, result.ID)
	assert.Equal(t, expectedTransactionRoute.Title, result.Title)
	assert.Equal(t, expectedTransactionRoute.Description, result.Description)
	assert.Equal(t, expectedMetadata, result.Metadata)
}

// TestUpdateTransactionRouteNotFound tests updating a non-existent transaction route
func TestUpdateTransactionRouteNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	input := &mmodel.UpdateTransactionRouteInput{
		Title:       "Updated Title",
		Description: "Updated Description",
	}

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)

	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedBusinessError, err)
	assert.Nil(t, result)
}

// TestUpdateTransactionRouteRepositoryError tests updating a transaction route with repository error
func TestUpdateTransactionRouteRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	input := &mmodel.UpdateTransactionRouteInput{
		Title:       "Updated Title",
		Description: "Updated Description",
	}

	expectedError := errors.New("database connection error")

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

// TestUpdateTransactionRouteMetadataError tests updating a transaction route with metadata error
func TestUpdateTransactionRouteMetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	input := &mmodel.UpdateTransactionRouteInput{
		Title:       "Updated Title",
		Description: "Updated Description",
		Metadata:    map[string]any{"key": "updated_value"},
	}

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          input.Title,
		Description:    input.Description,
	}

	metadataError := errors.New("metadata update error")

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(expectedTransactionRoute, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, metadataError).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)
	assert.Equal(t, metadataError, err)
	assert.Nil(t, result)
}

// TestUpdateTransactionRouteWithOperationRoutes tests updating operation route relationships
func TestUpdateTransactionRouteWithOperationRoutes(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	opRouteID1 := uuid.New()
	opRouteID2 := uuid.New()

	input := &mmodel.UpdateTransactionRouteInput{
		Title:       "Updated Route",
		Description: "Updated Description",
		OperationRoutes: &[]mmodel.OperationRouteActionInput{
			{Action: "direct", OperationRouteID: opRouteID1},
			{Action: "direct", OperationRouteID: opRouteID2},
		},
		Metadata: map[string]any{"key": "value"},
	}

	existingOpRouteID1 := uuid.Must(libCommons.GenerateUUIDv7())
	existingOpRouteID2 := uuid.Must(libCommons.GenerateUUIDv7())

	currentTransactionRoute := &mmodel.TransactionRoute{
		ID: transactionRouteID,
		OperationRoutes: []mmodel.OperationRoute{
			{ID: existingOpRouteID1, OperationType: "source", Action: "direct"},
			{ID: existingOpRouteID2, OperationType: "destination", Action: "direct"},
		},
	}

	transactionRoute := &mmodel.TransactionRoute{
		ID:          transactionRouteID,
		Title:       input.Title,
		Description: input.Description,
		OperationRoutes: []mmodel.OperationRoute{
			{ID: opRouteID1, OperationType: "source"},
			{ID: opRouteID2, OperationType: "destination"},
		},
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	operationRoutes := []*mmodel.OperationRoute{
		{ID: opRouteID1, OperationType: "source"},
		{ID: opRouteID2, OperationType: "destination"},
	}

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(currentTransactionRoute, nil).
		Times(1)

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(operationRoutes, nil).
		Times(1)

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, lID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []mmodel.OperationRouteActionInput) (*mmodel.TransactionRoute, error) {
			// All existing routes should be removed and new ones added (different route IDs)
			assert.Len(t, toAdd, 2)
			assert.Len(t, toRemove, 2)

			addIDs := make(map[uuid.UUID]string)
			for _, entry := range toAdd {
				addIDs[entry.OperationRouteID] = entry.Action
			}

			assert.Equal(t, "direct", addIDs[opRouteID1])
			assert.Equal(t, "direct", addIDs[opRouteID2])

			removeIDs := make(map[uuid.UUID]string)
			for _, entry := range toRemove {
				removeIDs[entry.OperationRouteID] = entry.Action
			}

			assert.Equal(t, "direct", removeIDs[existingOpRouteID1])
			assert.Equal(t, "direct", removeIDs[existingOpRouteID2])

			return transactionRoute, nil
		}).
		Times(1)

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, nil).
		Times(1)

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		Update(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String(), input.Metadata).
		Return(nil).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, input.Title, result.Title)
	assert.Equal(t, input.Description, result.Description)
	assert.Len(t, result.OperationRoutes, 2)
}

// TestUpdateTransactionRouteInvalidOperationRouteCount tests validation error for insufficient operation routes
func TestUpdateTransactionRouteInvalidOperationRouteCount(t *testing.T) {
	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Only 1 operation route instead of required minimum 2
	invalidOperationRouteInputs := []mmodel.OperationRouteActionInput{
		{Action: "direct", OperationRouteID: uuid.New()},
	}

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Title",
		OperationRoutes: &invalidOperationRouteInputs,
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedBusinessError, err)
}

// TestUpdateTransactionRouteWithoutOperationRoutes tests updating without changing operation routes (OperationRoutes = nil)
func TestUpdateTransactionRouteWithoutOperationRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Title",
		Description:     "Updated Description",
		OperationRoutes: nil,
		Metadata:        map[string]any{"key": "value"},
	}

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          input.Title,
		Description:    input.Description,
	}

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, lID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []mmodel.OperationRouteActionInput) (*mmodel.TransactionRoute, error) {
			assert.Empty(t, toAdd)
			assert.Empty(t, toRemove)
			return expectedTransactionRoute, nil
		}).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String(), input.Metadata).
		Return(nil).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedTransactionRoute.ID, result.ID)
}

// TestUpdateTransactionRouteInvalidOperationRouteTypes tests validation error for operation routes missing debit or credit
func TestUpdateTransactionRouteInvalidOperationRouteTypes(t *testing.T) {
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	opRouteID1 := uuid.New()
	opRouteID2 := uuid.New()

	operationRouteInputs := []mmodel.OperationRouteActionInput{
		{Action: "direct", OperationRouteID: opRouteID1},
		{Action: "direct", OperationRouteID: opRouteID2},
	}

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Title",
		OperationRoutes: &operationRouteInputs,
	}

	currentTransactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OperationRoutes: []mmodel.OperationRoute{},
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(currentTransactionRoute, nil).
		Times(1)

	operationRoutes := []*mmodel.OperationRoute{
		{ID: opRouteID1, OperationType: "source"},
		{ID: opRouteID2, OperationType: "source"}, // Both are source, missing destination
	}

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(operationRoutes, nil).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrNoDestinationForAction, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), "direct")
	assert.Equal(t, expectedBusinessError, err)
}

// TestUpdateTransactionRouteWithMultipleOperationRoutes tests updating with more than 2 operation routes
func TestUpdateTransactionRouteWithMultipleOperationRoutes(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	opID1 := uuid.Must(libCommons.GenerateUUIDv7())
	opID2 := uuid.Must(libCommons.GenerateUUIDv7())
	opID3 := uuid.Must(libCommons.GenerateUUIDv7())
	opID4 := uuid.Must(libCommons.GenerateUUIDv7())

	operationRouteInputs := []mmodel.OperationRouteActionInput{
		{Action: "direct", OperationRouteID: opID1},
		{Action: "direct", OperationRouteID: opID2},
		{Action: "direct", OperationRouteID: opID3},
		{Action: "direct", OperationRouteID: opID4},
	}

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Route",
		Description:     "Updated Description",
		OperationRoutes: &operationRouteInputs,
		Metadata:        map[string]any{"key": "value"},
	}

	currentTransactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OperationRoutes: []mmodel.OperationRoute{},
	}

	transactionRoute := &mmodel.TransactionRoute{
		ID:          transactionRouteID,
		Title:       input.Title,
		Description: input.Description,
		OperationRoutes: []mmodel.OperationRoute{
			{ID: opID1, OperationType: "source"},
			{ID: opID2, OperationType: "source"},
			{ID: opID3, OperationType: "destination"},
			{ID: opID4, OperationType: "destination"},
		},
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	operationRoutes := []*mmodel.OperationRoute{
		{ID: opID1, OperationType: "source"},
		{ID: opID2, OperationType: "source"},
		{ID: opID3, OperationType: "destination"},
		{ID: opID4, OperationType: "destination"},
	}

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(currentTransactionRoute, nil).
		Times(1)

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(operationRoutes, nil).
		Times(1)

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, lID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []mmodel.OperationRouteActionInput) (*mmodel.TransactionRoute, error) {
			assert.Len(t, toAdd, 4)
			assert.Empty(t, toRemove)
			return transactionRoute, nil
		}).
		Times(1)

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, nil).
		Times(1)

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		Update(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String(), input.Metadata).
		Return(nil).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, input.Title, result.Title)
	assert.Equal(t, input.Description, result.Description)
	assert.Len(t, result.OperationRoutes, 4)
}

// TestHandleOperationRouteUpdatesDiffsByActionTuples tests that the diff logic considers
// (routeID, action) composite keys rather than just routeID. The same operation route with different
// actions must be treated as distinct entries.
func TestHandleOperationRouteUpdatesDiffsByActionTuples(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	opRouteSource := libCommons.GenerateUUIDv7()
	opRouteDest := libCommons.GenerateUUIDv7()

	// Currently the transaction route has opRouteSource/direct, opRouteDest/direct
	currentTransactionRoute := &mmodel.TransactionRoute{
		ID: transactionRouteID,
		OperationRoutes: []mmodel.OperationRoute{
			{ID: opRouteSource, OperationType: "source", Action: "direct"},
			{ID: opRouteDest, OperationType: "destination", Action: "direct"},
		},
	}

	// New desired state: change action from "direct" to "hold" for both routes
	// This keeps per-action validation valid (hold has source + destination)
	newInputs := []mmodel.OperationRouteActionInput{
		{Action: "hold", OperationRouteID: opRouteSource},
		{Action: "hold", OperationRouteID: opRouteDest},
	}

	operationRoutes := []*mmodel.OperationRoute{
		{ID: opRouteSource, OperationType: "source"},
		{ID: opRouteDest, OperationType: "destination"},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(ctrl),
		OperationRouteRepo:   operationroute.NewMockRepository(ctrl),
		MetadataRepo:         mongodb.NewMockRepository(ctrl),
	}

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(currentTransactionRoute, nil).
		Times(1)

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(operationRoutes, nil).
		Times(1)

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, lID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []mmodel.OperationRouteActionInput) (*mmodel.TransactionRoute, error) {
			// The composite key diff detects that (opRouteSource, "direct") and (opRouteDest, "direct")
			// should be removed and (opRouteSource, "hold") and (opRouteDest, "hold") should be added.
			assert.Len(t, toAdd, 2, "expected 2 entries to add (both routes with hold action)")
			assert.Len(t, toRemove, 2, "expected 2 entries to remove (both routes with direct action)")

			addActions := make(map[uuid.UUID]string)
			for _, entry := range toAdd {
				addActions[entry.OperationRouteID] = entry.Action
			}

			assert.Equal(t, "hold", addActions[opRouteSource])
			assert.Equal(t, "hold", addActions[opRouteDest])

			removeActions := make(map[uuid.UUID]string)
			for _, entry := range toRemove {
				removeActions[entry.OperationRouteID] = entry.Action
			}

			assert.Equal(t, "direct", removeActions[opRouteSource])
			assert.Equal(t, "direct", removeActions[opRouteDest])

			return &mmodel.TransactionRoute{
				ID:          transactionRouteID,
				Title:       "Updated",
				Description: "Updated",
			}, nil
		}).
		Times(1)

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, nil).
		Times(1)

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		Update(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String(), gomock.Any()).
		Return(nil).
		Times(1)

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated",
		Description:     "Updated",
		OperationRoutes: &newInputs,
		Metadata:        map[string]any{"key": "value"},
	}

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestHandleOperationRouteUpdatesSameRouteMultipleActions tests that the same operation route ID
// can appear with different actions and all are preserved as distinct (routeID, action) tuples.
func TestHandleOperationRouteUpdatesSameRouteMultipleActions(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	opRouteSource := libCommons.GenerateUUIDv7()
	opRouteDest := libCommons.GenerateUUIDv7()

	// Current state: no operation routes
	currentTransactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OperationRoutes: []mmodel.OperationRoute{},
	}

	// New desired state: opRouteSource with "direct" AND "hold", opRouteDest with "direct" AND "hold"
	newInputs := []mmodel.OperationRouteActionInput{
		{Action: "direct", OperationRouteID: opRouteSource},
		{Action: "hold", OperationRouteID: opRouteSource},
		{Action: "direct", OperationRouteID: opRouteDest},
		{Action: "hold", OperationRouteID: opRouteDest},
	}

	operationRoutes := []*mmodel.OperationRoute{
		{ID: opRouteSource, OperationType: "source"},
		{ID: opRouteDest, OperationType: "destination"},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(ctrl),
		OperationRouteRepo:   operationroute.NewMockRepository(ctrl),
		MetadataRepo:         mongodb.NewMockRepository(ctrl),
	}

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(currentTransactionRoute, nil).
		Times(1)

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(operationRoutes, nil).
		Times(1)

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, lID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []mmodel.OperationRouteActionInput) (*mmodel.TransactionRoute, error) {
			// All 4 composite (routeID, action) tuples should be added (none existed before).
			assert.Len(t, toAdd, 4, "expected 4 entries to add (2 routes x 2 actions each)")
			assert.Empty(t, toRemove)

			return &mmodel.TransactionRoute{
				ID:          transactionRouteID,
				Title:       "Multi-Action Route",
				Description: "Route with multiple actions",
			}, nil
		}).
		Times(1)

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, nil).
		Times(1)

	uc.MetadataRepo.(*mongodb.MockRepository).
		EXPECT().
		Update(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String(), gomock.Any()).
		Return(nil).
		Times(1)

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Multi-Action Route",
		Description:     "Route with multiple actions",
		OperationRoutes: &newInputs,
		Metadata:        map[string]any{"key": "value"},
	}

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestUpdateTransactionRouteEmptyOperationRoutes tests validation error for empty operation routes array
func TestUpdateTransactionRouteEmptyOperationRoutes(t *testing.T) {
	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	emptyOperationRouteInputs := []mmodel.OperationRouteActionInput{}

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Title",
		OperationRoutes: &emptyOperationRouteInputs,
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedBusinessError, err)
}

// TestHandleOperationRouteUpdates_ErrorPaths tests error handling in handleOperationRouteUpdates
// using table-driven tests for FindByID and FindByIDs failures.
func TestHandleOperationRouteUpdates_ErrorPaths(t *testing.T) {
	t.Parallel()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	opRouteID1 := libCommons.GenerateUUIDv7()
	opRouteID2 := libCommons.GenerateUUIDv7()

	validInputs := []mmodel.OperationRouteActionInput{
		{Action: "direct", OperationRouteID: opRouteID1},
		{Action: "direct", OperationRouteID: opRouteID2},
	}

	tests := []struct {
		name         string
		setupMocks   func(ctrl *gomock.Controller) (*transactionroute.MockRepository, *operationroute.MockRepository)
		errContains  string
		expectNilAdd bool
		expectNilRem bool
	}{
		{
			name: "FindByID_returns_error_propagates_to_caller",
			setupMocks: func(ctrl *gomock.Controller) (*transactionroute.MockRepository, *operationroute.MockRepository) {
				mockTR := transactionroute.NewMockRepository(ctrl)
				mockOR := operationroute.NewMockRepository(ctrl)

				mockTR.EXPECT().
					FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
					Return(nil, errors.New("connection refused")).
					Times(1)

				return mockTR, mockOR
			},
			errContains:  "connection refused",
			expectNilAdd: true,
			expectNilRem: true,
		},
		{
			name: "FindByIDs_returns_error_propagates_to_caller",
			setupMocks: func(ctrl *gomock.Controller) (*transactionroute.MockRepository, *operationroute.MockRepository) {
				mockTR := transactionroute.NewMockRepository(ctrl)
				mockOR := operationroute.NewMockRepository(ctrl)

				mockTR.EXPECT().
					FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
					Return(&mmodel.TransactionRoute{
						ID:              transactionRouteID,
						OperationRoutes: []mmodel.OperationRoute{},
					}, nil).
					Times(1)

				mockOR.EXPECT().
					FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
					Return(nil, errors.New("timeout fetching operation routes")).
					Times(1)

				return mockTR, mockOR
			},
			errContains:  "timeout fetching operation routes",
			expectNilAdd: true,
			expectNilRem: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTR, mockOR := tt.setupMocks(ctrl)

			uc := UseCase{
				TransactionRouteRepo: mockTR,
				OperationRouteRepo:   mockOR,
				MetadataRepo:         mongodb.NewMockRepository(ctrl),
			}

			toAdd, toRemove, err := uc.handleOperationRouteUpdates(
				context.Background(), organizationID, ledgerID, transactionRouteID, validInputs,
			)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)

			if tt.expectNilAdd {
				assert.Nil(t, toAdd)
			}

			if tt.expectNilRem {
				assert.Nil(t, toRemove)
			}
		})
	}
}

// TestHandleOperationRouteUpdates_DiffScenarios tests edge cases in the composite key
// diff logic using table-driven tests for various add/remove scenarios.
func TestHandleOperationRouteUpdates_DiffScenarios(t *testing.T) {
	t.Parallel()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	opRouteSource := libCommons.GenerateUUIDv7()
	opRouteDest := libCommons.GenerateUUIDv7()

	tests := []struct {
		name              string
		existingRoutes    []mmodel.OperationRoute
		newInputs         []mmodel.OperationRouteActionInput
		fetchedOpRoutes   []*mmodel.OperationRoute
		expectedAddLen    int
		expectedRemoveLen int
	}{
		{
			name: "no_changes_when_existing_and_new_match_exactly",
			existingRoutes: []mmodel.OperationRoute{
				{ID: opRouteSource, OperationType: "source", Action: "direct"},
				{ID: opRouteDest, OperationType: "destination", Action: "direct"},
			},
			newInputs: []mmodel.OperationRouteActionInput{
				{Action: "direct", OperationRouteID: opRouteSource},
				{Action: "direct", OperationRouteID: opRouteDest},
			},
			fetchedOpRoutes: []*mmodel.OperationRoute{
				{ID: opRouteSource, OperationType: "source"},
				{ID: opRouteDest, OperationType: "destination"},
			},
			expectedAddLen:    0,
			expectedRemoveLen: 0,
		},
		{
			name: "remove_all_existing_and_add_all_new",
			existingRoutes: []mmodel.OperationRoute{
				{ID: opRouteSource, OperationType: "source", Action: "hold"},
				{ID: opRouteDest, OperationType: "destination", Action: "hold"},
			},
			newInputs: []mmodel.OperationRouteActionInput{
				{Action: "direct", OperationRouteID: opRouteSource},
				{Action: "direct", OperationRouteID: opRouteDest},
			},
			fetchedOpRoutes: []*mmodel.OperationRoute{
				{ID: opRouteSource, OperationType: "source"},
				{ID: opRouteDest, OperationType: "destination"},
			},
			expectedAddLen:    2,
			expectedRemoveLen: 2,
		},
		{
			name: "add_new_action_for_existing_route_without_removing_existing",
			existingRoutes: []mmodel.OperationRoute{
				{ID: opRouteSource, OperationType: "source", Action: "direct"},
				{ID: opRouteDest, OperationType: "destination", Action: "direct"},
			},
			newInputs: []mmodel.OperationRouteActionInput{
				{Action: "direct", OperationRouteID: opRouteSource},
				{Action: "hold", OperationRouteID: opRouteSource},
				{Action: "direct", OperationRouteID: opRouteDest},
				{Action: "hold", OperationRouteID: opRouteDest},
			},
			fetchedOpRoutes: []*mmodel.OperationRoute{
				{ID: opRouteSource, OperationType: "source"},
				{ID: opRouteDest, OperationType: "destination"},
			},
			expectedAddLen:    2,
			expectedRemoveLen: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTR := transactionroute.NewMockRepository(ctrl)
			mockOR := operationroute.NewMockRepository(ctrl)

			mockTR.EXPECT().
				FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
				Return(&mmodel.TransactionRoute{
					ID:              transactionRouteID,
					OperationRoutes: tt.existingRoutes,
				}, nil).
				Times(1)

			mockOR.EXPECT().
				FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
				Return(tt.fetchedOpRoutes, nil).
				Times(1)

			uc := UseCase{
				TransactionRouteRepo: mockTR,
				OperationRouteRepo:   mockOR,
				MetadataRepo:         mongodb.NewMockRepository(ctrl),
			}

			toAdd, toRemove, err := uc.handleOperationRouteUpdates(
				context.Background(), organizationID, ledgerID, transactionRouteID, tt.newInputs,
			)

			assert.NoError(t, err)
			assert.Len(t, toAdd, tt.expectedAddLen, "unexpected toAdd count")
			assert.Len(t, toRemove, tt.expectedRemoveLen, "unexpected toRemove count")
		})
	}
}

// TestHandleOperationRouteUpdates_SoftDeletePreserved verifies that when routes are removed,
// the entries with both routeID and action are passed to toRemove (which triggers action-aware
// soft-delete via SET deleted_at = NOW() with composite WHERE in the repository).
func TestHandleOperationRouteUpdates_SoftDeletePreserved(t *testing.T) {
	t.Parallel()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	opRouteToKeep := libCommons.GenerateUUIDv7()
	opRouteToRemove := libCommons.GenerateUUIDv7()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTR := transactionroute.NewMockRepository(ctrl)
	mockOR := operationroute.NewMockRepository(ctrl)

	// Existing state: two routes with direct action
	mockTR.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(&mmodel.TransactionRoute{
			ID: transactionRouteID,
			OperationRoutes: []mmodel.OperationRoute{
				{ID: opRouteToKeep, OperationType: "source", Action: "direct"},
				{ID: opRouteToRemove, OperationType: "destination", Action: "direct"},
			},
		}, nil).
		Times(1)

	// New desired state: only keep the source route, replace destination
	newDestID := libCommons.GenerateUUIDv7()

	newInputs := []mmodel.OperationRouteActionInput{
		{Action: "direct", OperationRouteID: opRouteToKeep},
		{Action: "direct", OperationRouteID: newDestID},
	}

	mockOR.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return([]*mmodel.OperationRoute{
			{ID: opRouteToKeep, OperationType: "source"},
			{ID: newDestID, OperationType: "destination"},
		}, nil).
		Times(1)

	uc := UseCase{
		TransactionRouteRepo: mockTR,
		OperationRouteRepo:   mockOR,
		MetadataRepo:         mongodb.NewMockRepository(ctrl),
	}

	toAdd, toRemove, err := uc.handleOperationRouteUpdates(
		context.Background(), organizationID, ledgerID, transactionRouteID, newInputs,
	)

	assert.NoError(t, err)

	// opRouteToRemove should be in toRemove with action "direct" (will be soft-deleted by repo)
	assert.Len(t, toRemove, 1)
	assert.Equal(t, opRouteToRemove, toRemove[0].OperationRouteID, "removed route should be in toRemove for soft-delete")
	assert.Equal(t, "direct", toRemove[0].Action, "removed route should carry its action")

	// new destination should be in toAdd with action "direct"
	assert.Len(t, toAdd, 1)
	assert.Equal(t, newDestID, toAdd[0].OperationRouteID, "new route should be in toAdd")
	assert.Equal(t, "direct", toAdd[0].Action, "new route should carry its action")
}
