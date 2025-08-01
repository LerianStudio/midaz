package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
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

	transactionRouteID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

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
		DoAndReturn(func(ctx context.Context, orgID, ledgerID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []uuid.UUID) (*mmodel.TransactionRoute, error) {
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

	transactionRouteID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

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

	transactionRouteID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

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

	transactionRouteID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

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
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	input := &mmodel.UpdateTransactionRouteInput{
		Title:       "Updated Route",
		Description: "Updated Description",
		OperationRoutes: &[]uuid.UUID{
			uuid.New(), // debit
			uuid.New(), // credit
		},
		Metadata: map[string]any{"key": "value"},
	}

	currentTransactionRoute := &mmodel.TransactionRoute{
		ID: transactionRouteID,
		OperationRoutes: []mmodel.OperationRoute{
			{ID: libCommons.GenerateUUIDv7(), OperationType: "source"},
			{ID: libCommons.GenerateUUIDv7(), OperationType: "destination"},
		},
	}

	transactionRoute := &mmodel.TransactionRoute{
		ID:          transactionRouteID,
		Title:       input.Title,
		Description: input.Description,
		OperationRoutes: []mmodel.OperationRoute{
			{ID: (*input.OperationRoutes)[0], OperationType: "source"},
			{ID: (*input.OperationRoutes)[1], OperationType: "destination"},
		},
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	operationRoutes := []*mmodel.OperationRoute{
		{ID: (*input.OperationRoutes)[0], OperationType: "source"},
		{ID: (*input.OperationRoutes)[1], OperationType: "destination"},
	}

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(currentTransactionRoute, nil).
		Times(1)

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, *input.OperationRoutes).
		Return(operationRoutes, nil).
		Times(1)

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, lID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []uuid.UUID) (*mmodel.TransactionRoute, error) {
			assert.Len(t, toAdd, 2)
			assert.Len(t, toRemove, 2)
			assert.Contains(t, toAdd, (*input.OperationRoutes)[0])
			assert.Contains(t, toAdd, (*input.OperationRoutes)[1])
			assert.Contains(t, toRemove, currentTransactionRoute.OperationRoutes[0].ID)
			assert.Contains(t, toRemove, currentTransactionRoute.OperationRoutes[1].ID)
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
	invalidOperationRouteIDs := []uuid.UUID{uuid.New()}

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Title",
		OperationRoutes: &invalidOperationRouteIDs,
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	// No repository expectations since validation should fail early

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	// Should return business error for insufficient operation routes
	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedBusinessError, err)
}

// TestUpdateTransactionRouteWithoutOperationRoutes tests updating without changing operation routes (OperationRoutes = nil)
func TestUpdateTransactionRouteWithoutOperationRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

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
		DoAndReturn(func(ctx context.Context, orgID, lID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []uuid.UUID) (*mmodel.TransactionRoute, error) {
			// Should have empty arrays since no operation route updates requested
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
	transactionRouteID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	operationRouteIDs := []uuid.UUID{uuid.New(), uuid.New()}

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Title",
		OperationRoutes: &operationRouteIDs,
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
		{ID: operationRouteIDs[0], OperationType: "source"},
		{ID: operationRouteIDs[1], OperationType: "source"}, // Both are source, missing destination
	}

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, operationRouteIDs).
		Return(operationRoutes, nil).
		Times(1)

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedBusinessError, err)
}

// TestUpdateTransactionRouteWithMultipleOperationRoutes tests updating with more than 2 operation routes
func TestUpdateTransactionRouteWithMultipleOperationRoutes(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	operationRouteIDs := []uuid.UUID{libCommons.GenerateUUIDv7(), libCommons.GenerateUUIDv7(), libCommons.GenerateUUIDv7(), libCommons.GenerateUUIDv7()}

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Route",
		Description:     "Updated Description",
		OperationRoutes: &operationRouteIDs,
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
			{ID: operationRouteIDs[0], OperationType: "source"},
			{ID: operationRouteIDs[1], OperationType: "source"},
			{ID: operationRouteIDs[2], OperationType: "destination"},
			{ID: operationRouteIDs[3], OperationType: "destination"},
		},
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	operationRoutes := []*mmodel.OperationRoute{
		{ID: operationRouteIDs[0], OperationType: "source"},
		{ID: operationRouteIDs[1], OperationType: "source"},
		{ID: operationRouteIDs[2], OperationType: "destination"},
		{ID: operationRouteIDs[3], OperationType: "destination"},
	}

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(currentTransactionRoute, nil).
		Times(1)

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, operationRouteIDs).
		Return(operationRoutes, nil).
		Times(1)

	uc.TransactionRouteRepo.(*transactionroute.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, lID, id uuid.UUID, tr *mmodel.TransactionRoute, toAdd, toRemove []uuid.UUID) (*mmodel.TransactionRoute, error) {
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

// TestUpdateTransactionRouteEmptyOperationRoutes tests validation error for empty operation routes array
func TestUpdateTransactionRouteEmptyOperationRoutes(t *testing.T) {
	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Empty operation routes array
	emptyOperationRouteIDs := []uuid.UUID{}

	input := &mmodel.UpdateTransactionRouteInput{
		Title:           "Updated Title",
		OperationRoutes: &emptyOperationRouteIDs,
	}

	uc := UseCase{
		TransactionRouteRepo: transactionroute.NewMockRepository(gomock.NewController(t)),
		OperationRouteRepo:   operationroute.NewMockRepository(gomock.NewController(t)),
		MetadataRepo:         mongodb.NewMockRepository(gomock.NewController(t)),
	}

	// No repository expectations since validation should fail early

	result, err := uc.UpdateTransactionRoute(context.Background(), organizationID, ledgerID, transactionRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	// Should return business error for insufficient operation routes
	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedBusinessError, err)
}
