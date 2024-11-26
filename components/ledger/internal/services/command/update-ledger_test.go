package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/ledger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateLedgerByIDSuccess is responsible to test UpdateLedgerByID with success
func TestUpdateLedgerByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()

	l := &mmodel.Ledger{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		LedgerRepo: ledger.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*ledger.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, id, l).
		Return(l, nil).
		Times(1)
	res, err := uc.LedgerRepo.Update(context.TODO(), organizationID, id, l)

	assert.Equal(t, l, res)
	assert.Nil(t, err)
}

// TestUpdateLedgerByIDError is responsible to test UpdateLedgerByID with error
func TestUpdateLedgerByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"

	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()

	l := &mmodel.Ledger{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		LedgerRepo: ledger.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*ledger.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, id, l).
		Return(nil, errors.New(errMSG))
	res, err := uc.LedgerRepo.Update(context.TODO(), organizationID, id, l)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
