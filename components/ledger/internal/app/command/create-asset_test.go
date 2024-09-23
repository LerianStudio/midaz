package command

import (
	"context"
	"errors"
	"testing"

	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/asset"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAssetSuccess is responsible to test CreateAsset with success
func TestCreateAssetSuccess(t *testing.T) {
	asset := &s.Asset{
		ID:             uuid.New().String(),
		LedgerID:       uuid.New().String(),
		OrganizationID: uuid.New().String(),
	}

	uc := UseCase{
		AssetRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), asset).
		Return(asset, nil).
		Times(1)
	res, err := uc.AssetRepo.Create(context.TODO(), asset)

	assert.Equal(t, asset, res)
	assert.Nil(t, err)
}

// TestCreateAssetError is responsible to test CreateAsset with error
func TestCreateAssetError(t *testing.T) {
	errMSG := "err to create asset on database"
	asset := &s.Asset{
		ID:       uuid.New().String(),
		LedgerID: uuid.New().String(),
	}

	uc := UseCase{
		AssetRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), asset).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRepo.Create(context.TODO(), asset)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
