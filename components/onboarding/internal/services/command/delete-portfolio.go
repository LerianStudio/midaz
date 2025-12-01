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

// DeletePortfolioByID removes a portfolio from the repository.
//
// This function performs a soft delete of a portfolio. Portfolios group
// accounts for a single entity and may have referential constraints.
// Portfolios with active accounts may be restricted from deletion.
//
// # Deletion Constraints
//
// Before deletion, consider:
//   - All accounts in the portfolio should be closed or reassigned
//   - Account balances should be zeroed
//   - Associated metadata will need cleanup
//   - External systems referencing EntityID should be updated
//
// # Soft Delete
//
// Portfolios are typically soft-deleted to preserve:
//   - Historical account groupings
//   - Customer relationship history
//   - Compliance audit trails
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.delete_portfolio_by_id"
//  3. Call repository Delete method
//  4. Handle not found error (ErrPortfolioIDNotFound)
//  5. Handle other database errors
//  6. Return success or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing this portfolio
//   - id: The UUID of the portfolio to delete
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Portfolio not found (ErrPortfolioIDNotFound)
//   - Referential integrity violation (has active accounts)
//   - Database operation fails
//
// # Error Scenarios
//
//   - ErrPortfolioIDNotFound: No portfolio with given ID
//   - Referential integrity violation (has accounts)
//   - Database connection failure
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.delete_portfolio_by_id" with error events.
// Logs portfolio ID at info level, warnings for not found, errors for failures.
func (uc *UseCase) DeletePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_portfolio_by_id")
	defer span.End()

	logger.Infof("Remove portfolio for id: %s", id.String())

	if err := uc.PortfolioRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			logger.Warnf("Portfolio ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete portfolio on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete portfolio on repo by id", err)

		logger.Errorf("Error deleting portfolio: %v", err)

		return err
	}

	return nil
}
