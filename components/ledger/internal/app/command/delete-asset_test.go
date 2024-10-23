package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"

	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/asset"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteAssetByIDSuccess is responsible to test DeleteAssetByID with success
func TestDeleteAssetByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()

	uc := UseCase{
		AssetRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)
	err := uc.AssetRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.Nil(t, err)
}

// TestDeleteAssetByIDError is responsible to test DeleteAssetByID with error
func TestDeleteAssetByIDError(t *testing.T) {
	id := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.AssetRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
