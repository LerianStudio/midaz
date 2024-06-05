package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/google/uuid"
)

// GetAllMetadataPortfolios fetch all Portfolios from the repository
func (uc *UseCase) GetAllMetadataPortfolios(ctx context.Context, organizationID, ledgerID string, filter commonHTTP.QueryHeader) ([]*p.Portfolio, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving portfolios")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(p.Portfolio{}).Name(), filter)
	if err != nil || metadata == nil {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
			Message:    "Portfolios by metadata was not found",
			Code:       "PORTFOLIO_NOT_FOUND",
			Err:        err,
		}
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	portfolios, err := uc.PortfolioRepo.ListByIDs(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuids)
	if err != nil {
		logger.Errorf("Error getting portfolios on repo by query params: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
				Message:    "Portfolios by metadata was not found",
				Code:       "PORTFOLIO_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	for i := range portfolios {
		if data, ok := metadataMap[portfolios[i].ID]; ok {
			portfolios[i].Metadata = data
		}
	}

	return portfolios, nil
}
