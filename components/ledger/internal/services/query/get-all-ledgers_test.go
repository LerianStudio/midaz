package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestGetAllLedgersError is responsible to test GetAllLedgers with success and error
func TestGetAllLedgers(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	organizationID := pkg.GenerateUUIDv7()
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}

	uc := UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	t.Run("Success", func(t *testing.T) {
		ledgers := []*mmodel.Ledger{{}}
		mockLedgerRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, filter.ToPagination()).
			Return(ledgers, nil).
			Times(1)
		res, err := uc.LedgerRepo.FindAll(context.TODO(), organizationID, filter.ToPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockLedgerRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, filter.ToPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.LedgerRepo.FindAll(context.TODO(), organizationID, filter.ToPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
