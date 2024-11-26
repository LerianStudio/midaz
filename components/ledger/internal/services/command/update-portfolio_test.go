package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdatePortfolioByIDSuccess is responsible to test UpdatePortfolioByID with success
func TestUpdatePortfolioByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	p := &mmodel.Portfolio{
		ID:             id.String(),
		EntityID:       pkg.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		PortfolioRepo: portfolio.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*portfolio.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, p).
		Return(p, nil).
		Times(1)
	res, err := uc.PortfolioRepo.Update(context.TODO(), organizationID, ledgerID, id, p)

	assert.Equal(t, p, res)
	assert.Nil(t, err)
}

// TestUpdatePortfolioByIDError is responsible to test UpdatePortfolioByID with error
func TestUpdatePortfolioByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	p := &mmodel.Portfolio{
		ID:             id.String(),
		OrganizationID: pkg.GenerateUUIDv7().String(),
		EntityID:       pkg.GenerateUUIDv7().String(),
		LedgerID:       ledgerID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		PortfolioRepo: portfolio.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*portfolio.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, p).
		Return(nil, errors.New(errMSG))
	res, err := uc.PortfolioRepo.Update(context.TODO(), organizationID, ledgerID, id, p)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
