// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// CreateHolderWithID inserts a holder using a caller-supplied deterministic ID.
//
// Unlike CreateHolder it does not mint a UUIDv7; the caller owns the ID so the
// record's _id is reproducible (used for the org self-holder and the backfill
// runner). A duplicate-key error on the _id is treated as idempotent success:
// the already-provisioned holder is re-fetched and returned, so re-running the
// provisioning path is a no-op.
func (uc *UseCase) CreateHolderWithID(ctx context.Context, organizationID string, id uuid.UUID, chi *mmodel.CreateHolderInput) (*mmodel.Holder, error) {
	logger, tracer, reqID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_holder_with_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Context cancelled before holder provisioning", err)

		return nil, err
	}

	now := time.Now()

	holder := &mmodel.Holder{
		ID:            &id,
		ExternalID:    chi.ExternalID,
		Type:          chi.Type,
		Name:          &chi.Name,
		Document:      &chi.Document,
		Addresses:     chi.Addresses,
		Contact:       chi.Contact,
		NaturalPerson: chi.NaturalPerson,
		LegalPerson:   chi.LegalPerson,
		Metadata:      chi.Metadata,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	createdHolder, err := uc.HolderRepo.Create(ctx, organizationID, holder)
	if err != nil {
		// A duplicate _id means the deterministic holder is already provisioned.
		// The repository wraps document-index collisions as a typed business error,
		// so only the raw _id collision still satisfies mongo.IsDuplicateKeyError —
		// re-fetch and return the existing holder as idempotent success.
		if mongo.IsDuplicateKeyError(err) {
			existing, findErr := uc.HolderRepo.Find(ctx, organizationID, id, false)
			if findErr != nil {
				libOpenTelemetry.HandleSpanError(span, "Failed to re-fetch existing holder after duplicate key", findErr)
				logger.Log(ctx, libLog.LevelError, "Failed to re-fetch existing holder after duplicate key", libLog.Err(findErr))

				return nil, findErr
			}

			return existing, nil
		}

		libOpenTelemetry.HandleSpanError(span, "Failed to create holder with id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create holder with id", libLog.Err(err))

		return nil, err
	}

	return createdHolder, nil
}
