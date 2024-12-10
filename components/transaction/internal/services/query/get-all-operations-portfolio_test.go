package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestGetAllOperationsByPortfolio is responsible to test GetAllOperationsByPortfolio with success and error
func TestGetAllOperationsByPortfolio(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	portfolioID := pkg.GenerateUUIDv7()
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
	mockOperationRepo := operation.NewMockRepository(ctrl)

	uc := UseCase{
		OperationRepo: mockOperationRepo,
	}

	t.Run("Success", func(t *testing.T) {
		trans := []*operation.Operation{{}}
		mockOperationRepo.
			EXPECT().
			FindAllByPortfolio(gomock.Any(), organizationID, ledgerID, portfolioID, filter.ToPagination()).
			Return(trans, nil).
			Times(1)
		res, err := uc.OperationRepo.FindAllByPortfolio(context.TODO(), organizationID, ledgerID, portfolioID, filter.ToPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockOperationRepo.
			EXPECT().
			FindAllByPortfolio(gomock.Any(), organizationID, ledgerID, portfolioID, filter.ToPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.OperationRepo.FindAllByPortfolio(context.TODO(), organizationID, ledgerID, portfolioID, filter.ToPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
