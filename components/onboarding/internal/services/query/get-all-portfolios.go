package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/google/uuid"
)

// GetAllPortfolio fetch all Portfolio from the repository
func (uc *UseCase) GetAllPortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Portfolio, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_portfolio")
	defer span.End()

	logger.Infof("Retrieving portfolios")

	portfolios, err := uc.PortfolioRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get portfolios on repo", err)

		logger.Errorf("Error getting portfolios on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		return nil, err
	}

	if portfolios != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

			return nil, pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
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
