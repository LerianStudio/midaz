// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"

	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"

	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// SoftDelete a package entity into mongodb.
func (pm *PackageMongoDBRepository) SoftDelete(ctx context.Context, id, organizationID, ledgerID uuid.UUID) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.delete")
	defer span.End()

	attributes := append(
		[]attribute.KeyValue{attribute.String("app.request.request_id", reqId)},
		packageScopeAttributes(id, organizationID, ledgerID)...,
	)

	span.SetAttributes(attributes...)

	db, err := pm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	coll := db.Collection(strings.ToLower(feeconstant.PackageCollection))

	_, spanDelete := tracer.Start(ctx, "repository.package.delete.delete_one")
	defer spanDelete.End()

	spanDelete.SetAttributes(attributes...)

	deletedAt := bson.D{{Key: "$set", Value: bson.D{{Key: "deleted_at", Value: time.Now()}}}}

	deleted, err := coll.UpdateOne(ctx, packageScopeFilter(id, organizationID, ledgerID), deletedAt)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete package", err)

		return err
	}

	if deleted.MatchedCount == 0 {
		bizErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", feeconstant.PackageCollection)
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanDelete, "No package found to delete", bizErr)

		return bizErr
	}

	return nil
}
