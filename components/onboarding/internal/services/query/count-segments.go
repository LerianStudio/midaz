package query

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
	"go.opentelemetry.io/otel/attribute"
)

// CountSegments returns the number of segments for the specified organization and ledger.
func (uc *UseCase) CountSegments(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_segments")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	logger.Infof("Counting segments for organization %s and ledger %s", organizationID, ledgerID)

	count, err := uc.SegmentRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Errorf("Error counting segments on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoSegmentsFound, reflect.TypeOf(mmodel.Segment{}).Name())

			logger.Warnf("No segments found for organization: %s", organizationID.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count segments on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count segments on repo", err)

		return 0, err
	}

	logger.Infof("Found %d segments for organization %s and ledger %s", count, organizationID, ledgerID)

	return count, nil
}
