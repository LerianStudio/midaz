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

	op := uc.Telemetry.NewLedgerOperation("delete", id.String())

	op.WithAttributes(
		attribute.String("ledger_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Remove ledger for id: %s", id.String())

	if err := uc.LedgerRepo.Delete(ctx, organizationID, id); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to delete ledger on repo by id", err)
		logger.Errorf("Error deleting ledger on repo by id: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "delete_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		return err
	}

	op.End(ctx, "success")

	return nil
}
