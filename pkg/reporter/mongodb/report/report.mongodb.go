// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package report

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"

	"github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"

	libMongo "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
)

// Repository provides an interface for operations related to reports collection in MongoDB.
//
//go:generate mockgen --destination=report.mongodb.mock.go --package=report --copyright_file=../../../COPYRIGHT . Repository
type Repository interface {
	UpdateReportStatusById(ctx context.Context, status string, id uuid.UUID, completedAt time.Time, metadata map[string]any) error
	Create(ctx context.Context, record *Report) (*Report, error)
	FindByID(ctx context.Context, id uuid.UUID) (*Report, error)
	FindList(ctx context.Context, filters http.QueryHeader) ([]*Report, error)
	CountAll(ctx context.Context) (int64, error)
	CountByStatus(ctx context.Context, status string, from, to time.Time) (int64, error)
}

// ReportMongoDBRepository is a MongoDB-specific implementation of the ReportRepository.
type ReportMongoDBRepository struct {
	connection *libMongo.MongoConnection
	Database   string
}

// Compile-time interface satisfaction check.
var _ Repository = (*ReportMongoDBRepository)(nil)

// NewReportMongoDBRepository returns a new instance of ReportMongoDBRepository using the given MongoDB connection.
func NewReportMongoDBRepository(mc *libMongo.MongoConnection) (*ReportMongoDBRepository, error) {
	r := &ReportMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}
	if _, err := r.connection.GetDB(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb for reports: %w", err)
	}

	return r, nil
}

// NewReportMongoDBRepositoryLazy returns a new repository without eagerly dialing MongoDB.
// This is used for multi-tenant worker mode, where the runtime path injects a tenant-scoped
// Mongo database into the context and the static connection is only a fallback.
func NewReportMongoDBRepositoryLazy(mc *libMongo.MongoConnection) (*ReportMongoDBRepository, error) {
	if mc == nil {
		return nil, fmt.Errorf("mongo connection is required")
	}

	return &ReportMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}, nil
}

// getCollection returns the MongoDB collection for reports, using tenant-scoped connection when
// available (multi-tenant mode) or falling back to the static connection (single-tenant mode).
func (rm *ReportMongoDBRepository) getCollection(ctx context.Context) (*mongo.Collection, error) {
	db, err := libMongo.ResolveDatabase(ctx, rm.connection, rm.Database)
	if err != nil {
		return nil, fmt.Errorf("resolving mongodb connection: %w", err)
	}

	return db.Collection(strings.ToLower(constant.MongoCollectionReport)), nil
}

// UpdateReportStatusById updates only the status, completedAt and metadata fields of a report document by UUID.
func (rm *ReportMongoDBRepository) UpdateReportStatusById(
	ctx context.Context,
	status string,
	id uuid.UUID,
	completedAt time.Time,
	metadata map[string]any,
) error {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.report.update_status")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.report_id", id.String()),
		attribute.String("app.request.status", status),
		attribute.String("app.request.completed_at", completedAt.String()),
	}

	span.SetAttributes(attributes...)

	coll, err := rm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	// Create a filter using the UUID directly for matching the _id field stored as BinData
	filter := bson.M{"_id": id}

	ctx, spanUpdate := tracer.Start(ctx, "repository.report.update_status_exec")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	// Create an update document with only the fields we want to update
	updateFields := bson.M{}

	if status != "" {
		updateFields["status"] = status
	}

	// Only set completedAt if it's not a zero time
	if !completedAt.IsZero() {
		updateFields["completed_at"] = completedAt
	}

	// Only set metadata if it's not nil
	if metadata != nil {
		updateFields["metadata"] = metadata
	}

	// Use $set to update only the specified fields
	update := bson.M{
		"$set": updateFields,
	}

	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update report status", err)
		return err
	}

	if result.MatchedCount == 0 {
		errNotFound := mapReportNotFound(mongo.ErrNoDocuments)
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "No report found with the provided UUID", errNotFound)

		return errNotFound
	}

	return nil
}

