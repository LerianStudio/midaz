package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/mpointers"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestUpdateAssetRateSuccess is responsible to test TestUpdateAssetRateSuccess with success
func TestUpdateAssetRateSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	orgID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	exID := pkg.GenerateUUIDv7()

	assetRate := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     exID.String(),
		From:           "USD",
		To:             "BRL",
		Rate:           100,
		Scale:          mpointers.Float64(2),
		Source:         mpointers.String("External System"),
		TTL:            3600,
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindByCurrencyPair(gomock.Any(), orgID, ledgerID, assetRate.From, assetRate.To).
		Return(assetRate, nil).
		Times(1)
	res, err := uc.AssetRateRepo.FindByCurrencyPair(context.TODO(), orgID, ledgerID, assetRate.From, assetRate.To)
	if err != nil {
		t.Errorf("Error finding asset rate by currency pair: %v", err)
	}

	assert.Equal(t, assetRate.OrganizationID, res.OrganizationID)
	assert.Equal(t, assetRate.LedgerID, res.LedgerID)
	assert.Equal(t, assetRate.ExternalID, res.ExternalID)
	assert.Equal(t, assetRate.From, res.From)
	assert.Equal(t, assetRate.To, res.To)
	assert.Equal(t, assetRate.Rate, res.Rate)
	assert.Equal(t, assetRate.Scale, res.Scale)
	assert.Equal(t, assetRate.Source, res.Source)
	assert.Equal(t, assetRate.TTL, res.TTL)
	assert.Nil(t, err)

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Update(gomock.Any(), orgID, ledgerID, id, assetRate).
		Return(assetRate, nil).
		Times(1)
	res, err = uc.AssetRateRepo.Update(context.TODO(), orgID, ledgerID, id, assetRate)
	if err != nil {
		t.Errorf("Error creating asset rate: %v", err)
	}

	assert.Equal(t, assetRate.OrganizationID, res.OrganizationID)
	assert.Equal(t, assetRate.LedgerID, res.LedgerID)
	assert.Equal(t, assetRate.ExternalID, res.ExternalID)
	assert.Equal(t, assetRate.From, res.From)
	assert.Equal(t, assetRate.To, res.To)
	assert.Equal(t, assetRate.Rate, res.Rate)
	assert.Equal(t, assetRate.Scale, res.Scale)
	assert.Equal(t, assetRate.Source, res.Source)
	assert.Equal(t, assetRate.TTL, res.TTL)
	assert.Nil(t, err)
}

// TestCreateAssetRateSuccess is responsible to test TestCreateAssetRateSuccess with success
func TestCreateAssetRateSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	orgID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	exID := pkg.GenerateUUIDv7()

	assetRate := &assetrate.AssetRate{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     exID.String(),
		From:           "USD",
		To:             "BRL",
		Rate:           100,
		Scale:          mpointers.Float64(2),
		Source:         mpointers.String("External System"),
		TTL:            3600,
	}

	uc := UseCase{
		AssetRateRepo: assetrate.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		FindByCurrencyPair(gomock.Any(), orgID, ledgerID, assetRate.From, assetRate.To).
		Return(nil, nil).
		Times(1)
	res, err := uc.AssetRateRepo.FindByCurrencyPair(context.TODO(), orgID, ledgerID, assetRate.From, assetRate.To)
	if err != nil {
		t.Errorf("Error finding asset rate by currency pair: %v", err)
	}

	assert.Nil(t, res)
	assert.Nil(t, err)

	uc.AssetRateRepo.(*assetrate.MockRepository).
		EXPECT().
		Create(gomock.Any(), assetRate).
		Return(assetRate, nil).
		Times(1)
	res, err = uc.AssetRateRepo.Create(context.TODO(), assetRate)
	if err != nil {
		t.Errorf("Error creating asset rate: %v", err)
	}

	assert.Equal(t, assetRate.OrganizationID, res.OrganizationID)
	assert.Equal(t, assetRate.LedgerID, res.LedgerID)
	assert.Equal(t, assetRate.ExternalID, res.ExternalID)
	assert.Equal(t, assetRate.From, res.From)
	assert.Equal(t, assetRate.To, res.To)
	assert.Equal(t, assetRate.Rate, res.Rate)
	assert.Equal(t, assetRate.Scale, res.Scale)
	assert.Equal(t, assetRate.Source, res.Source)
	assert.Equal(t, assetRate.TTL, res.TTL)
	assert.Nil(t, err)
}

// TestCreateAssetRateError is responsible to test CreateAssetRateError with error
func TestCreateAssetRateError(t *testing.T) {
	errMSG := "err to create asset rate on database"
	assetRate := &assetrate.AssetRate{
		ID:             pkg.GenerateUUIDv7().String(),
		OrganizationID: pkg.GenerateUUIDv7().String(),
		LedgerID:       pkg.GenerateUUIDv7().String(),
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
