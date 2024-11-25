package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/ledger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteLedgerByIDSuccess is responsible to test DeleteLedgerByID with success
func TestDeleteLedgerByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()

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
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
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
