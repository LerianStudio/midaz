package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/google/uuid"
)

// GetAllMetadataAssets fetch all Assets from the repository
func (uc *UseCase) GetAllMetadataAssets(ctx context.Context, organizationID, ledgerID string, filter commonHTTP.QueryHeader) ([]*s.Asset, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving assets")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(s.Asset{}).Name(), filter)
	if err != nil || metadata == nil {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(s.Asset{}).Name(),
			Message:    "Assets by metadata was not found",
			Code:       "ASSET_NOT_FOUND",
			Err:        err,
		}
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for idx, meta := range metadata {
		uuids[idx] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	assets, err := uc.AssetRepo.ListByIDs(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuids)
	if err != nil {
		logger.Errorf("Error getting assets on repo by query params: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(s.Asset{}).Name(),
				Message:    "Assets by metadata was not found",
				Code:       "ASSET_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	for idx := range assets {
		if data, ok := metadataMap[assets[idx].ID]; ok {
			assets[idx].Metadata = data
		}
	}

	return assets, nil
}
