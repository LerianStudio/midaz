package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"

	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	mock "github.com/LerianStudio/midaz/components/transaction/internal/gen/mock/operation"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllOperationsByPortfolio is responsible to test GetAllOperationsByPortfolio with success and error
func TestGetAllOperationsByPortfolio(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolioID := common.GenerateUUIDv7()
	limit := 10
	page := 1

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockOperationRepo := mock.NewMockRepository(ctrl)

	uc := UseCase{
		OperationRepo: mockOperationRepo,
	}

	t.Run("Success", func(t *testing.T) {
		trans := []*o.Operation{{}}
		mockOperationRepo.
			EXPECT().
			FindAllByPortfolio(gomock.Any(), organizationID, ledgerID, portfolioID, limit, page).
			Return(trans, nil).
			Times(1)
		res, err := uc.OperationRepo.FindAllByPortfolio(context.TODO(), organizationID, ledgerID, portfolioID, limit, page)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockOperationRepo.
			EXPECT().
			FindAllByPortfolio(gomock.Any(), organizationID, ledgerID, portfolioID, limit, page).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.OperationRepo.FindAllByPortfolio(context.TODO(), organizationID, ledgerID, portfolioID, limit, page)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
