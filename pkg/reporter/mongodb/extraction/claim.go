// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package extraction

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// AtomicClaimPending atomically transitions an extraction mapping from "pending" to "processing"
// using MongoDB findOneAndUpdate. Returns true if this worker successfully claimed the job
// (document was found with status=pending and updated to processing). Returns false if the
// document was not found or its status was already != "pending" (another worker claimed it).
// This provides idempotency guarantees per P17 decision.
func (r *ExtractionMappingMongoDBRepository) AtomicClaimPending(ctx context.Context, jobID string) (bool, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.extraction_mapping.atomic_claim_pending")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.job_id", jobID),
	)

	coll, err := r.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return false, err
	}

	filter := bson.M{
		"job_id": jobID,
		"status": constant.ExtractionStatusPending,
	}

	update := bson.M{
		"$set": bson.M{
			"status": constant.ExtractionStatusProcessing,
		},
	}

	_, spanUpdate := tracer.Start(ctx, "repository.extraction_mapping.atomic_claim_pending_exec")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.job_id", jobID),
	)

	var model ExtractionMappingMongoDBModel

	err = coll.FindOneAndUpdate(ctx, filter, update).Decode(&model)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Not found or already claimed by another worker -- not an error
			span.SetAttributes(attribute.Bool("app.request.claimed", false))
			return false, nil
		}

		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to atomically claim extraction mapping", err)

		return false, fmt.Errorf("atomic claim pending for job %s: %w", jobID, err)
	}

	span.SetAttributes(attribute.Bool("app.request.claimed", true))

	return true, nil
}
