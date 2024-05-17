package query

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

// TestGetLedgerByIDSuccess is responsible to test GetLedgerByID with success
func TestGetLedgerByIDSuccess(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	ledger := &l.Ledger{ID: id.String()}

	uc := UseCase{
		LedgerRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, id).
		Return(ledger, nil).
		Times(1)
	res, err := uc.LedgerRepo.Find(context.TODO(), organizationID, id)

	assert.Equal(t, res, ledger)
	assert.Nil(t, err)
}

// TestGetLedgerByIDError is responsible to test GetLedgerByID with error
func TestGetLedgerByIDError(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		LedgerRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.LedgerRepo.Find(context.TODO(), organizationID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
