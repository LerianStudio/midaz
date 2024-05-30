package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/google/uuid"
)

// GetAllPortfolio fetch all Portfolio from the repository
func (uc *UseCase) GetAllPortfolio(ctx context.Context, organizationID, ledgerID string, filter common.QueryHeader) ([]*p.Portfolio, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Retrieving portfolios")

	portfolios, err := uc.PortfolioRepo.FindAll(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), filter.Limit, filter.Page)
	if err != nil {
		logger.Errorf("Error getting portfolios on repo: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
				Message:    "Portfolio was not found",
				Code:       "PORTFOLIO_NOT_FOUND",
				Err:        err,
			}
		}

		return nil, err
	}

	if portfolios != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(p.Portfolio{}).Name(), filter.Metadata)
		if err != nil {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
				Message:    "Metadata was not found",
				Code:       "PORTFOLIO_NOT_FOUND",
				Err:        err,
			}
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range portfolios {
			if data, ok := metadataMap[portfolios[i].ID]; ok {
				portfolios[i].Metadata = data
			}
		}
	}

	return portfolios, nil
}
