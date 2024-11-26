package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestDeletePortfolioByIDSuccess is responsible to test DeletePortfolioByID with success
func TestDeletePortfolioByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()

	uc := UseCase{
		PortfolioRepo: portfolio.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*portfolio.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)
	err := uc.PortfolioRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.Nil(t, err)
}

// TestDeletePortfolioByIDError is responsible to test DeletePortfolioByID with error
func TestDeletePortfolioByIDError(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		PortfolioRepo: portfolio.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*portfolio.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.PortfolioRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
