package command

import (
	"context"
	"errors"
	"testing"

	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/account"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteAccountByIDSuccess is responsible to test DeleteAccountByID with success
func TestDeleteAccountByIDSuccess(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()
	id := uuid.New()
	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, portfolioID, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, portfolioID, id)

	assert.Nil(t, err)
}

// TestDeleteAccountByIDError is responsible to test DeleteAccountByID with error
func TestDeleteAccountByIDError(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()
	id := uuid.New()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, portfolioID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, portfolioID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
