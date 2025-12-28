package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
)

// TestCreateOperationSuccess is responsible to test CreateOperation with success
func TestCreateOperationSuccess(t *testing.T) {
	o := &operation.Operation{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*operation.MockRepository).
		EXPECT().
		Create(gomock.Any(), o).
		Return(o, nil).
		Times(1)
	res, err := uc.OperationRepo.Create(context.TODO(), o)

	assert.Equal(t, o, res)
	assert.Nil(t, err)
}

// TestCreateOperationError is responsible to test CreateOperation with error
func TestCreateOperationError(t *testing.T) {
	errMSG := "err to create Operation on database"

	o := &operation.Operation{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*operation.MockRepository).
		EXPECT().
		Create(gomock.Any(), o).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.OperationRepo.Create(context.TODO(), o)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}

// TestCreateOperation_NilResultChannel_Panics verifies that passing a nil result
// channel causes a panic with descriptive context rather than a silent crash.
func TestCreateOperation_NilResultChannel_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(ctrl),
	}

	ctx := context.Background()
	errChan := make(chan error, 1)

	require.Panics(t, func() {
		uc.CreateOperation(ctx, nil, "txn-123", nil, pkgTransaction.Responses{}, nil, errChan)
	}, "CreateOperation should panic when result channel is nil")
}

// TestCreateOperation_NilErrorChannel_Panics verifies that passing a nil error
// channel causes a panic with descriptive context rather than a silent crash.
func TestCreateOperation_NilErrorChannel_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(ctrl),
	}

	ctx := context.Background()
	resultChan := make(chan []*operation.Operation, 1)

	require.Panics(t, func() {
		uc.CreateOperation(ctx, nil, "txn-123", nil, pkgTransaction.Responses{}, resultChan, nil)
	}, "CreateOperation should panic when error channel is nil")
}

// TestCreateOperationForBalance_SetsBalanceAffectedTrue verifies that operations
// created via createOperationForBalance have BalanceAffected set to true.
// TODO(review): Consider moving MockLogger to a shared test helper file instead of
// depending on the one defined in create-balance-transaction-operations-async_test.go
// (reported by code-reviewer on 2024-12-28, severity: Low)
// TODO(review): Consider adding more defensive assertions for other critical fields
// like ID, TransactionID, Type to strengthen test coverage
// (reported by business-logic-reviewer on 2024-12-28, severity: Low)
func TestCreateOperationForBalance_SetsBalanceAffectedTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOpRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		OperationRepo: mockOpRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	// Capture the operation passed to Create
	var capturedOp *operation.Operation
	mockOpRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
			capturedOp = op
			return op, nil
		}).
		Times(1)

	// Setup test data
	accountAlias := "@test"
	available := decimal.NewFromInt(100)
	onHold := decimal.NewFromInt(0)
	testBalance := &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		AccountID:      libCommons.GenerateUUIDv7().String(),
		Alias:          accountAlias,
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
		Available:      available,
		OnHold:         onHold,
	}

	testAmount := decimal.NewFromInt(10)
	testFromTo := pkgTransaction.FromTo{
		AccountAlias: accountAlias,
		IsFrom:       true,
		Amount: &pkgTransaction.Amount{
			Asset: "USD",
			Value: testAmount,
		},
	}

	testDSL := &pkgTransaction.Transaction{
		Description: "Test transaction",
		Send: pkgTransaction.Send{
			Asset: "USD",
		},
	}

	// Responses must have From entry for the account alias when IsFrom=true
	// TransactionType and Operation are required for ValidateFromToOperation
	testValidate := pkgTransaction.Responses{
		From: map[string]pkgTransaction.Amount{
			accountAlias: {
				Asset:           "USD",
				Value:           testAmount,
				Operation:       "DEBIT",
				TransactionType: "CREATED",
			},
		},
	}

	ctx := context.Background()
	logger := &MockLogger{}
	var span trace.Span

	// Call the function under test
	op, err := uc.createOperationForBalance(
		ctx,
		logger,
		&span,
		testBalance,
		testFromTo,
		"test-txn-id",
		testDSL,
		testValidate,
	)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, op)
	require.NotNil(t, capturedOp)

	// THE KEY ASSERTION: BalanceAffected must be true for normal operations
	assert.True(t, capturedOp.BalanceAffected,
		"BalanceAffected must be true for normal (non-annotation) operations")
}