// Create inserts a new report entity into mongo.
func (rm *ReportMongoDBRepository) Create(ctx context.Context, report *Report) (*Report, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.report.create")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.report_id", report.ID.String()),
	}

	span.SetAttributes(attributes...)

	coll, err := rm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	record := &ReportMongoDBModel{}

	if err := record.FromEntity(report); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert report to model", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "repository.report.create_exec")

	spanInsert.SetAttributes(attributes...)

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to insert report", err)

		return nil, err
	}

	spanInsert.End()

	return record.ToEntity(report.Filters), nil
}

// FindByID retrieves a report from the mongodb using the provided entity_id.
func (rm *ReportMongoDBRepository) FindByID(ctx context.Context, id uuid.UUID) (*Report, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.report.find_by_id")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.report_id", id.String()),
	}

	span.SetAttributes(attributes...)

	coll, err := rm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	var record *ReportMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "repository.report.find_by_id_exec")

	spanFindOne.SetAttributes(attributes...)

	filter := bson.M{"_id": id, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}

	if err = coll.
		FindOne(ctx, filter).
		Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(spanFindOne, "Failed to find report by entity", err)
		return nil, mapReportNotFound(err)
	}

	if nil == record {
		libOpentelemetry.HandleSpanError(span, "Report record is nil after decode", mongo.ErrNoDocuments)
		return nil, mapReportNotFound(mongo.ErrNoDocuments)
	}

	spanFindOne.End()

	return record.ToEntityFindByID(), nil
}

// FindList retrieves all reports from the mongodb with filtering and pagination support.
func (rm *ReportMongoDBRepository) FindList(ctx context.Context, filters http.QueryHeader) ([]*Report, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.report.find_list")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
	}

	span.SetAttributes(attributes...)

	coll, err := rm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	queryFilter := bson.M{}

	// Filter by status
	if !commons.IsNilOrEmpty(&filters.Status) {
		queryFilter["status"] = filters.Status
	}

	// Filter by template_id
	if filters.TemplateID != uuid.Nil {
		queryFilter["template_id"] = filters.TemplateID
	}

	// Filter by created_at date range
	if !filters.CreatedAt.IsZero() {
		end := filters.CreatedAt.Add(constant.HoursPerDay * time.Hour)
		queryFilter["created_at"] = bson.M{
			"$gte": filters.CreatedAt,
			"$lt":  end,
		}
	}

	// Filter non-deleted records
	queryFilter["deleted_at"] = bson.D{{Key: "$eq", Value: nil}}

	// Pagination
	limit := int64(filters.Limit)
	skip := int64(filters.Page*filters.Limit - filters.Limit)
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "created_at", Value: -1}})

	ctx, spanFind := tracer.Start(ctx, "repository.report.find_list_exec")

	spanFind.SetAttributes(attributes...)

	cur, err := coll.Find(ctx, queryFilter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find reports", err)
		return nil, err
	}

	spanFind.End()

	var results []*ReportMongoDBModel

	for cur.Next(ctx) {
		var record ReportMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode report", err)
			return nil, err
		}

		results = append(results, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate reports", err)
		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to close cursor", err)
		return nil, err
	}

	reports := make([]*Report, 0, len(results))
	for i := range results {
		reports = append(reports, results[i].ToEntityFindByID())
	}

	return reports, nil
}

// CountAll returns the total number of non-deleted reports.
func (rm *ReportMongoDBRepository) CountAll(ctx context.Context) (int64, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.report.count_all")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	coll, err := rm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return 0, err
	}

	filter := bson.M{"deleted_at": bson.D{{Key: "$eq", Value: nil}}}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count all reports", err)
		return 0, err
	}

	return total, nil
}

// CountByStatus returns the total number of non-deleted reports matching the given status within a time range.
func (rm *ReportMongoDBRepository) CountByStatus(ctx context.Context, status string, from, to time.Time) (int64, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.report.count_by_status")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	coll, err := rm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return 0, err
	}

	filter := bson.M{
		"status":     status,
		"created_at": bson.M{"$gte": from, "$lt": to},
		"deleted_at": bson.D{{Key: "$eq", Value: nil}},
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count reports by status", err)
		return 0, err
	}

	return total, nil
}

// mapReportNotFound maps the MongoDB driver's not-found sentinel to the canonical
// typed not-found error at the adapter boundary, so callers receive a 404-rendering
// error instead of the raw driver error. Other errors pass through unchanged.
func mapReportNotFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, cnErr.EntityReport, constant.MongoCollectionReport)
	}

	return err
}
