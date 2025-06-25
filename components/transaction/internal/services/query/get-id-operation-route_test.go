package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetOperationRouteByIDSuccess is responsible to test GetOperationRouteByID with success
func TestGetOperationRouteByIDSuccess(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID := uuid.New()
	now := time.Now()

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Operation Route",
		Description:    "Test Description",
		Type:           "debit",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	uc := UseCase{
		OperationRouteRepo: operationroute.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(expectedOperationRoute, nil).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.TODO(), organizationID, ledgerID, nil, operationRouteID)

	assert.Equal(t, expectedOperationRoute, result)
	assert.Nil(t, err)
}

// TestGetOperationRouteByIDError is responsible to test GetOperationRouteByID with database error
func TestGetOperationRouteByIDError(t *testing.T) {
	errMSG := "database connection error"
	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID := uuid.New()

	uc := UseCase{
		OperationRouteRepo: operationroute.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(nil, errors.New(errMSG)).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.TODO(), organizationID, ledgerID, nil, operationRouteID)

	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, result)
}

// TestGetOperationRouteByIDNotFound is responsible to test GetOperationRouteByID with not found error
func TestGetOperationRouteByIDNotFound(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID := uuid.New()

	expectedError := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, "OperationRoute")

	uc := UseCase{
		OperationRouteRepo: operationroute.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.GetOperationRouteByID(context.TODO(), organizationID, ledgerID, nil, operationRouteID)

	assert.NotNil(t, err)
	assert.Equal(t, expectedError.Error(), err.Error())
	assert.Nil(t, result)

	// Verify the error is of the correct type
	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0101", entityNotFoundError.Code)
	assert.Equal(t, "Operation Route Not Found", entityNotFoundError.Title)
}
