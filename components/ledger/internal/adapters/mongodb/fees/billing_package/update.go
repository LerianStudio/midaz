// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Update updates a billing package in the database.
func (r *BillingPackageMongoDBRepository) Update(ctx context.Context, id string, organizationID string, updateFields *bson.M) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.billing_package.update")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", updateFields, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert billing package update fields to JSON string", err)
	}

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	coll := db.Collection(strings.ToLower(constant.BillingPackageCollection))
	opts := options.UpdateOne().SetUpsert(false)

	ctx, spanUpdate := tracer.Start(ctx, "repository.billing_package.update.update_one")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanUpdate, "app.request.repository_input", updateFields, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to convert billing package update fields to JSON string", err)
	}

	filter := bson.M{
		"_id":             id,
		"organization_id": organizationID,
		"deleted_at":      bson.M{"$eq": nil},
	}

	result, err := coll.UpdateOne(ctx, filter, updateFields, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update billing package", err)

		return err
	}

	if result.MatchedCount == 0 {
		libOpentelemetry.HandleSpanError(spanUpdate, "No document matched for update", mongo.ErrNoDocuments)

		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "BillingPackage", constant.BillingPackageCollection)
	}

	return nil
}
