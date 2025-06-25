package command

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

// TestCreateOperationRouteSuccess is responsible to test CreateOperationRoute with success
func TestCreateOperationRouteSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:       "Test Operation Route",
		Description: "Test Description",
		Type:        "debit",
	}

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		Type:           payload.Type,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedOperationRoute, nil).
		Times(1)

	result, err := uc.CreateOperationRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Equal(t, expectedOperationRoute, result)
	assert.Nil(t, err)
}

// TestCreateOperationRouteError is responsible to test CreateOperationRoute with error
func TestCreateOperationRouteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	errMSG := "err to create OperationRoute on database"
	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:       "Test Operation Route",
		Description: "Test Description",
		Type:        "debit",
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)

	result, err := uc.CreateOperationRoute(context.Background(), organizationID, ledgerID, payload)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, result)
}
