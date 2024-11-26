package query

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestGetLedgerByIDSuccess is responsible to test GetLedgerByID with success
func TestGetLedgerByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	l := &mmodel.Ledger{ID: id.String()}

	uc := UseCase{
		LedgerRepo: ledger.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*ledger.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, id).
		Return(l, nil).
		Times(1)
	res, err := uc.LedgerRepo.Find(context.TODO(), organizationID, id)

	assert.Equal(t, res, l)
	assert.Nil(t, err)
}

// TestGetLedgerByIDError is responsible to test GetLedgerByID with error
func TestGetLedgerByIDError(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		LedgerRepo: ledger.NewMockRepository(gomock.NewController(t)),
	}

	uc.LedgerRepo.(*ledger.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.LedgerRepo.Find(context.TODO(), organizationID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
