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
	"github.com/google/uuid"
)

// GetPortfolioByID retrieves a single portfolio by ID with metadata.
//
// This method implements the get portfolio query use case, which:
// 1. Fetches the portfolio from PostgreSQL by ID
// 2. Fetches associated metadata from MongoDB
// 3. Merges metadata into the portfolio object
// 4. Returns the enriched portfolio
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the portfolio to retrieve
//
// Returns:
//   - *mmodel.Portfolio: Portfolio with metadata
//   - error: Business error if not found or query fails
//
// Possible Errors:
//   - ErrPortfolioIDNotFound: Portfolio doesn't exist or is deleted
//
// OpenTelemetry:
//   - Creates span "query.get_portfolio_by_id"
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
