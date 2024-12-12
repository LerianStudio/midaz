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

// TestGetAllOperationsByAccount is responsible to test GetAllOperationsByAccount with success and error
func TestGetAllOperationsByAccount(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	accountID := pkg.GenerateUUIDv7()
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := http.CursorPagination{
		Next: "next",
		Prev: "prev",
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
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)
		res, cur, err := uc.OperationRepo.FindAllByAccount(context.TODO(), organizationID, ledgerID, accountID, filter.ToCursorPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.NotNil(t, cur)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockOperationRepo.
			EXPECT().
			FindAllByAccount(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(nil, http.CursorPagination{}, errors.New(errMsg)).
			Times(1)
		res, cur, err := uc.OperationRepo.FindAllByAccount(context.TODO(), organizationID, ledgerID, accountID, filter.ToCursorPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
		assert.Equal(t, cur, http.CursorPagination{})
	})
}
