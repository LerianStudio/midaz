// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for deleting a ledger.
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
// This use case performs a soft delete by setting the DeletedAt timestamp.
// The ledger record is preserved for auditing but is excluded from normal queries.
// Child entities such as assets and accounts are not automatically deleted.
//
// Business Rules:
//   - The ledger must exist and not have been previously deleted.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization that owns the ledger.
//   - id: The UUID of the ledger to be deleted.
//
// Returns:
//   - error: An error if the ledger is not found or if the deletion fails.
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
