package command

import (
	"context"
	"errors"
	"testing"
	"time"

	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/portfolio"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdatePortfolioByIDSuccess is responsible to test UpdatePortfolioByID with success
func TestUpdatePortfolioByIDSuccess(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolio := &p.Portfolio{
		ID:             id.String(),
		EntityID:       uuid.New().String(),
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
	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolio := &p.Portfolio{
		ID:             id.String(),
		OrganizationID: uuid.New().String(),
		EntityID:       uuid.New().String(),
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
