// Package command provides CQRS command handlers for the onboarding component.
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
// This function performs a partial update of a portfolio's mutable fields.
// It updates the portfolio record and synchronizes associated metadata.
//
// # Updatable Fields
//
// The following fields can be updated:
//   - EntityID: Change the associated external entity
//   - Name: Update portfolio name
//   - Status: Change portfolio status
//   - Metadata: Update arbitrary key-value data
//
// Non-updatable fields (set at creation): ID, OrganizationID, LedgerID, CreatedAt
//
// # Status Transitions
//
// Common status transitions:
//   - ACTIVE -> INACTIVE: Disable portfolio temporarily
//   - ACTIVE -> CLOSED: Permanently close portfolio
//   - INACTIVE -> ACTIVE: Reactivate portfolio
//
// Note: Status transition rules may be enforced at the repository level.
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.update_portfolio_by_id"
//  3. Build partial portfolio update model
//  4. Update portfolio in PostgreSQL via repository
//  5. Handle not found error (ErrPortfolioIDNotFound)
//  6. Update associated metadata in MongoDB
//  7. Return updated portfolio with metadata
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing this portfolio
//   - id: The UUID of the portfolio to update
//   - upi: UpdatePortfolioInput containing fields to update
//
// # Returns
//
//   - *mmodel.Portfolio: The updated portfolio
//   - error: If portfolio not found or database operations fail
//
// # Error Scenarios
//
//   - ErrPortfolioIDNotFound: Portfolio with given ID not found
//   - Database connection failure
//   - Metadata update failure (MongoDB)
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.update_portfolio_by_id" with error events.
// Logs operation progress, warnings for not found, errors for failures.
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
