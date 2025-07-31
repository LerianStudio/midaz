package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
)

// TestGetOperationRouteByIDSuccess tests getting an operation route by ID successfully with metadata
func TestGetOperationRouteByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test Description",
		OperationType:  "source",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(expectedOperationRoute, nil).
		Times(1)

	expectedMetadata := &mongodb.Metadata{
		ID:       primitive.NewObjectID(),
		EntityID: operationRouteID.String(),
		Data:     map[string]any{"key": "value", "type": "important"},
	}

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRouteID.String()).
		Return(expectedMetadata, nil).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.Background(), organizationID, ledgerID, nil, operationRouteID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, operationRouteID, result.ID)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "Test Route", result.Title)
	assert.Equal(t, "Test Description", result.Description)
	assert.Equal(t, "source", result.OperationType)
	assert.Equal(t, map[string]any{"key": "value", "type": "important"}, result.Metadata)
}

// TestGetOperationRouteByIDSuccessWithoutMetadata tests getting an operation route by ID successfully without metadata
func TestGetOperationRouteByIDSuccessWithoutMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test Description",
		OperationType:  "destination",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(expectedOperationRoute, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRouteID.String()).
		Return(nil, nil).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.Background(), organizationID, ledgerID, nil, operationRouteID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, operationRouteID, result.ID)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "Test Route", result.Title)
	assert.Equal(t, "Test Description", result.Description)
	assert.Equal(t, "destination", result.OperationType)
	assert.Nil(t, result.Metadata)
}

// TestGetOperationRouteByIDError tests getting an operation route by ID with database error
func TestGetOperationRouteByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	expectedError := errors.New("database error")

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.Background(), organizationID, ledgerID, nil, operationRouteID)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

// TestGetOperationRouteByIDNotFound tests getting an operation route by ID when not found (ErrDatabaseItemNotFound)
func TestGetOperationRouteByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.Background(), organizationID, ledgerID, nil, operationRouteID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "The provided operation route does not exist in our records")
}

// TestGetOperationRouteByIDMetadataError tests getting an operation route by ID with metadata error
func TestGetOperationRouteByIDMetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	metadataError := errors.New("metadata repository error")

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		Description:    "Test Description",
		OperationType:  "source",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(expectedOperationRoute, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRouteID.String()).
		Return(nil, metadataError).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.Background(), organizationID, ledgerID, nil, operationRouteID)

	assert.Error(t, err)
	assert.Equal(t, metadataError, err)
	assert.Nil(t, result)
}

// TestGetOperationRouteByIDWithPortfolioID tests getting an operation route by ID with portfolio ID parameter
func TestGetOperationRouteByIDWithPortfolioID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Portfolio Route",
		Description:    "Portfolio Description",
		OperationType:  "destination",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(expectedOperationRoute, nil).
		Times(1)

	expectedMetadata := &mongodb.Metadata{
		ID:       primitive.NewObjectID(),
		EntityID: operationRouteID.String(),
		Data:     map[string]any{"portfolio": "specific", "category": "premium"},
	}

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRouteID.String()).
		Return(expectedMetadata, nil).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.Background(), organizationID, ledgerID, &portfolioID, operationRouteID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, operationRouteID, result.ID)
	assert.Equal(t, "Portfolio Route", result.Title)
	assert.Equal(t, map[string]any{"portfolio": "specific", "category": "premium"}, result.Metadata)
}
