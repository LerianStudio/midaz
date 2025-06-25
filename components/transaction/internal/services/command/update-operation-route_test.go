package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateOperationRouteSuccess tests updating an operation route successfully
func TestUpdateOperationRouteSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:       "Updated Title",
		Description: "Updated Description",
	}

	updatedRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          input.Title,
		Description:    input.Description,
		Type:           "debit",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(updatedRoute, nil).
		Times(1)

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedRoute, result)
}

// TestUpdateOperationRouteNotFound tests when operation route is not found during update
func TestUpdateOperationRouteNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title: "Updated Title",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.Error(t, err)

	// Check if it's the proper business error
	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0101", entityNotFoundError.Code)
	assert.Nil(t, result)
}

// TestUpdateOperationRouteUpdateError tests error during update operation
func TestUpdateOperationRouteUpdateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	updateError := errors.New("database connection error")

	input := &mmodel.UpdateOperationRouteInput{
		Title: "Updated Title",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(nil, updateError).
		Times(1)

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.Error(t, err)
	assert.Equal(t, updateError, err)
	assert.Nil(t, result)
}

// TestUpdateOperationRoutePartialUpdate tests partial update with only description
func TestUpdateOperationRoutePartialUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Description: "Updated Description Only",
	}

	updatedRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "", // Title not provided in input
		Description:    input.Description,
		Type:           "debit",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(updatedRoute, nil).
		Times(1)

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedRoute, result)
}
