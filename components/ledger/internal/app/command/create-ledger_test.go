package command

import (
	"context"
	"errors"
	"testing"

	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/ledger"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateLedgerSuccess is responsible to test CreateLedger with success
func TestCreateLedgerSuccess(t *testing.T) {
	ledger := &l.Ledger{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
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

	ledger := &l.Ledger{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
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
