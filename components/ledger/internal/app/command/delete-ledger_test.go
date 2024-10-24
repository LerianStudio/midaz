package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"

	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/ledger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteLedgerByIDSuccess is responsible to test DeleteLedgerByID with success
func TestDeleteLedgerByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()

	uc := UseCase{
		LedgerRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*mock.MockRepository).
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
		LedgerRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.LedgerRepo.Delete(context.TODO(), organizationID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
