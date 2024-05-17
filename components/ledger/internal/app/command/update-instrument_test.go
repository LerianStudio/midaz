package command

import (
	"context"
	"errors"
	"testing"
	"time"

	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/instrument"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateInstrumentByIDSuccess is responsible to test UpdateInstrumentByID with success
func TestUpdateInstrumentByIDSuccess(t *testing.T) {
	id := uuid.New()
	ledgerID := uuid.New()
	organizationID := uuid.New()
	instrument := &i.Instrument{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		InstrumentRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.InstrumentRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, instrument).
		Return(instrument, nil).
		Times(1)
	res, err := uc.InstrumentRepo.Update(context.TODO(), organizationID, ledgerID, id, instrument)

	assert.Equal(t, instrument, res)
	assert.Nil(t, err)
}

// TestUpdateInstrumentByIDError is responsible to test UpdateInstrumentByID with error
func TestUpdateInstrumentByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	id := uuid.New()
	ledgerID := uuid.New()
	organizationID := uuid.New()
	instrument := &i.Instrument{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		InstrumentRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.InstrumentRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, instrument).
		Return(nil, errors.New(errMSG))
	res, err := uc.InstrumentRepo.Update(context.TODO(), organizationID, ledgerID, id, instrument)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
