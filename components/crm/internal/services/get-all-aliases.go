package services

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllAliases retrieves all aliases with optional filtering and pagination.
func (uc *UseCase) GetAllAliases(ctx context.Context, organizationID string, holderID uuid.UUID, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Alias, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_all_aliases")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	)

	if holderID != uuid.Nil {
		span.SetAttributes(attribute.String("app.request.holder_id", holderID.String()))
	}

	aliases, err := uc.AliasRepo.FindAll(ctx, organizationID, holderID, filter, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get aliases", err)

		logger.Errorf("Failed to get aliases: %v", err)

		return nil, pkg.ValidateInternalError(err, "CRM")
	}

	for _, alias := range aliases {
		uc.enrichAliasWithLinkType(ctx, organizationID, alias)
	}

	return aliases, nil
}
