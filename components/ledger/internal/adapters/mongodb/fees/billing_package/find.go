// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// FindByID finds a billing package by ID and organization ID.
func (r *BillingPackageMongoDBRepository) FindByID(ctx context.Context, id string, organizationID string) (*model.BillingPackage, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.billing_package.find_by_id")
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

	coll := db.Collection(strings.ToLower(constant.BillingPackageCollection))

	var record BillingPackageMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "repository.billing_package.find_by_id.find_one")
	defer spanFindOne.End()

	spanFindOne.SetAttributes(attributes...)

	filter := bson.M{
		"_id":             id,
		"organization_id": organizationID,
		"deleted_at":      bson.M{"$eq": nil},
	}

	if err = coll.FindOne(ctx, filter).Decode(&record); err != nil {
		if err == mongo.ErrNoDocuments {
			libOpentelemetry.HandleSpanError(spanFindOne, "Billing package not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanFindOne, "Failed to find billing package by ID", err)

		return nil, err
	}

	entity, err := record.ToEntity()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFindOne, "Failed to convert billing package record to entity", err)

		return nil, err
	}

	return entity, nil
}

// FindAll returns a paginated list of billing packages for an organization and ledger.
func (r *BillingPackageMongoDBRepository) FindAll(ctx context.Context, organizationID, ledgerID, billingType string, limit, page int) ([]*model.BillingPackage, int64, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.billing_package.find_all")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.ledger_id", ledgerID),
		attribute.Int("app.request.limit", limit),
		attribute.Int("app.request.page", page),
	}

	span.SetAttributes(attributes...)

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, 0, err
	}

	coll := db.Collection(strings.ToLower(constant.BillingPackageCollection))

	queryFilter := bson.M{
		"organization_id": organizationID,
		"deleted_at":      bson.M{"$eq": nil},
	}

	if ledgerID != "" {
		queryFilter["ledger_id"] = ledgerID
	}

	if billingType != "" {
		queryFilter["type"] = billingType
	}

	// Count total documents
	ctx, spanCount := tracer.Start(ctx, "repository.billing_package.find_all.count")

	spanCount.SetAttributes(attributes...)

	total, err := coll.CountDocuments(ctx, queryFilter)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanCount, "Failed to count billing packages", err)
		spanCount.End()

		return nil, 0, err
	}

	spanCount.End()

	if page < 1 {
		page = 1
	}

	if limit < 1 {
		limit = 10
	}

	findLimit := int64(limit)
	skip := int64(page*limit - limit)

	opts := options.Find().
		SetLimit(findLimit).
		SetSkip(skip).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	ctx, spanFind := tracer.Start(ctx, "repository.billing_package.find_all.find")

	spanFind.SetAttributes(attributes...)

	cur, err := coll.Find(ctx, queryFilter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find billing packages", err)
		spanFind.End()

		return nil, 0, err
	}
	defer cur.Close(ctx)

	spanFind.End()

	var results []*BillingPackageMongoDBModel

	for cur.Next(ctx) {
		var record BillingPackageMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode billing package", err)

			return nil, 0, err
		}

		results = append(results, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate billing packages", err)

		return nil, 0, err
	}

	packages := make([]*model.BillingPackage, 0, len(results))
	for i := range results {
		entity, err := results[i].ToEntity()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to convert billing package record to entity", err)

			return nil, 0, err
		}

		packages = append(packages, entity)
	}

	return packages, total, nil
}

// FindMatchingPackages finds enabled billing packages matching a specific transaction route.
func (r *BillingPackageMongoDBRepository) FindMatchingPackages(ctx context.Context, orgID, ledgerID, transactionRouteID string) ([]*model.BillingPackage, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.billing_package.find_matching_packages")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", orgID),
		attribute.String("app.request.ledger_id", ledgerID),
		attribute.String("app.request.transaction_route", transactionRouteID),
	}

	span.SetAttributes(attributes...)

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(constant.BillingPackageCollection))

	queryFilter := bson.M{
		"organization_id":                orgID,
		"ledger_id":                      ledgerID,
		"event_filter.transaction_route": transactionRouteID,
		"enable":                         true,
		"deleted_at":                     bson.M{"$eq": nil},
	}

	ctx, spanFind := tracer.Start(ctx, "repository.billing_package.find_matching_packages.find")

	spanFind.SetAttributes(attributes...)

	cur, err := coll.Find(ctx, queryFilter)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find matching billing packages", err)
		spanFind.End()

		return nil, err
	}
	defer cur.Close(ctx)

	spanFind.End()

	var results []*BillingPackageMongoDBModel

	for cur.Next(ctx) {
		var record BillingPackageMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode billing package", err)

			return nil, err
		}

		results = append(results, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate billing packages", err)

		return nil, err
	}

	packages := make([]*model.BillingPackage, 0, len(results))
	for i := range results {
		entity, err := results[i].ToEntity()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to convert billing package record to entity", err)

			return nil, err
		}

		packages = append(packages, entity)
	}

	return packages, nil
}

// FindActiveByType finds enabled billing packages by type for an organization and ledger.
func (r *BillingPackageMongoDBRepository) FindActiveByType(ctx context.Context, orgID, ledgerID string, billingType string) ([]*model.BillingPackage, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.billing_package.find_active_by_type")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", orgID),
		attribute.String("app.request.ledger_id", ledgerID),
		attribute.String("app.request.billing_type", billingType),
	}

	span.SetAttributes(attributes...)

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(constant.BillingPackageCollection))

	queryFilter := bson.M{
		"organization_id": orgID,
		"ledger_id":       ledgerID,
		"type":            billingType,
		"enable":          true,
		"deleted_at":      bson.M{"$eq": nil},
	}

	ctx, spanFind := tracer.Start(ctx, "repository.billing_package.find_active_by_type.find")

	spanFind.SetAttributes(attributes...)

	cur, err := coll.Find(ctx, queryFilter)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find active billing packages by type", err)
		spanFind.End()

		return nil, err
	}
	defer cur.Close(ctx)

	spanFind.End()

	var results []*BillingPackageMongoDBModel

	for cur.Next(ctx) {
		var record BillingPackageMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode billing package", err)

			return nil, err
		}

		results = append(results, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate billing packages", err)

		return nil, err
	}

	packages := make([]*model.BillingPackage, 0, len(results))
	for i := range results {
		entity, err := results[i].ToEntity()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to convert billing package record to entity", err)

			return nil, err
		}

		packages = append(packages, entity)
	}

	return packages, nil
}
