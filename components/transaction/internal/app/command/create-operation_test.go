package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"

	op "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	mock "github.com/LerianStudio/midaz/components/transaction/internal/gen/mock/operation"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateOperationSuccess is responsible to test CreateOperation with success
func TestCreateOperationSuccess(t *testing.T) {
	Operation := &op.Operation{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
		PortfolioID:    common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		OperationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), Operation).
		Return(Operation, nil).
		Times(1)
	res, err := uc.OperationRepo.Create(context.TODO(), Operation)

	assert.Equal(t, Operation, res)
	assert.Nil(t, err)
}

// TestCreateOperationError is responsible to test CreateOperation with error
func TestCreateOperationError(t *testing.T) {
	errMSG := "err to create Operation on database"

	Operation := &op.Operation{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
		PortfolioID:    common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		OperationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), Operation).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.OperationRepo.Create(context.TODO(), Operation)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
