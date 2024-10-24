package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"
	"time"

	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/portfolio"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdatePortfolioByIDSuccess is responsible to test UpdatePortfolioByID with success
func TestUpdatePortfolioByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolio := &p.Portfolio{
		ID:             id.String(),
		EntityID:       common.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		PortfolioRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, portfolio).
		Return(portfolio, nil).
		Times(1)
	res, err := uc.PortfolioRepo.Update(context.TODO(), organizationID, ledgerID, id, portfolio)

	assert.Equal(t, portfolio, res)
	assert.Nil(t, err)
}

// TestUpdatePortfolioByIDError is responsible to test UpdatePortfolioByID with error
func TestUpdatePortfolioByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolio := &p.Portfolio{
		ID:             id.String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		EntityID:       common.GenerateUUIDv7().String(),
		LedgerID:       ledgerID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		PortfolioRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, portfolio).
		Return(nil, errors.New(errMSG))
	res, err := uc.PortfolioRepo.Update(context.TODO(), organizationID, ledgerID, id, portfolio)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
