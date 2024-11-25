package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/assetrate"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAssetRateByID(t *testing.T) {
	ID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()

	assetRate := &assetrate.AssetRate{
		ID:             ID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(assetRate, nil).
		Times(1)
	res, err := uc.AssetRateRepo.Find(context.TODO(), organizationID, ledgerID, ID)

	assert.Equal(t, assetRate, res)
	assert.Nil(t, err)
}

// TestGetAssetRateByIDError is responsible to test GetAssetRateByID with error
func TestGetAssetRateByIDError(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRateRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
