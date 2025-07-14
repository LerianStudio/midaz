package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetOperationRouteByIDSuccess tests getting an operation route by ID successfully
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
		Type:           "debit",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(expectedOperationRoute, nil).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.Background(), organizationID, ledgerID, nil, operationRouteID)

	assert.NoError(t, err)
	assert.Equal(t, expectedOperationRoute, result)
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
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
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

// TestGetOperationRouteByIDNotFound tests getting an operation route by ID when not found
func TestGetOperationRouteByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(nil, nil).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.Background(), organizationID, ledgerID, nil, operationRouteID)

	assert.NoError(t, err)
	assert.Nil(t, result)
}
