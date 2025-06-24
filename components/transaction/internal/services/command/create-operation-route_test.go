package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateOperationRouteSuccess is responsible to test CreateOperationRoute with success
func TestCreateOperationRouteSuccess(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:       "Test Operation Route",
		Description: "Test Description",
		Type:        "test-type",
	}

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:          uuid.New(),
		Title:       payload.Title,
		Description: payload.Description,
		Type:        payload.Type,
	}

	uc := UseCase{
		OperationRouteRepo: operationroute.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(expectedOperationRoute, nil).
		Times(1)

	result, err := uc.CreateOperationRoute(context.TODO(), organizationID, ledgerID, payload)

	assert.Equal(t, expectedOperationRoute, result)
	assert.Nil(t, err)
}

// TestCreateOperationRouteError is responsible to test CreateOperationRoute with error
func TestCreateOperationRouteError(t *testing.T) {
	errMSG := "err to create OperationRoute on database"
	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:       "Test Operation Route",
		Description: "Test Description",
		Type:        "test-type",
	}

	uc := UseCase{
		OperationRouteRepo: operationroute.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)

	result, err := uc.CreateOperationRoute(context.TODO(), organizationID, ledgerID, payload)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, result)
}

// TestCreateOperationRouteDuplicateTitleTypeError is responsible to test CreateOperationRoute with duplicate title and type error
func TestCreateOperationRouteDuplicateTitleTypeError(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:       "Test Operation Route",
		Description: "Test Description",
		Type:        "test-type",
	}

	expectedError := pkg.ValidateBusinessError(constant.ErrOperationRouteTitleAlreadyExists, "OperationRoute")

	uc := UseCase{
		OperationRouteRepo: operationroute.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRouteRepo.(*operationroute.MockRepository).
		EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.CreateOperationRoute(context.TODO(), organizationID, ledgerID, payload)

	assert.NotNil(t, err)
	assert.Equal(t, expectedError.Error(), err.Error())
	assert.Nil(t, result)

	// Verify the error is of the correct type
	var entityConflictError pkg.EntityConflictError
	assert.True(t, errors.As(err, &entityConflictError))
	assert.Equal(t, "0100", entityConflictError.Code)
	assert.Equal(t, "Operation Route Title Already Exists", entityConflictError.Title)
}
