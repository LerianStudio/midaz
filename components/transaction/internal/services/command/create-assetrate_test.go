package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAssetRateSuccess is responsible to test CreateAssetRate with success
func TestCreateAssetRateSuccess(t *testing.T) {
	assetRate := &assetrate.AssetRate{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Create(gomock.Any(), assetRate).
		Return(assetRate, nil).
		Times(1)
	res, err := uc.AssetRateRepo.Create(context.TODO(), assetRate)

	assert.Equal(t, assetRate, res)
	assert.Nil(t, err)
}

// TestCreateAssetRateError is responsible to test CreateAssetRateError with error
func TestCreateAssetRateError(t *testing.T) {
	errMSG := "err to create asset rate on database"
	assetRate := &assetrate.AssetRate{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Create(gomock.Any(), assetRate).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRateRepo.Create(context.TODO(), assetRate)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
