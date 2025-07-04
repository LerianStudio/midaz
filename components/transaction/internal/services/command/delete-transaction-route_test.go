package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteTransactionRouteByIDSuccess tests successful deletion of a transaction route
func TestDeleteTransactionRouteByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()
	operationRouteID2 := uuid.New()

	mockRepo := transactionroute.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockRepo,
	}

	transactionRoute := &mmodel.TransactionRoute{
		ID: transactionRouteID,
		OperationRoutes: []mmodel.OperationRoute{
			{ID: operationRouteID1},
			{ID: operationRouteID2},
		},
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.DeleteTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
}

// TestDeleteTransactionRouteByIDNotFoundOnFind tests deletion when transaction route is not found during find
func TestDeleteTransactionRouteByIDNotFoundOnFind(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockRepo := transactionroute.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	err := uc.DeleteTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)

	var businessError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &businessError))
	assert.Equal(t, "0101", businessError.Code)
}

// TestDeleteTransactionRouteByIDFindError tests deletion with database error during find
func TestDeleteTransactionRouteByIDFindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	databaseError := errors.New("database connection error")

	mockRepo := transactionroute.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, databaseError).
		Times(1)

	err := uc.DeleteTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
}

// TestDeleteTransactionRouteByIDDeleteError tests deletion with database error during delete
func TestDeleteTransactionRouteByIDDeleteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID := uuid.New()
	databaseError := errors.New("database deletion error")

	mockRepo := transactionroute.NewMockRepository(ctrl)
	uc := &UseCase{
		TransactionRouteRepo: mockRepo,
	}

	transactionRoute := &mmodel.TransactionRoute{
		ID: transactionRouteID,
		OperationRoutes: []mmodel.OperationRoute{
			{ID: operationRouteID},
		},
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, transactionRouteID, gomock.Any()).
		Return(databaseError).
		Times(1)

	err := uc.DeleteTransactionRouteByID(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
}
