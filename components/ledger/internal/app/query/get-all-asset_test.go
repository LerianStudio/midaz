package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"testing"

	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/asset"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllAssetsError is responsible to test GetAllAssets with success and error
func TestGetAllAssets(t *testing.T) {
	ledgerID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	limit := 10
	page := 1

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockAssetRepo := mock.NewMockRepository(ctrl)

	uc := UseCase{
		AssetRepo: mockAssetRepo,
	}

	t.Run("Success", func(t *testing.T) {
		assets := []*mmodel.Asset{{}}
		mockAssetRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, page, limit).
			Return(assets, nil).
			Times(1)
		res, err := uc.AssetRepo.FindAll(context.TODO(), organizationID, ledgerID, page, limit)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockAssetRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, page, limit).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.AssetRepo.FindAll(context.TODO(), organizationID, ledgerID, page, limit)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
