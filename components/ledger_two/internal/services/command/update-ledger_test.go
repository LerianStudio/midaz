package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	mock "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/mock/onboarding/ledger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateLedgerByIDSuccess is responsible to test UpdateLedgerByID with success
func TestUpdateLedgerByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()

	ledger := &mmodel.Ledger{
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

	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()

	ledger := &mmodel.Ledger{
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
