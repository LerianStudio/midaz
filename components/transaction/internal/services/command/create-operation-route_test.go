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
		Title:        "Test Operation Route",
		Description:  "Test Description",
		Type:         "debit",
		AccountTypes: []string{"asset", "liability"},
		AccountAlias: "@cash_account",
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, payload.Title, operationRoute.Title)
			assert.Equal(t, payload.Description, operationRoute.Description)
			assert.Equal(t, payload.Type, operationRoute.Type)
			assert.Equal(t, payload.AccountTypes, operationRoute.AccountTypes)
			assert.Equal(t, payload.AccountAlias, operationRoute.AccountAlias)
			return operationRoute, nil
		})

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.CreateOperationRoute(context.Background(), organizationID, ledgerID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, payload.Title, operationRoute.Title)
	assert.Equal(t, payload.Description, operationRoute.Description)
	assert.Equal(t, payload.Type, operationRoute.Type)
	assert.Equal(t, payload.AccountTypes, operationRoute.AccountTypes)
	assert.Equal(t, payload.AccountAlias, operationRoute.AccountAlias)
}

// TestCreateOperationRouteWithEmptyAccountTypes tests creating an operation route with empty account types
func TestCreateOperationRouteWithEmptyAccountTypes(t *testing.T) {
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

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:        "Test Operation Route",
		Description:  "Test Description",
		Type:         "debit",
		AccountTypes: []string{"asset", "liability"},
		AccountAlias: "@cash_account",
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, errors.New("failed to create operation route"))

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.CreateOperationRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Nil(t, operationRoute)
}
