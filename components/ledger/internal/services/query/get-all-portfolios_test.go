package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/portfolio"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllPortfoliosError is responsible to test GetAllPortfolios with success and error
func TestGetAllPortfolios(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	limit := 10
	page := 1

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)

	uc := UseCase{
		PortfolioRepo: mockPortfolioRepo,
	}

	t.Run("Success", func(t *testing.T) {
		portfolios := []*mmodel.Portfolio{{}}
		mockPortfolioRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, limit, page).
			Return(portfolios, nil).
			Times(1)
		res, err := uc.PortfolioRepo.FindAll(context.TODO(), organizationID, ledgerID, limit, page)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockPortfolioRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, limit, page).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.PortfolioRepo.FindAll(context.TODO(), organizationID, ledgerID, limit, page)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
