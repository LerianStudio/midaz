package services

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllHolders that match a query filter, and returns inside a paginated array
func (uc *UseCase) GetAllHolders(ctx context.Context, organizationID string, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Holder, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_all_holders")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	)

	logger.Infof("Retrieving holders")

	holders, err := uc.HolderRepo.FindAll(ctx, organizationID, filter, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get holders", err)

		logger.Errorf("Failed to get holders: %v", err)

		return nil, fmt.Errorf("failed to find all holders: %w", err)
	}

	return holders, nil
}
