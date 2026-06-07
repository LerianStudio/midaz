// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

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
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// SoftDelete a package entity into mongodb.
func (pm *PackageMongoDBRepository) SoftDelete(ctx context.Context, id, organizationID uuid.UUID) error {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.delete")
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

		return err
	}

	coll := db.Collection(strings.ToLower(constant.PackageCollection))

	ctx, spanDelete := tracer.Start(ctx, "repository.package.delete.delete_one")
	defer spanDelete.End()

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "organization_id", Value: organizationID},
		{Key: "deleted_at", Value: bson.M{"$eq": nil}},
	}
	deletedAt := bson.D{{Key: "$set", Value: bson.D{{Key: "deleted_at", Value: time.Now()}}}}

	deleted, err := coll.UpdateOne(ctx, filter, deletedAt)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete package", err)

		return err
	}

	if deleted.MatchedCount == 0 {
		libOpentelemetry.HandleSpanError(spanDelete, "No package found to delete", mongo.ErrNoDocuments)
		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.PackageCollection)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Return from delete one: %v", deleted))

	return nil
}
