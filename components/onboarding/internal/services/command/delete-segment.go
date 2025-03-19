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

// DeleteSegmentByID delete a segment from the repository by ids.
func (uc *UseCase) DeleteSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)

	op := uc.Telemetry.NewSegmentOperation("delete", id.String())

	op.WithAttributes(
		attribute.String("segment_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Remove segment for id: %s", id.String())

	if err := uc.SegmentRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to delete segment on repo by id", err)
		logger.Errorf("Error deleting segment on repo by id: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "delete_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())
		}

		return err
	}

	op.End(ctx, "success")

	return nil
}
