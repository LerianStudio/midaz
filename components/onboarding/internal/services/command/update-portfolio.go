// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

package command

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

// UpdatePortfolioByID updates an existing portfolio in the repository.
//
// This method implements the update portfolio use case, which:
// 1. Updates the portfolio in PostgreSQL
// 2. Updates associated metadata in MongoDB using merge semantics
// 3. Returns the updated portfolio with merged metadata
//
// Business Rules:
//   - Only provided fields are updated (partial updates supported)
//   - Name can be updated
//   - Entity ID can be updated
//   - Status can be updated
//   - Metadata is merged with existing
//
// Update Behavior:
//   - Empty strings in input are treated as "clear the field"
//   - Empty status means "don't update status"
//   - Metadata is merged with existing metadata (RFC 7396)
//
// Data Storage:
//   - Primary data: PostgreSQL (portfolios table)
//   - Metadata: MongoDB (merged with existing)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the portfolio to update
//   - upi: Update portfolio input with fields to update
//
// Returns:
//   - *mmodel.Portfolio: Updated portfolio with merged metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrPortfolioIDNotFound: Portfolio doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.UpdatePortfolioInput{
//	    Name:     "Corporate Accounts - Updated",
//	    EntityID: "EXT-CORP-002",
//	}
//	portfolio, err := useCase.UpdatePortfolioByID(ctx, orgID, ledgerID, portfolioID, input)
//
// OpenTelemetry:
//   - Creates span "command.update_portfolio_by_id"
//   - Records errors as span events
func (uc *UseCase) UpdatePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdatePortfolioInput) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_portfolio_by_id")
	defer span.End()

	logger.Infof("Trying to update portfolio: %v", upi)

	portfolio := &mmodel.Portfolio{
		EntityID: upi.EntityID,
		Name:     upi.Name,
		Status:   upi.Status,
	}

	portfolioUpdated, err := uc.PortfolioRepo.Update(ctx, organizationID, ledgerID, id, portfolio)
	if err != nil {
		logger.Errorf("Error updating portfolio on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			logger.Warnf("Portfolio ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update portfolio on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update portfolio on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	portfolioUpdated.Metadata = metadataUpdated

	return portfolioUpdated, nil
}
