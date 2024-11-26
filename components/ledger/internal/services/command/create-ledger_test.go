package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestCreateLedgerSuccess is responsible to test CreateLedger with success
func TestCreateLedgerSuccess(t *testing.T) {
	l := &mmodel.Ledger{
		ID:             pkg.GenerateUUIDv7().String(),
		OrganizationID: pkg.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		LedgerRepo: ledger.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*ledger.MockRepository).
		EXPECT().
		Create(gomock.Any(), l).
		Return(l, nil).
		Times(1)
	res, err := uc.LedgerRepo.Create(context.TODO(), l)

	assert.Equal(t, l, res)
	assert.Nil(t, err)
}

// TestCreateLedgerError is responsible to test CreateLedger with error
func TestCreateLedgerError(t *testing.T) {
	errMSG := "err to create ledger on database"

	l := &mmodel.Ledger{
		ID:             pkg.GenerateUUIDv7().String(),
		OrganizationID: pkg.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		LedgerRepo: ledger.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*ledger.MockRepository).
		EXPECT().
		Create(gomock.Any(), l).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.LedgerRepo.Create(context.TODO(), l)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
