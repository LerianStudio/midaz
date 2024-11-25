package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/operation"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateOperationSuccess is responsible to test CreateOperation with success
func TestCreateOperationSuccess(t *testing.T) {
	o := &operation.Operation{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
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
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
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
