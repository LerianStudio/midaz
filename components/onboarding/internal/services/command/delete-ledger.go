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

// DeleteLedgerByID removes a ledger from the repository.
//
// This function performs a soft delete of a ledger. Ledgers contain critical
// financial data and are subject to referential integrity constraints.
// Ledgers with active accounts, balances, or transactions may be restricted
// from deletion depending on the repository implementation.
//
// # Deletion Constraints
//
// Before deletion, the following should be cleaned up:
//   - All accounts in the ledger should be closed
//   - All balances should be zeroed
//   - All portfolios should be deleted
//   - All assets should be removed
//   - Associated metadata will need cleanup
//
// # Soft Delete
//
// Ledgers are typically soft-deleted to preserve:
//   - Transaction history for audit compliance
//   - Historical reporting capabilities
//   - References from other systems
//   - Regulatory data retention requirements
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.delete_ledger_by_id"
//  3. Call repository Delete method with organization and ledger IDs
//  4. Handle not found error (ErrLedgerIDNotFound)
//  5. Handle other database errors
//  6. Return success or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - id: The UUID of the ledger to delete
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Ledger not found (ErrLedgerIDNotFound)
//   - Referential integrity violation (has dependent entities)
//   - Database operation fails
//
// # Error Scenarios
//
//   - ErrLedgerIDNotFound: No ledger with given ID in organization
//   - Referential integrity violation (has accounts, transactions)
//   - Database connection failure
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "command.delete_ledger_by_id" with error events.
// Logs ledger ID at info level, warnings for not found, errors for failures.
func (uc *UseCase) DeleteLedgerByID(ctx context.Context, organizationID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_ledger_by_id")
	defer span.End()

	logger.Infof("Remove ledger for id: %s", id.String())

	if err := uc.LedgerRepo.Delete(ctx, organizationID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			logger.Warnf("Ledger ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete ledger on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete ledger on repo by id", err)

		logger.Errorf("Error deleting ledger: %v", err)

		return err
	}

	return nil
}
