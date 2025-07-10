package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateTransactionRouteSuccess tests successful transaction route creation
func TestCreateTransactionRouteSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()
	operationRouteID2 := uuid.New()

	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		Description:     "Test Description",
		OperationRoutes: []uuid.UUID{operationRouteID1, operationRouteID2},
		Metadata:        map[string]any{"key": "value"},
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:             operationRouteID1,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Debit Route",
			OperationType:  "source",
		},
		{
			ID:             operationRouteID2,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Title:          "Credit Route",
			OperationType:  "destination",
		},
	}

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		OperationRoutes: []mmodel.OperationRoute{
			*expectedOperationRoutes[0],
			*expectedOperationRoutes[1],
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, payload.OperationRoutes).
		Return(expectedOperationRoutes, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedTransactionRoute, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(nil).
		Times(1)

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, ledgerID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedTransactionRoute.Title, result.Title)
	assert.Equal(t, expectedTransactionRoute.Description, result.Description)
	assert.Equal(t, len(expectedOperationRoutes), len(result.OperationRoutes))
	assert.Equal(t, payload.Metadata, result.Metadata)
}

// TestCreateTransactionRouteSuccessWithoutMetadata tests successful creation without metadata
func TestCreateTransactionRouteSuccessWithoutMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()
	operationRouteID2 := uuid.New()

	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		Description:     "Test Description",
		OperationRoutes: []uuid.UUID{operationRouteID1, operationRouteID2},
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:            operationRouteID1,
			OperationType: "source",
		},
		{
			ID:            operationRouteID2,
			OperationType: "destination",
		},
	}

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:          uuid.New(),
		Title:       payload.Title,
		Description: payload.Description,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, payload.OperationRoutes).
		Return(expectedOperationRoutes, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedTransactionRoute, nil).
		Times(1)

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, ledgerID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedTransactionRoute.Title, result.Title)
}

// TestCreateTransactionRouteErrorOperationRoutesNotFound tests error when operation routes are not found
func TestCreateTransactionRouteErrorOperationRoutesNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()

	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		OperationRoutes: []uuid.UUID{operationRouteID1},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	expectedError := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, payload.OperationRoutes).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
}

// TestCreateTransactionRouteErrorMissingDebitRoute tests error when debit operation route is missing
func TestCreateTransactionRouteErrorMissingDebitRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()

	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		OperationRoutes: []uuid.UUID{operationRouteID1},
	}

	// Only credit operation route, missing debit
	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:            operationRouteID1,
			OperationType: "destination",
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, payload.OperationRoutes).
		Return(expectedOperationRoutes, nil).
		Times(1)

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	expectedError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedError, err)
}

// TestCreateTransactionRouteErrorMissingCreditRoute tests error when credit operation route is missing
func TestCreateTransactionRouteErrorMissingCreditRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()

	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		OperationRoutes: []uuid.UUID{operationRouteID1},
	}

	// Only debit operation route, missing credit
	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:            operationRouteID1,
			OperationType: "source",
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, payload.OperationRoutes).
		Return(expectedOperationRoutes, nil).
		Times(1)

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	expectedError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedError, err)
}

// TestCreateTransactionRouteErrorTransactionRouteCreationFails tests error when transaction route creation fails
func TestCreateTransactionRouteErrorTransactionRouteCreationFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()
	operationRouteID2 := uuid.New()

	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		OperationRoutes: []uuid.UUID{operationRouteID1, operationRouteID2},
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:            operationRouteID1,
			OperationType: "source",
		},
		{
			ID:            operationRouteID2,
			OperationType: "destination",
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, payload.OperationRoutes).
		Return(expectedOperationRoutes, nil).
		Times(1)

	expectedError := errors.New("failed to create transaction route")
	mockTransactionRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
}

// TestCreateTransactionRouteErrorMetadataCreationFails tests error when metadata creation fails
func TestCreateTransactionRouteErrorMetadataCreationFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()
	operationRouteID2 := uuid.New()

	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		OperationRoutes: []uuid.UUID{operationRouteID1, operationRouteID2},
		Metadata:        map[string]any{"key": "value"},
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:            operationRouteID1,
			OperationType: "source",
		},
		{
			ID:            operationRouteID2,
			OperationType: "destination",
		},
	}

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:          uuid.New(),
		Title:       payload.Title,
		Description: payload.Description,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		MetadataRepo:         mockMetadataRepo,
	}

	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, payload.OperationRoutes).
		Return(expectedOperationRoutes, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedTransactionRoute, nil).
		Times(1)

	expectedError := errors.New("failed to create metadata")
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), reflect.TypeOf(mmodel.TransactionRoute{}).Name(), gomock.Any()).
		Return(expectedError).
		Times(1)

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
}

// TestCreateTransactionRouteErrorInvalidMetadata tests error when metadata validation fails
func TestCreateTransactionRouteErrorInvalidMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID1 := uuid.New()
	operationRouteID2 := uuid.New()

	// Create metadata that exceeds key length limit
	longKey := string(make([]byte, 101)) // Exceeds 100 character limit
	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		OperationRoutes: []uuid.UUID{operationRouteID1, operationRouteID2},
		Metadata:        map[string]any{longKey: "value"},
	}

	expectedOperationRoutes := []*mmodel.OperationRoute{
		{
			ID:            operationRouteID1,
			OperationType: "source",
		},
		{
			ID:            operationRouteID2,
			OperationType: "destination",
		},
	}

	expectedTransactionRoute := &mmodel.TransactionRoute{
		ID:          uuid.New(),
		Title:       payload.Title,
		Description: payload.Description,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), organizationID, ledgerID, payload.OperationRoutes).
		Return(expectedOperationRoutes, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedTransactionRoute, nil).
		Times(1)

	result, err := uc.CreateTransactionRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "0050")
}

// TestValidateOperationRouteTypesSuccess tests successful validation
func TestValidateOperationRouteTypesSuccess(t *testing.T) {
	operationRoutes := []*mmodel.OperationRoute{
		{OperationType: "source"},
		{OperationType: "destination"},
	}

	err := validateOperationRouteTypes(operationRoutes)
	assert.NoError(t, err)
}

// TestValidateOperationRouteTypesMissingDebit tests validation error when debit is missing
func TestValidateOperationRouteTypesMissingDebit(t *testing.T) {
	operationRoutes := []*mmodel.OperationRoute{
		{OperationType: "destination"},
		{OperationType: "destination"},
	}

	err := validateOperationRouteTypes(operationRoutes)
	assert.Error(t, err)
	expectedError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedError, err)
}

// TestValidateOperationRouteTypesMissingCredit tests validation error when credit is missing
func TestValidateOperationRouteTypesMissingCredit(t *testing.T) {
	operationRoutes := []*mmodel.OperationRoute{
		{OperationType: "source"},
		{OperationType: "source"},
	}

	err := validateOperationRouteTypes(operationRoutes)
	assert.Error(t, err)
	expectedError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedError, err)
}

// TestValidateOperationRouteTypesEmpty tests validation with empty array
func TestValidateOperationRouteTypesEmpty(t *testing.T) {
	operationRoutes := []*mmodel.OperationRoute{}

	err := validateOperationRouteTypes(operationRoutes)
	assert.Error(t, err)
	expectedError := pkg.ValidateBusinessError(constant.ErrMissingOperationRoutes, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	assert.Equal(t, expectedError, err)
}
