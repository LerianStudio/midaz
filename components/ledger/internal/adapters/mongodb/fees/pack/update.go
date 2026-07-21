// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"errors"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Update updates a package in the database and returns the persisted document.
func (pm *PackageMongoDBRepository) Update(ctx context.Context, id, organizationID uuid.UUID, updateFields *bson.M) (*Package, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.update")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	}

	span.SetAttributes(attributes...)

	db, err := pm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Collection(strings.ToLower(feeconstant.PackageCollection))

	filter := bson.M{"_id": id, "organization_id": organizationID, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}
	pipeline := buildUpdatePipeline(updateFields)
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	_, spanUpdate := tracer.Start(ctx, "repository.package.update.find_one_and_update")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	var record PackageMongoDBModel

	if err = coll.FindOneAndUpdate(ctx, filter, pipeline, opts).Decode(&record); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			bizErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", feeconstant.PackageCollection)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "No document matched for update", bizErr)

			return nil, bizErr
		}

		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update package", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// buildUpdatePipeline translates the classic $set/$unset update document into an
// aggregation pipeline and appends the auto-disable stage, so a single
// FindOneAndUpdate reflects the fees change and its enable side effect in one
// atomic write. The final stage sets enable to false when the resulting fees map
// is empty, and otherwise leaves it as the value produced by the preceding stages.
func buildUpdatePipeline(updateFields *bson.M) bson.A {
	pipeline := bson.A{}

	if updateFields != nil {
		if setFields, ok := (*updateFields)["$set"]; ok {
			pipeline = append(pipeline, bson.M{"$set": setFields})
		}

		if unsetPaths := unsetFieldPaths((*updateFields)["$unset"]); len(unsetPaths) > 0 {
			pipeline = append(pipeline, bson.M{"$unset": unsetPaths})
		}
	}

	autoDisable := bson.M{"$set": bson.M{"enable": bson.M{"$cond": bson.A{
		bson.M{"$eq": bson.A{
			bson.M{"$size": bson.M{"$objectToArray": bson.M{"$ifNull": bson.A{"$fees", bson.M{}}}}}, 0,
		}},
		false,
		"$enable",
	}}}}

	return append(pipeline, autoDisable)
}

// unsetFieldPaths extracts the field paths from a classic $unset document (a map
// whose keys are the paths to remove) into the array form the aggregation
// pipeline $unset stage expects.
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
