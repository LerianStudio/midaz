package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	mock "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/mock/portfolio/portfolio"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreatePortfolioSuccess is responsible to test CreatePortfolio with success
func TestCreatePortfolioSuccess(t *testing.T) {
	portfolio := &mmodel.Portfolio{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		EntityID:       common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		PortfolioRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), portfolio).
		Return(portfolio, nil).
		Times(1)
	res, err := uc.PortfolioRepo.Create(context.TODO(), portfolio)

	assert.Equal(t, portfolio, res)
	assert.Nil(t, err)
}

// TestCreatePortfolioError is responsible to test CreatePortfolio with error
func TestCreatePortfolioError(t *testing.T) {
	errMSG := "err to create portfolio on database"
	portfolio := &mmodel.Portfolio{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		EntityID:       common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		PortfolioRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), portfolio).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.PortfolioRepo.Create(context.TODO(), portfolio)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
