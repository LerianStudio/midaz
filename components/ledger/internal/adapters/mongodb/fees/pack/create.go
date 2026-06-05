// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// Create creates a new package in the MongoDB database.
func (pm *PackageMongoDBRepository) Create(ctx context.Context, p *Package, organizationID uuid.UUID) (*Package, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.create")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", p, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert package payload to JSON string", err)
	}

	db, err := pm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(constant.PackageCollection))
	record := &PackageMongoDBModel{}

	if err := record.FromEntity(p, organizationID); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert pack to model", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "repository.package.create.insert")
	defer spanInsert.End()

	spanInsert.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanInsert, "app.request.repository_input", record, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to convert package record from entity to JSON string", err)
	}

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to insert pack", err)

		return nil, err
	}

	return record.ToEntity(), nil
}
