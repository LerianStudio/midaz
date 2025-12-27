package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
