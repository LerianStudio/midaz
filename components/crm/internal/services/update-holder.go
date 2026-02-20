package services

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) UpdateHolderByID(ctx context.Context, organizationID string, id uuid.UUID, uhi *mmodel.UpdateHolderInput, fieldsToRemove []string) (*mmodel.Holder, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.update_holder")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

	logger.Infof("Trying to update holder: %v", id.String())

	holder := &mmodel.Holder{
		ExternalID:    uhi.ExternalID,
		Name:          uhi.Name,
		Contact:       uhi.Contact,
		Addresses:     uhi.Addresses,
		NaturalPerson: uhi.NaturalPerson,
		LegalPerson:   uhi.LegalPerson,
		Metadata:      uhi.Metadata,
		UpdatedAt:     time.Now(),
	}

	updatedHolder, err := uc.HolderRepo.Update(ctx, organizationID, id, holder, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to update holder", err)

		logger.Errorf("Failed to update holder: %v", err)

		return nil, err
	}

	return updatedHolder, nil
}
