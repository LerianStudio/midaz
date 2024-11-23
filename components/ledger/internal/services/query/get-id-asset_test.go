package query

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

// TestGetAssetByIDSuccess is responsible to test GetAssetByID with success
func TestGetAssetByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	a := &mmodel.Asset{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
	}

	uc := UseCase{
		AssetRepo: asset.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*asset.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(a, nil).
		Times(1)
	res, err := uc.AssetRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.Equal(t, res, a)
	assert.Nil(t, err)
}

// TestGetAssetByIDError is responsible to test GetAssetByID with error
func TestGetAssetByIDError(t *testing.T) {
	id := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRepo: asset.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*asset.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AssetRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
