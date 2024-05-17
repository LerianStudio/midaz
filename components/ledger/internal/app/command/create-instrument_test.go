package command

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

// TestCreateInstrumentSuccess is responsible to test CreateInstrument with success
func TestCreateInstrumentSuccess(t *testing.T) {
	instrument := &i.Instrument{
		ID:             uuid.New().String(),
		LedgerID:       uuid.New().String(),
		OrganizationID: uuid.New().String(),
	}

	uc := UseCase{
		InstrumentRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.InstrumentRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), instrument).
		Return(instrument, nil).
		Times(1)
	res, err := uc.InstrumentRepo.Create(context.TODO(), instrument)

	assert.Equal(t, instrument, res)
	assert.Nil(t, err)
}

// TestCreateInstrumentError is responsible to test CreateInstrument with error
func TestCreateInstrumentError(t *testing.T) {
	errMSG := "err to create instrument on database"
	instrument := &i.Instrument{
		ID:       uuid.New().String(),
		LedgerID: uuid.New().String(),
	}

	uc := UseCase{
		InstrumentRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.InstrumentRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), instrument).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.InstrumentRepo.Create(context.TODO(), instrument)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
