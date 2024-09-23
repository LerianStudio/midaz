package query

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

// TestGetAssetByIDSuccess is responsible to test GetAssetByID with success
func TestGetAssetByIDSuccess(t *testing.T) {
	id := uuid.New()
	ledgerID := uuid.New()
	organizationID := uuid.New()
	asset := &s.Asset{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
	}

	uc := UseCase{
		AssetRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(asset, nil).
		Times(1)
	res, err := uc.AssetRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.Equal(t, res, asset)
	assert.Nil(t, err)
}

// TestGetAssetByIDError is responsible to test GetAssetByID with error
func TestGetAssetByIDError(t *testing.T) {
	id := uuid.New()
	ledgerID := uuid.New()
	organizationID := uuid.New()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
