package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/asset"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAssetSuccess is responsible to test CreateAsset with success
func TestCreateAssetSuccess(t *testing.T) {
	a := &mmodel.Asset{
		ID:             common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		AssetRepo: asset.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*asset.MockRepository).
		EXPECT().
		Create(gomock.Any(), a).
		Return(a, nil).
		Times(1)
	res, err := uc.AssetRepo.Create(context.TODO(), a)

	assert.Equal(t, a, res)
	assert.Nil(t, err)
}

// TestCreateAssetError is responsible to test CreateAsset with error
func TestCreateAssetError(t *testing.T) {
	errMSG := "err to create asset on database"
	a := &mmodel.Asset{
		ID:       common.GenerateUUIDv7().String(),
		LedgerID: common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		AssetRepo: asset.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*asset.MockRepository).
		EXPECT().
		Create(gomock.Any(), a).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRepo.Create(context.TODO(), a)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
