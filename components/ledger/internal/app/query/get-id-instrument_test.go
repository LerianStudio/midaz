package query

import (
	"context"
	"errors"
	"testing"

	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/instrument"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetInstrumentByIDSuccess is responsible to test GetInstrumentByID with success
func TestGetInstrumentByIDSuccess(t *testing.T) {
	id := uuid.New()
	ledgerID := uuid.New()
	organizationID := uuid.New()
	instrument := &i.Instrument{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
	}

	uc := UseCase{
		InstrumentRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.InstrumentRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(instrument, nil).
		Times(1)
	res, err := uc.InstrumentRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.Equal(t, res, instrument)
	assert.Nil(t, err)
}

// TestGetInstrumentByIDError is responsible to test GetInstrumentByID with error
func TestGetInstrumentByIDError(t *testing.T) {
	id := uuid.New()
	ledgerID := uuid.New()
	organizationID := uuid.New()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		InstrumentRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.InstrumentRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.InstrumentRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
