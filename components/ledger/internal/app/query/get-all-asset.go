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

// GetAllAssets fetch all Asset from the repository
func (uc *UseCase) GetAllAssets(ctx context.Context, organizationID, ledgerID string, filter commonHTTP.QueryHeader) ([]*s.Asset, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving assets")

	assets, err := uc.AssetRepo.FindAll(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting assets on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(s.Asset{}).Name(),
				Message:    "Asset was not found",
				Code:       "ASSET_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if assets != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(s.Asset{}).Name(), filter)
		if err != nil {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(s.Asset{}).Name(),
				Message:    "Metadata was not found",
				Code:       "ASSET_NOT_FOUND",
				Err:        err,
			}
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for idx := range assets {
			if data, ok := metadataMap[assets[idx].ID]; ok {
				assets[idx].Metadata = data
			}
		}
	}

	return assets, nil
}
