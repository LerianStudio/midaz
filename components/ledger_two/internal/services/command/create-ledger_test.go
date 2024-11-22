package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	mock "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/mock/onboarding/ledger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateLedgerSuccess is responsible to test CreateLedger with success
func TestCreateLedgerSuccess(t *testing.T) {
	ledger := &mmodel.Ledger{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		LedgerRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), ledger).
		Return(ledger, nil).
		Times(1)
	res, err := uc.LedgerRepo.Create(context.TODO(), ledger)

	assert.Equal(t, ledger, res)
	assert.Nil(t, err)
}

// TestCreateLedgerError is responsible to test CreateLedger with error
func TestCreateLedgerError(t *testing.T) {
	errMSG := "err to create ledger on database"

	ledger := &mmodel.Ledger{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		LedgerRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), ledger).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.LedgerRepo.Create(context.TODO(), ledger)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
