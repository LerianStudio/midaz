package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestGetAssetRateByID(t *testing.T) {
	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	exID := libCommons.GenerateUUIDv7()

	assetRate := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     exID.String(),
		From:           "USD",
		To:             "BRL",
		Rate:           100,
		Scale:          libPointers.Float64(2),
		Source:         libPointers.String("External System"),
		TTL:            3600,
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindByExternalID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(assetRate, nil).
		Times(1)
	res, err := uc.AssetRateRepo.FindByExternalID(context.TODO(), orgID, ledgerID, exID)

	assert.Equal(t, assetRate, res)
	assert.Nil(t, err)
}

// TestGetAssetRateByIDError is responsible to test GetAssetRateByExternalID with error
func TestGetAssetRateByIDError(t *testing.T) {
	id := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindByExternalID(gomock.Any(), organizationID, ledgerID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRateRepo.FindByExternalID(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
