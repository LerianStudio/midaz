package command

import (
	"context"
	"errors"
	"testing"

	a "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
	mock "github.com/LerianStudio/midaz/components/transaction/internal/gen/mock/assetrate"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAssetRateSuccess is responsible to test CreateAssetRate with success
func TestCreateAssetRateSuccess(t *testing.T) {
	assetRate := &a.AssetRate{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
	}

	uc := UseCase{
		AssetRateRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*mock.MockRepository).
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
	assetRate := &a.AssetRate{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
	}

	uc := UseCase{
		AssetRateRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), assetRate).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRateRepo.Create(context.TODO(), assetRate)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
