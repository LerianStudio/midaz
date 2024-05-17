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

// TestGetAllInstrumentsError is responsible to test GetAllInstruments with success and error
func TestGetAllInstruments(t *testing.T) {
	ledgerID := uuid.New()
	organizationID := uuid.New()

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockInstrumentRepo := mock.NewMockRepository(ctrl)

	uc := UseCase{
		InstrumentRepo: mockInstrumentRepo,
	}

	t.Run("Success", func(t *testing.T) {
		instruments := []*i.Instrument{{}}
		mockInstrumentRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID).
			Return(instruments, nil).
			Times(1)
		res, err := uc.InstrumentRepo.FindAll(context.TODO(), organizationID, ledgerID)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockInstrumentRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.InstrumentRepo.FindAll(context.TODO(), organizationID, ledgerID)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
