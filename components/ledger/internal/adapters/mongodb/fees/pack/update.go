// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"errors"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"

	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"

	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Update updates a package in the database and returns the persisted document.
func (pm *PackageMongoDBRepository) Update(ctx context.Context, id, organizationID, ledgerID uuid.UUID, updatedAt time.Time, updateFields *bson.M) (*Package, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.update")
	defer span.End()

	attributes := append(
		[]attribute.KeyValue{attribute.String("app.request.request_id", reqId)},
		packageScopeAttributes(id, organizationID, ledgerID)...,
	)

	span.SetAttributes(attributes...)

	db, err := pm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Collection(strings.ToLower(feeconstant.PackageCollection))

	filter := packageScopeFilter(id, organizationID, ledgerID)
	filter["updated_at"] = updatedAt
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	_, spanUpdate := tracer.Start(ctx, "repository.package.update.find_one_and_update")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	var record PackageMongoDBModel

	if err = coll.FindOneAndUpdate(ctx, filter, updateFields, opts).Decode(&record); err != nil {
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
