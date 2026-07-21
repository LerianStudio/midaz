// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"
	"errors"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Update applies the fields and returns the persisted billing package.
func (r *BillingPackageMongoDBRepository) Update(ctx context.Context, id string, organizationID string, updateFields *bson.M) (*model.BillingPackage, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.billing_package.update")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
	}

	span.SetAttributes(attributes...)

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(feeconstant.BillingPackageCollection))

	filter := bson.M{
		"_id":             id,
		"organization_id": organizationID,
		"deleted_at":      bson.M{"$eq": nil},
	}
	pipeline := buildUpdatePipeline(updateFields)
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	_, spanUpdate := tracer.Start(ctx, "repository.billing_package.update.find_one_and_update")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	var record BillingPackageMongoDBModel

	if err = coll.FindOneAndUpdate(ctx, filter, pipeline, opts).Decode(&record); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			bizErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, "BillingPackage", feeconstant.BillingPackageCollection)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "No document matched for update", bizErr)

			return nil, bizErr
		}

		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update billing package", err)

		return nil, err
	}

	entity, err := record.ToEntity()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to map updated billing package", err)

		return nil, err
	}

	return entity, nil
}

// buildUpdatePipeline translates the classic $set/$unset update document into an
// aggregation pipeline so a single FindOneAndUpdate can return the persisted document.
func buildUpdatePipeline(updateFields *bson.M) bson.A {
	pipeline := bson.A{}

	if updateFields == nil {
		return pipeline
	}

	if setFields, ok := (*updateFields)["$set"]; ok {
		pipeline = append(pipeline, bson.M{"$set": setFields})
	}

	if unsetPaths := unsetFieldPaths((*updateFields)["$unset"]); len(unsetPaths) > 0 {
		pipeline = append(pipeline, bson.M{"$unset": unsetPaths})
	}

	return pipeline
}

// unsetFieldPaths extracts field paths from a classic $unset document into the
// array form the aggregation pipeline $unset stage expects.
func unsetFieldPaths(unset any) bson.A {
	unsetMap, ok := unset.(bson.M)
	if !ok {
		return nil
	}

	paths := make(bson.A, 0, len(unsetMap))
	for path := range unsetMap {
		paths = append(paths, path)
	}

	return paths
}
