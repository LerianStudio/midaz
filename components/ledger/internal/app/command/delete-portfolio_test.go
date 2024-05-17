package command

import (
	"context"
	"errors"
	"testing"

	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/portfolio"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeletePortfolioByIDSuccess is responsible to test DeletePortfolioByID with success
func TestDeletePortfolioByIDSuccess(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc := UseCase{
		PortfolioRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)
	err := uc.PortfolioRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.Nil(t, err)
}

// TestDeletePortfolioByIDError is responsible to test DeletePortfolioByID with error
func TestDeletePortfolioByIDError(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		PortfolioRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.PortfolioRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
