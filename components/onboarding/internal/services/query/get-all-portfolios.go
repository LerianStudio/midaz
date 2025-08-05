package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllPortfolio fetch all Portfolio from the repository
func (uc *UseCase) GetAllPortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Portfolio, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_portfolio")
	defer span.End()

	logger.Infof("Retrieving portfolios")

	portfolios, err := uc.PortfolioRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get portfolios on repo", err)

		logger.Errorf("Error getting portfolios on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		return nil, err
	}

	if portfolios != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), filter)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

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
