// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"
	"fmt"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// SoftDelete performs a soft delete on a billing package entity in MongoDB.
func (r *BillingPackageMongoDBRepository) SoftDelete(ctx context.Context, id string, organizationID string) error {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.billing_package.delete")
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

		return err
	}

	coll := db.Collection(strings.ToLower(constant.BillingPackageCollection))

	ctx, spanDelete := tracer.Start(ctx, "repository.billing_package.delete.update_one")
	defer spanDelete.End()

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "organization_id", Value: organizationID},
		{Key: "deleted_at", Value: bson.M{"$eq": nil}},
	}

	now := time.Now().UTC().Format(time.RFC3339)
	deletedAt := bson.D{{Key: "$set", Value: bson.D{{Key: "deleted_at", Value: now}}}}

	err = libOpentelemetry.SetSpanAttributesFromValue(spanDelete, "app.request.filter", filter, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanDelete, "Failed to convert filter to JSON string", err)
	}

	deleted, err := coll.UpdateOne(ctx, filter, deletedAt)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete billing package", err)

		return err
	}

	if deleted.MatchedCount == 0 {
		libOpentelemetry.HandleSpanError(spanDelete, "No billing package found to delete", mongo.ErrNoDocuments)

		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "BillingPackage", constant.BillingPackageCollection)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Soft deleted billing package: id=%s, org=%s", id, organizationID))

	return nil
}
