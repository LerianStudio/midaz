package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/operation"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetOperationByAccount(t *testing.T) {
	ID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	accountID := common.GenerateUUIDv7()

	o := &operation.Operation{
		ID:             ID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
	}

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*operation.MockRepository).
		EXPECT().
		FindByAccount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(o, nil).
		Times(1)
	res, err := uc.OperationRepo.FindByAccount(context.TODO(), organizationID, ledgerID, accountID, ID)

	assert.Equal(t, o, res)
	assert.Nil(t, err)
}

func TestGetOperationByAccountError(t *testing.T) {
	errMSG := "err to get operation on database"
	ID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	accountID := common.GenerateUUIDv7()

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*operation.MockRepository).
		EXPECT().
		FindByAccount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.OperationRepo.FindByAccount(context.TODO(), organizationID, ledgerID, accountID, ID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
