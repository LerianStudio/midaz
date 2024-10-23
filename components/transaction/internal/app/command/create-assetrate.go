package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	a "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
	m "github.com/LerianStudio/midaz/components/transaction/internal/domain/metadata"
	"github.com/google/uuid"
)

// CreateAssetRate creates a new asset rate and persists data in the repository.
func (uc *UseCase) CreateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, cari *a.CreateAssetRateInput) (*a.AssetRate, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create asset rate: %v", cari)

	if err := common.ValidateCode(cari.BaseAssetCode); err != nil {
		return nil, common.ValidateBusinessError(err, reflect.TypeOf(a.AssetRate{}).Name())
	}

	if err := common.ValidateCode(cari.CounterAssetCode); err != nil {
		return nil, common.ValidateBusinessError(err, reflect.TypeOf(a.AssetRate{}).Name())
	}

	assetRateDB := &a.AssetRate{
		ID:               uuid.New().String(),
		BaseAssetCode:    cari.BaseAssetCode,
		CounterAssetCode: cari.CounterAssetCode,
		Amount:           cari.Amount,
		Scale:            cari.Scale,
		Source:           cari.Source,
		OrganizationID:   organizationID.String(),
		LedgerID:         ledgerID.String(),
		CreatedAt:        time.Now(),
	}

	assetRate, err := uc.AssetRateRepo.Create(ctx, assetRateDB)
	if err != nil {
		logger.Errorf("Error creating asset rate: %v", err)
		return nil, err
	}

	if cari.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, cari.Metadata); err != nil {
			return nil, common.ValidateBusinessError(err, reflect.TypeOf(a.AssetRate{}).Name())
		}

		meta := m.Metadata{
			EntityID:   assetRate.ID,
			EntityName: reflect.TypeOf(a.AssetRate{}).Name(),
			Data:       cari.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(a.AssetRate{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating asset rate metadata: %v", err)
			return nil, err
		}

		assetRate.Metadata = cari.Metadata
	}

	return assetRate, nil
}
