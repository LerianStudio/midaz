package services

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteHolderByID delete a holder by its ID
func (uc *UseCase) DeleteHolderByID(ctx context.Context, organizationID string, id uuid.UUID, hardDelete bool) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_holder_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

	logger.Infof("Delete holder by id %v", id)

	count, err := uc.AliasRepo.Count(ctx, organizationID, id)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to check linked aliases for holder: %v", err)

		logger.Errorf("Failed to check linked aliases for holder: %v", err)

		return pkg.ValidateInternalError(err, "CRM")
	}

	if count > 0 {
		return pkg.ValidateBusinessError(cn.ErrHolderHasAliases, reflect.TypeOf(mmodel.Holder{}).Name())
	}

	holderLinks, err := uc.HolderLinkRepo.FindByHolderID(ctx, organizationID, id, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to find holder links by holder id: %v", err)

		logger.Errorf("Failed to find holder links by holder id: %v", err)

		return pkg.ValidateInternalError(err, "CRM")
	}

	var firstErr error

	for _, holderLink := range holderLinks {
		deleteErr := uc.HolderLinkRepo.Delete(ctx, organizationID, *holderLink.ID, hardDelete)
		if deleteErr != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to delete holder link by id: %v", deleteErr)
			logger.Errorf("Failed to delete holder link id %s: %v", holderLink.ID.String(), deleteErr)

			if firstErr == nil {
				firstErr = deleteErr
			}

			continue
		}
	}

	if firstErr != nil {
		return pkg.ValidateInternalError(firstErr, "CRM")
	}

	err = uc.HolderRepo.Delete(ctx, organizationID, id, hardDelete)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to delete holder by id: %v", err)

		logger.Errorf("Failed to delete holder by id: %v", err)

		return pkg.ValidateInternalError(err, "CRM")
	}

	return nil
}
