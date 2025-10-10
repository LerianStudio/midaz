// Package query implements read operations (queries) for the onboarding service.
// This file contains the query for retrieving a portfolio by its ID.
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
	"github.com/google/uuid"
)

// GetPortfolioByID retrieves a single portfolio by its ID, enriched with metadata.
//
// This use case fetches a portfolio from the PostgreSQL database and its corresponding
// metadata from MongoDB, then merges them into a single response.
// Soft-deleted portfolios are excluded from the result.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the portfolio to retrieve.
//
// Returns:
//   - *mmodel.Portfolio: The portfolio with its metadata, or nil if not found.
//   - error: An error if the portfolio is not found or if the query fails.
func (uc *UseCase) GetPortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_portfolio_by_id")
	defer span.End()

	logger.Infof("Retrieving portfolio for id: %s", id)

	portfolio, err := uc.PortfolioRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Error getting portfolio on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get portfolio on repo by id", err)

			logger.Warn("No portfolio found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get portfolio on repo by id", err)

		return nil, err
	}

	if portfolio != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), id.String())
		if err != nil {
			// FIXME: This error handling is incorrect. It returns an ErrPortfolioIDNotFound, but the
			// error is related to fetching metadata, not the portfolio itself. The function should
			// either return the portfolio without metadata or a more appropriate metadata-specific error.
			err := pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb portfolio", err)

			logger.Warn("No metadata found")

			return nil, err
		}

		if metadata != nil {
			portfolio.Metadata = metadata.Data
		}
	}

	return portfolio, nil
}
