package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetTransactionRouteByIDSuccess tests getting a transaction route by ID successfully with metadata
func TestGetTransactionRouteByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Transaction Route",
		Description:    "Test Description",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            uuid.New(),
				OperationType: "source",
			},
		},
	}

	expectedMetadata := &mongodb.Metadata{
		EntityID:   transactionRouteID.String(),
		EntityName: reflect.TypeOf(mmodel.TransactionRoute{}).Name(),
		Data:       mongodb.JSON{"key": "value"},
	}

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(expectedTransactionRoute, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(expectedMetadata, nil).
		Times(1)

	result, err := uc.GetTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedTransactionRoute.ID, result.ID)
	assert.Equal(t, expectedTransactionRoute.Title, result.Title)
	assert.Equal(t, map[string]any{"key": "value"}, result.Metadata)
}

// TestGetTransactionRouteByIDSuccessWithoutMetadata tests getting a transaction route by ID successfully without metadata
func TestGetTransactionRouteByIDSuccessWithoutMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Transaction Route",
		Description:    "Test Description",
	}

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(expectedTransactionRoute, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, nil).
		Times(1)

	result, err := uc.GetTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedTransactionRoute.ID, result.ID)
	assert.Equal(t, expectedTransactionRoute.Title, result.Title)
	assert.Nil(t, result.Metadata)
}

// TestGetTransactionRouteByIDErrorTransactionRouteRepo tests getting a transaction route by ID with transaction route repository error
func TestGetTransactionRouteByIDErrorTransactionRouteRepo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	expectedError := errors.New("database error")

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.GetTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

// TestGetTransactionRouteByIDNotFound tests getting a transaction route by ID when not found returns business error
func TestGetTransactionRouteByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.GetTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)

	// Should return business error for transaction route not found
	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedBusinessError, err)
	assert.Nil(t, result)
}

// TestGetTransactionRouteByIDErrorMetadataRepo tests getting a transaction route by ID with metadata repository error
func TestGetTransactionRouteByIDErrorMetadataRepo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	metadataError := errors.New("metadata database error")

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Transaction Route",
		Description:    "Test Description",
	}

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(expectedTransactionRoute, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRouteID.String()).
		Return(nil, metadataError).
		Times(1)

	result, err := uc.GetTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, metadataError, err)
	assert.Nil(t, result)
}

// TestGetTransactionRouteByIDNilTransactionRoute tests getting a transaction route by ID when transaction route is nil
func TestGetTransactionRouteByIDNilTransactionRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, nil).
		Times(1)

	// Metadata should not be called when transaction route is nil

	result, err := uc.GetTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
	assert.Nil(t, result)
}
