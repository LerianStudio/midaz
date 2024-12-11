package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestGetAllAssetsError is responsible to test GetAllAssets with success and error
func TestGetAllAssets(t *testing.T) {
	ledgerID := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
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
	mockAssetRepo := asset.NewMockRepository(ctrl)

	uc := UseCase{
		AssetRepo: mockAssetRepo,
	}

	t.Run("Success", func(t *testing.T) {
		assets := []*mmodel.Asset{{}}
		mockAssetRepo.
			EXPECT().
			FindAllWithDeleted(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
			Return(assets, nil).
			Times(1)
		res, err := uc.AssetRepo.FindAllWithDeleted(context.TODO(), organizationID, ledgerID, filter.ToOffsetPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockAssetRepo.
			EXPECT().
			FindAllWithDeleted(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.AssetRepo.FindAllWithDeleted(context.TODO(), organizationID, ledgerID, filter.ToOffsetPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
