package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestGetAllPortfoliosError is responsible to test GetAllPortfolios with success and error
func TestGetAllPortfolios(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}

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
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToPagination()).
			Return(portfolios, nil).
			Times(1)
		res, err := uc.PortfolioRepo.FindAll(context.TODO(), organizationID, ledgerID, filter.ToPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockPortfolioRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.PortfolioRepo.FindAll(context.TODO(), organizationID, ledgerID, filter.ToPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
