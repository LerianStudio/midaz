// Package command implements write operations (commands) for the onboarding service.
// This file contains the DeleteLedgerByID command implementation.
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

// DeleteLedgerByID soft-deletes a ledger from the repository.
//
// This method implements the delete ledger use case, which performs a soft delete
// by setting the DeletedAt timestamp. The ledger record remains in the database
// but is excluded from normal queries.
//
// Business Rules:
//   - Ledger must exist and not be already deleted
//   - Ledger should not have active child entities (assets, accounts, etc.)
//   - Soft delete is idempotent (deleting already deleted ledger returns error)
//
// Soft Deletion:
//   - Sets DeletedAt timestamp to current time
//   - Ledger remains in database for audit purposes
//   - Excluded from list and get operations (WHERE deleted_at IS NULL)
//   - Can be used for historical reporting
//   - Cannot be undeleted (no restore operation)
//
// Cascade Behavior:
//   - Child entities (assets, accounts, portfolios, segments) are NOT automatically deleted
//   - Clients should delete child entities first if desired
//   - Foreign key constraints may prevent deletion if child entities exist
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization (used for scoping)
//   - id: UUID of the ledger to delete
//
// Returns:
//   - error: Business error if ledger not found, database error if deletion fails
//
// Possible Errors:
//   - ErrLedgerIDNotFound: Ledger doesn't exist or already deleted
//   - Database errors: Foreign key violations, connection failures
//
// Example:
//
//	err := useCase.DeleteLedgerByID(ctx, orgID, ledgerID)
//	if err != nil {
//	    return err
//	}
//	// Ledger is soft-deleted
//
// OpenTelemetry:
//   - Creates span "command.delete_ledger_by_id"
//   - Records errors as span events
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
