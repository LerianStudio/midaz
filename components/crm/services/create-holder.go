// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// CreateHolder inserts a holder data in the repository.
func (uc *UseCase) CreateHolder(ctx context.Context, organizationID string, chi *mmodel.CreateHolderInput) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_holder")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	)

	holderID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate holder id", err)

		return nil, err
	}

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
		recordSpanError(span, "Failed to create holder", err)

		return nil, err
	}

	return createdHolder, nil
}
