// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

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

// GetAllPortfolio retrieves a paginated list of portfolios with metadata.
//
// Fetches portfolios from PostgreSQL with pagination, then enriches with MongoDB metadata.
// Returns empty array if no portfolios found (not an error). Excludes soft-deleted portfolios.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - filter: Query parameters (pagination, sorting, date range)
//
// Returns:
//   - []*mmodel.Portfolio: Array of portfolios with metadata
//   - error: Business error if query fails
//
// OpenTelemetry: Creates span "query.get_all_portfolio"
func (uc *UseCase) GetAllPortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_portfolio")
	defer span.End()

	logger.Infof("Retrieving portfolios")

	portfolios, err := uc.PortfolioRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		logger.Errorf("Error getting portfolios on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get portfolios on repo", err)

			logger.Warn("No portfolios found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get portfolios on repo", err)

		return nil, err
	}

	if len(portfolios) == 0 {
		return portfolios, nil
	}

	portfolioIDs := make([]string, len(portfolios))
	for i, p := range portfolios {
		portfolioIDs[i] = p.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), portfolioIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on repo", err)

		logger.Warn("No metadata found")

		return nil, err
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

	return portfolios, nil
}
