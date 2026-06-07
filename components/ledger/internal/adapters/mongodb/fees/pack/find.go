// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/bsondecimal"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"

	"github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// FindList returns a list of packages based on the provided filters.
func (pm *PackageMongoDBRepository) FindList(ctx context.Context, filters http.QueryHeader) ([]*Package, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.find_list")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", filters.OrganizationID.String()),
	}

	span.SetAttributes(attributes...)

	span.SetAttributes(
		attribute.Int("app.request.limit", filters.Limit),
		attribute.Int("app.request.page", filters.Page),
		attribute.Bool("app.request.has_segment_id", filters.SegmentID != uuid.Nil),
		attribute.Bool("app.request.has_ledger_id", filters.LedgerID != uuid.Nil),
		attribute.Bool("app.request.has_transaction_route", filters.TransactionRoute != nil),
		attribute.Bool("app.request.has_enable", filters.Enable != nil),
	)

	db, err := pm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Collection(strings.ToLower(constant.PackageCollection))

	queryFilter := bson.M{}

	if !commons.IsNilOrEmpty(filters.TransactionRoute) {
		queryFilter["transaction_route"] = filters.TransactionRoute
	}

	if filters.Enable != nil {
		queryFilter["enable"] = filters.Enable
	}

	if filters.SegmentID.ID() != 0 {
		queryFilter["segment_id"] = filters.SegmentID
	}

	if filters.LedgerID.ID() != 0 {
		queryFilter["ledger_id"] = filters.LedgerID
	}

	queryFilter["organization_id"] = filters.OrganizationID
	queryFilter["deleted_at"] = bson.D{{Key: "$eq", Value: nil}}

	if filters.Page < 1 {
		filters.Page = 1
	}

	if filters.Limit < 1 {
		filters.Limit = 10
	}

	limit := int64(filters.Limit)
	skip := int64(filters.Page*filters.Limit - filters.Limit)
	opts := options.Find().SetLimit(limit).SetSkip(skip)

	ctx, spanFind := tracer.Start(ctx, "repository.package.find_list.find")

	spanFind.SetAttributes(attributes...)

	cur, err := coll.Find(ctx, queryFilter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find packages", err)
		return nil, err
	}
	defer cur.Close(ctx)

	spanFind.End()

	var results []*PackageMongoDBModel

	for cur.Next(ctx) {
		var record PackageMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode package", err)
			return nil, err
		}

		results = append(results, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate packages", err)
		return nil, err
	}

	packages := make([]*Package, 0, len(results))
	for i := range results {
		packages = append(packages, results[i].ToEntity())
	}

	return packages, nil
}

// FindByID finds a package by ID.
func (pm *PackageMongoDBRepository) FindByID(ctx context.Context, id, organizationID uuid.UUID) (*Package, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.find_by_entity")
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

	coll := db.Collection(strings.ToLower(constant.PackageCollection))

	var record *PackageMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "repository.package.find_by_entity.find_one")
	defer spanFindOne.End()

	spanFindOne.SetAttributes(attributes...)

	if err = coll.
		FindOne(ctx, bson.M{"_id": id, "organization_id": organizationID, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}).
		Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(spanFindOne, "Failed to find package by entity", err)
		return nil, err
	}

	if nil == record {
		err := mongo.ErrNoDocuments
		libOpentelemetry.HandleSpanError(span, "Package not found", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByOrganizationIDAndLedgerID recover a package by organizationID and ledgerID
func (pm *PackageMongoDBRepository) FindByOrganizationIDAndLedgerID(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*Package, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.find_by_org_and_ledger")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := pm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Collection(strings.ToLower(constant.PackageCollection))

	queryFilter := bson.M{}

	queryFilter["organization_id"] = organizationID
	queryFilter["ledger_id"] = ledgerID
	queryFilter["deleted_at"] = bson.D{{Key: "$eq", Value: nil}}
	queryFilter["enable"] = bson.D{{Key: "$eq", Value: true}}

	ctx, spanFind := tracer.Start(ctx, "repository.package.find_by_org_and_ledger.find")

	spanFind.SetAttributes(attributes...)

	cur, err := coll.Find(ctx, queryFilter)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, fmt.Sprintf("Failed to find packages by organizationID %v and ledgerId %v", organizationID, ledgerID), err)
		return nil, err
	}
	defer cur.Close(ctx)

	spanFind.End()

	var results []*PackageMongoDBModel

	for cur.Next(ctx) {
		var record PackageMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode package", err)
			return nil, err
		}

		results = append(results, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate packages", err)
		return nil, err
	}

	packages := make([]*Package, 0, len(results))
	for i := range results {
		packages = append(packages, results[i].ToEntity())
	}

	return packages, nil
}

// FindFeesAndAmountDataByPackageID find fees and amount data by package id
func (pm *PackageMongoDBRepository) FindFeesAndAmountDataByPackageID(ctx context.Context, organizationID, packageID uuid.UUID) (*model.AmountData, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.package.find_fees_by_package_id")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", packageID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := pm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Collection(strings.ToLower(constant.PackageCollection))

	filter := bson.M{
		"_id":             packageID,
		"organization_id": organizationID,
		"deleted_at":      bson.D{{Key: "$eq", Value: nil}},
	}

	// return only fee of a package
	projection := bson.M{
		"fees":              1,
		"minimum_amount":    1,
		"maximum_amount":    1,
		"ledger_id":         1,
		"segment_id":        1,
		"transaction_route": 1,
	}

	var result struct {
		Fees             map[string]Fee      `bson:"fees"`
		MinAmount        bsondecimal.Decimal `bson:"minimum_amount"`
		MaxAmount        bsondecimal.Decimal `bson:"maximum_amount"`
		LedgerID         uuid.UUID           `bson:"ledger_id"`
		SegmentID        *uuid.UUID          `bson:"segment_id"`
		TransactionRoute *string             `bson:"transaction_route"`
	}

	ctx, spanFindOne := tracer.Start(ctx, "repository.package.find_fees_by_package_id.find_one")
	defer spanFindOne.End()

	spanFindOne.SetAttributes(attributes...)

	err = coll.FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", reflect.TypeOf(Package{}).Name())
		}

		libOpentelemetry.HandleSpanError(span, "Failed to find fees by package ID", err)

		return nil, err
	}

	amountData := &model.AmountData{
		MinAmount:        result.MinAmount.Decimal,
		MaxAmount:        result.MaxAmount.Decimal,
		Fees:             ToEntityFeeMap(result.Fees),
		LedgerID:         result.LedgerID,
		SegmentID:        result.SegmentID,
		TransactionRoute: result.TransactionRoute,
	}

	return amountData, nil
}
