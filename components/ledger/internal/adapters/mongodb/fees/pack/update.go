// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Update updates a package in the database
func (pm *PackageMongoDBRepository) Update(ctx context.Context, id, organizationID uuid.UUID, updateFields *bson.M) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.update")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", updateFields, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert package record from entity to JSON string", err)
	}

	db, err := pm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	coll := db.Collection(strings.ToLower(constant.PackageCollection))
	opts := options.UpdateOne().SetUpsert(false)

	ctx, spanUpdate := tracer.Start(ctx, "repository.package.update.update_one")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanUpdate, "app.request.repository_input", updateFields, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to convert package record from entity to JSON string", err)
	}

	result, err := coll.UpdateOne(ctx, bson.M{"_id": id, "organization_id": organizationID, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}, updateFields, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update package", err)
		return err
	}

	if result.MatchedCount == 0 {
		libOpentelemetry.HandleSpanError(spanUpdate, "No document matched for update", mongo.ErrNoDocuments)
		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.PackageCollection)
	}

	ctx, spanUpdateEnable := tracer.Start(ctx, "repository.package.update.enable")
	defer spanUpdateEnable.End()

	spanUpdateEnable.SetAttributes(attributes...)

	// Update flag if needed to false when fees does not exist
	updateEnableFilter := bson.M{
		"_id":             id,
		"organization_id": organizationID,
		"deleted_at":      bson.D{{Key: "$eq", Value: nil}},
		"$expr": bson.M{
			"$eq": bson.A{
				bson.M{"$size": bson.M{"$objectToArray": "$fees"}}, 0,
			},
		},
	}

	updateEnable := bson.M{"$set": bson.M{"enable": false}}

	err = libOpentelemetry.SetSpanAttributesFromValue(spanUpdateEnable, "app.request.repository_input", updateEnable, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdateEnable, "Failed to convert package record from entity to JSON string", err)
	}

	_, err = coll.UpdateOne(ctx, updateEnableFilter, updateEnable)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdateEnable, "Failed to update enable flag", err)
		return err
	}

	return nil
}
