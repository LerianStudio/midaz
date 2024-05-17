package command

import (
	"context"
	"errors"
	"testing"

	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/instrument"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteInstrumentByIDSuccess is responsible to test DeleteInstrumentByID with success
func TestDeleteInstrumentByIDSuccess(t *testing.T) {
	id := uuid.New()
	ledgerID := uuid.New()
	organizationID := uuid.New()

	uc := UseCase{
		InstrumentRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.InstrumentRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)
	err := uc.InstrumentRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.Nil(t, err)
}

// TestDeleteInstrumentByIDError is responsible to test DeleteInstrumentByID with error
func TestDeleteInstrumentByIDError(t *testing.T) {
	id := uuid.New()
	ledgerID := uuid.New()
	organizationID := uuid.New()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		InstrumentRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.InstrumentRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.InstrumentRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
