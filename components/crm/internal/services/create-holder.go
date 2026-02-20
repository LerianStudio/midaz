package services

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// CreateHolder inserts a holder data in the repository
func (uc *UseCase) CreateHolder(ctx context.Context, organizationID string, chi *mmodel.CreateHolderInput) (*mmodel.Holder, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_holder")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	)

	holderID := libCommons.GenerateUUIDv7()

	holder := &mmodel.Holder{
		ID:            &holderID,
		ExternalID:    chi.ExternalID,
		Type:          chi.Type,
		Name:          &chi.Name,
		Document:      &chi.Document,
		Addresses:     chi.Addresses,
		Contact:       chi.Contact,
		NaturalPerson: chi.NaturalPerson,
		LegalPerson:   chi.LegalPerson,
		Metadata:      chi.Metadata,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	createdHolder, err := uc.HolderRepo.Create(ctx, organizationID, holder)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create holder", err)

		logger.Errorf("Failed to create holder: %v", err)

		return nil, err
	}

	return createdHolder, nil
}
