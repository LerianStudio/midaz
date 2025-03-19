package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// DeleteLedgerByID deletes a ledger from the repository
func (uc *UseCase) DeleteLedgerByID(ctx context.Context, organizationID, id uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a new ledger operation with telemetry for delete
	op := uc.Telemetry.NewLedgerOperation("delete", id.String())

	// Add important attributes for telemetry
	op.WithAttributes(
		attribute.String("ledger_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
	)

	// Record system metric
	op.RecordSystemicMetric(ctx)

	// Start trace span for this operation
	ctx = op.StartTrace(ctx)

	defer func() {
		// End span will be done by op.End() at the end of the function
	}()

	logger.Infof("Remove ledger for id: %s", id.String())

	if err := uc.LedgerRepo.Delete(ctx, organizationID, id); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to delete ledger on repo by id", err)

		logger.Errorf("Error deleting ledger on repo by id: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "delete_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		return err
	}

	// Mark operation as successful
	op.End(ctx, "success")

	return nil
}
