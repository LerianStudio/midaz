package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestDeleteLedgerByIDSuccess is responsible to test DeleteLedgerByID with success
func TestDeleteLedgerByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()

	uc := UseCase{
		LedgerRepo: ledger.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*ledger.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, id).
		Return(nil).
		Times(1)
	err := uc.LedgerRepo.Delete(context.TODO(), organizationID, id)

	assert.Nil(t, err)
}

// TestDeleteLedgerByIDError is responsible to test DeleteLedgerByID with error
func TestDeleteLedgerByIDError(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		LedgerRepo: ledger.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*ledger.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.LedgerRepo.Delete(context.TODO(), organizationID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
