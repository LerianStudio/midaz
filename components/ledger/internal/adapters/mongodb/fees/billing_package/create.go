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
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	"go.opentelemetry.io/otel/attribute"
)

// Create creates a new billing package in the MongoDB database.
func (r *BillingPackageMongoDBRepository) Create(ctx context.Context, bp *model.BillingPackage) (*model.BillingPackage, error) {
	if bp == nil {
		return nil, errors.New("billing package cannot be nil")
	}

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.billing_package.create")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", bp.OrganizationID),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", bp, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert billing package payload to JSON string", err)
	}

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(constant.BillingPackageCollection))
	record := &BillingPackageMongoDBModel{}
	record.FromEntity(bp)

	ctx, spanInsert := tracer.Start(ctx, "repository.billing_package.create.insert")
	defer spanInsert.End()

	spanInsert.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanInsert, "app.request.repository_input", record, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to convert billing package record to JSON string", err)
	}

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to insert billing package", err)

		return nil, err
	}

	entity, err := record.ToEntity()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to convert billing package record to entity", err)

		return nil, err
	}

	return entity, nil
}
