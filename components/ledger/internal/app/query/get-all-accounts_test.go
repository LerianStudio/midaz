package query

import (
	"context"
	"errors"
	"testing"

	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/account"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllAccountsError is responsible to test GetAllAccounts with success and error
func TestGetAllAccounts(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockAccountRepo := mock.NewMockRepository(ctrl)

	uc := UseCase{
		AccountRepo: mockAccountRepo,
	}

	t.Run("Success", func(t *testing.T) {
		accounts := []*a.Account{{}}
		mockAccountRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, portfolioID).
			Return(accounts, nil).
			Times(1)
		res, err := uc.AccountRepo.FindAll(context.TODO(), organizationID, ledgerID, portfolioID)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockAccountRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, portfolioID).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.AccountRepo.FindAll(context.TODO(), organizationID, ledgerID, portfolioID)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
