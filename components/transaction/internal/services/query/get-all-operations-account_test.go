package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllOperationsByAccount is responsible to test GetAllOperationsByAccount with success and error
func TestGetAllOperationsByAccount(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	accountID := common.GenerateUUIDv7()
	limit := 10
	page := 1

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
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, limit, page).
			Return(trans, nil).
			Times(1)
		res, err := uc.OperationRepo.FindAllByAccount(context.TODO(), organizationID, ledgerID, accountID, limit, page)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockOperationRepo.
			EXPECT().
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, limit, page).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.OperationRepo.FindAllByAccount(context.TODO(), organizationID, ledgerID, accountID, limit, page)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
