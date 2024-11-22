package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/adapters/mock/portfolio/portfolio"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetPortfolioByIDSuccess is responsible to test GetPortfolioByID with success
func TestGetPortfolioByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolio := &mmodel.Portfolio{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
	}

	uc := UseCase{
		PortfolioRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(portfolio, nil).
		Times(1)
	res, err := uc.PortfolioRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.Equal(t, res, portfolio)
	assert.Nil(t, err)
}

// TestGetPortfolioByIDError is responsible to test GetPortfolioByID with error
func TestGetPortfolioByIDError(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		PortfolioRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.PortfolioRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
