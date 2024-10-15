package query

import (
	"context"
	"errors"
	"testing"

	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	mock "github.com/LerianStudio/midaz/components/transaction/internal/gen/mock/operation"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetOperationByAccount(t *testing.T) {
	ID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	operation := &o.Operation{
		ID:             ID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
	}

	uc := UseCase{
		OperationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*mock.MockRepository).
		EXPECT().
		FindByAccount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(operation, nil).
		Times(1)
	res, err := uc.OperationRepo.FindByAccount(context.TODO(), organizationID, ledgerID, accountID, ID)

	assert.Equal(t, operation, res)
	assert.Nil(t, err)
}

func TestGetOperationByAccountError(t *testing.T) {
	errMSG := "err to get operation on database"
	ID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	uc := UseCase{
		OperationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*mock.MockRepository).
		EXPECT().
		FindByAccount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.OperationRepo.FindByAccount(context.TODO(), organizationID, ledgerID, accountID, ID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
