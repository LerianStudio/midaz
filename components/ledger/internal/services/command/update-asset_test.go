package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestUpdateAssetByIDSuccess is responsible to test UpdateAssetByID with success
func TestUpdateAssetByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	a := &mmodel.Asset{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		AssetRepo: asset.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*asset.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, a).
		Return(a, nil).
		Times(1)
	res, err := uc.AssetRepo.Update(context.TODO(), organizationID, ledgerID, id, a)

	assert.Equal(t, a, res)
	assert.Nil(t, err)
}

// TestUpdateAssetByIDError is responsible to test UpdateAssetByID with error
func TestUpdateAssetByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	id := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	a := &mmodel.Asset{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		AssetRepo: asset.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*asset.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, a).
		Return(nil, errors.New(errMSG))
	res, err := uc.AssetRepo.Update(context.TODO(), organizationID, ledgerID, id, a)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
