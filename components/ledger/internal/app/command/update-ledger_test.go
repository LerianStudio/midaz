package command

import (
	"context"
	"errors"
	"testing"
	"time"

	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/ledger"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateLedgerByIDSuccess is responsible to test UpdateLedgerByID with success
func TestUpdateLedgerByIDSuccess(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()

	ledger := &l.Ledger{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		LedgerRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, id, ledger).
		Return(ledger, nil).
		Times(1)
	res, err := uc.LedgerRepo.Update(context.TODO(), organizationID, id, ledger)

	assert.Equal(t, ledger, res)
	assert.Nil(t, err)
}

// TestUpdateLedgerByIDError is responsible to test UpdateLedgerByID with error
func TestUpdateLedgerByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"

	id := uuid.New()
	organizationID := uuid.New()

	ledger := &l.Ledger{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		LedgerRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, id, ledger).
		Return(nil, errors.New(errMSG))
	res, err := uc.LedgerRepo.Update(context.TODO(), organizationID, id, ledger)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
