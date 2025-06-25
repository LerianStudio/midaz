package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteOperationRouteByIDSuccess tests successful deletion of an operation route
func TestDeleteOperationRouteByIDSuccess(t *testing.T) {
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
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(nil).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.NoError(t, err)
}

// TestDeleteOperationRouteByIDNotFound tests deletion when operation route is not found
func TestDeleteOperationRouteByIDNotFound(t *testing.T) {
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
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(services.ErrDatabaseItemNotFound).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.Error(t, err)

	// Check if it's the proper business error
	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0101", entityNotFoundError.Code)
}

// TestDeleteOperationRouteByIDError tests deletion with database error
func TestDeleteOperationRouteByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	databaseError := errors.New("database connection error")

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(databaseError).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
}
