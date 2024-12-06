package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/mpointers"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// GetAllAssetRatesByAssetCode is responsible to test GetAllAssetRatesByAssetCode
func GetAllAssetRatesByAssetCode(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	orgID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	fromAssetCode := "USD"
	toAssetCodes := []string{"BRL"}
	limit := 10
	page := 1

	assetRate := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     pkg.GenerateUUIDv7().String(),
		From:           fromAssetCode,
		To:             toAssetCodes[0],
		Rate:           100,
		Scale:          mpointers.Float64(2),
		Source:         mpointers.String("External System"),
		TTL:            3600,
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, toAssetCodes, limit, page).
		Return(assetRate, nil).
		Times(1)
	res, err := uc.AssetRateRepo.FindAllByAssetCodes(context.TODO(), orgID, ledgerID, fromAssetCode, toAssetCodes, limit, page)

	assert.Equal(t, assetRate, res)
	assert.Nil(t, err)
}

// GetAllAssetRatesByAssetCodeError is responsible to test GetAllAssetRatesByAssetCode with error
func GetAllAssetRatesByAssetCodeError(t *testing.T) {
	orgID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	fromAssetCode := "USD"
	toAssetCodes := []string{"BRL"}
	limit := 10
	page := 1
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, toAssetCodes, limit, page).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRateRepo.FindAllByAssetCodes(context.TODO(), orgID, ledgerID, fromAssetCode, toAssetCodes, limit, page)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
