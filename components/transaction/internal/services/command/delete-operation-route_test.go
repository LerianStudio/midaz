package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
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
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(false, nil).
		Times(1)

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
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(false, nil).
		Times(1)

	notFoundErr := &pkg.EntityNotFoundError{
		EntityType: "OperationRoute",
		Code:       constant.ErrOperationRouteNotFound.Error(),
		Title:      "Operation Route Not Found",
		Message:    "The provided operation route does not exist in our records. Please verify the operation route and try again.",
	}

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(notFoundErr).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.Error(t, err)

	// Check if it's the proper business error
	var entityNotFoundError *pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, constant.ErrOperationRouteNotFound.Error(), entityNotFoundError.Code)
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
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(databaseError).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.Error(t, err)
	assert.ErrorIs(t, err, databaseError)
}

// TestDeleteOperationRouteByIDLinkedToTransactionRoutes tests deletion when operation route is linked to transaction routes
func TestDeleteOperationRouteByIDLinkedToTransactionRoutes(t *testing.T) {
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
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(true, nil).
		Times(1)

	// Delete should not be called since operation route is linked
	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Times(0)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.Error(t, err)

	// Check if it's the proper business error for linked operation routes
	var unprocessableOperationError pkg.UnprocessableOperationError
	assert.True(t, errors.As(err, &unprocessableOperationError))
	assert.Equal(t, "0107", unprocessableOperationError.Code)
}

// TestDeleteOperationRouteByIDHasLinksCheckError tests deletion when checking for links fails
func TestDeleteOperationRouteByIDHasLinksCheckError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	linkCheckError := errors.New("failed to check transaction route links")

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(false, linkCheckError).
		Times(1)

	// Delete should not be called since link check failed
	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Times(0)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.Error(t, err)
	assert.ErrorIs(t, err, linkCheckError)
}
