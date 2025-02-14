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

	"github.com/google/uuid"
)

// DeleteSegmentByID delete a segment from the repository by ids.
func (uc *UseCase) DeleteSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_segment_by_id")
	defer span.End()

	logger.Infof("Remove segment for id: %s", id.String())

	if err := uc.SegmentRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete segment on repo by id", err)

		logger.Errorf("Error deleting segment on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrSegmentIDNotFound, reflect.TypeOf(mmodel.Segment{}).Name())
		}

		return err
	}

	return nil
}
