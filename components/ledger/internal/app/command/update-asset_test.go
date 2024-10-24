package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"
	"time"

	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/asset"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateAssetByIDSuccess is responsible to test UpdateAssetByID with success
func TestUpdateAssetByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	asset := &s.Asset{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		AssetRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, asset).
		Return(asset, nil).
		Times(1)
	res, err := uc.AssetRepo.Update(context.TODO(), organizationID, ledgerID, id, asset)

	assert.Equal(t, asset, res)
	assert.Nil(t, err)
}

// TestUpdateAssetByIDError is responsible to test UpdateAssetByID with error
func TestUpdateAssetByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	id := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	asset := &s.Asset{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		AssetRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, asset).
		Return(nil, errors.New(errMSG))
	res, err := uc.AssetRepo.Update(context.TODO(), organizationID, ledgerID, id, asset)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
