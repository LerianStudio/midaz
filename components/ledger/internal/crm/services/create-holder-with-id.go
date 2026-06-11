// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// CreateHolderWithID inserts a holder using a caller-supplied deterministic ID.
//
// Unlike CreateHolder it does not mint a UUIDv7; the caller owns the ID so the
// record's _id is reproducible (used for the org self-holder and the backfill
// runner). A conflict that could mean the deterministic holder already exists —
// a duplicate _id or a document-association conflict on the supplied id — is
// treated as idempotent success: the holder is re-fetched by id and returned, so
// re-running the provisioning path is a no-op. If the document conflict resolves
// to a genuinely different holder, the conflict propagates unchanged.
func (uc *UseCase) CreateHolderWithID(ctx context.Context, organizationID string, id uuid.UUID, chi *mmodel.CreateHolderInput) (_ *mmodel.Holder, err error) {
	logger, tracer, reqID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_holder_with_id")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "create_holder_with_id", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context cancelled before holder provisioning", err)

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
		// A collision on the deterministic _id satisfies mongo.IsDuplicateKeyError.
		// A collision on the unique search.document index is mapped by the repository
		// to a typed document-association business error, which does NOT wrap the
		// mongo write exception and so fails IsDuplicateKeyError. Both can mean the
		// self-holder is already provisioned, so both trigger an idempotent re-fetch
		// by the supplied _id: a hit confirms the same record (success); a different
		// owner means the document genuinely belongs to another holder (propagate).
		if mongo.IsDuplicateKeyError(err) || isDocumentAssociationError(err) {
			existing, findErr := uc.HolderRepo.Find(ctx, organizationID, id, false)
			if findErr != nil {
				recordSpanError(span, "Failed to re-fetch existing holder after duplicate key", findErr)

				return nil, findErr
			}

			if existing.ID != nil && *existing.ID == id {
				return existing, nil
			}

			recordSpanError(span, "Failed to create holder with id", err)

			return nil, err
		}

		recordSpanError(span, "Failed to create holder with id", err)

		return nil, err
	}

	return createdHolder, nil
}

// isDocumentAssociationError reports whether err is the holder document-association
// conflict raised by the repository on a unique search.document index collision.
// EntityConflictError implements neither Is nor a stable value equality (its Err
// field defeats ==), so we discriminate by Code — mirroring holderReaderAdapter.Exists.
func isDocumentAssociationError(err error) bool {
	var conflict pkg.EntityConflictError

	return errors.As(err, &conflict) && conflict.Code == constant.ErrDocumentAssociationError.Error()
}
