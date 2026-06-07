// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package extraction

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"

	libMongo "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
)

// Repository provides CRUD operations for ExtractionMapping documents in MongoDB.
//
//go:generate mockgen --destination=extraction.mongodb.mock.go --package=extraction --copyright_file=../../../COPYRIGHT . Repository
type Repository interface {
	Create(ctx context.Context, mapping *datasource.ExtractionMapping) error
	FindByJobID(ctx context.Context, jobID string) (*datasource.ExtractionMapping, error)
	FindByReportID(ctx context.Context, reportID string) (*datasource.ExtractionMapping, error)
	FindStalePending(ctx context.Context, threshold time.Duration) ([]*datasource.ExtractionMapping, error)
	FindStaleProcessing(ctx context.Context, threshold time.Duration) ([]*datasource.ExtractionMapping, error)
	UpdateStatus(ctx context.Context, jobID string, status string, completedAt *time.Time) error
	AtomicClaimPending(ctx context.Context, jobID string) (bool, error)
}

// ExtractionMappingMongoDBRepository is a MongoDB-specific implementation of the Repository interface.
type ExtractionMappingMongoDBRepository struct {
	connection *libMongo.MongoConnection
	Database   string
}

// Compile-time interface satisfaction check.
var _ Repository = (*ExtractionMappingMongoDBRepository)(nil)

// NewExtractionMappingMongoDBRepository returns a new repository backed by the given MongoDB connection.
func NewExtractionMappingMongoDBRepository(mc *libMongo.MongoConnection) (*ExtractionMappingMongoDBRepository, error) {
	if mc == nil {
		return nil, fmt.Errorf("mongo connection is required")
	}

	r := &ExtractionMappingMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}

	if _, err := r.connection.GetDB(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb for extraction mappings: %w", err)
	}

	return r, nil
}

// NewExtractionMappingMongoDBRepositoryLazy returns a new repository without eagerly dialing MongoDB.
// Used for multi-tenant worker mode where the runtime path injects a tenant-scoped Mongo database.
func NewExtractionMappingMongoDBRepositoryLazy(mc *libMongo.MongoConnection) (*ExtractionMappingMongoDBRepository, error) {
	if mc == nil {
		return nil, fmt.Errorf("mongo connection is required")
	}

	return &ExtractionMappingMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}, nil
}

// getCollection returns the MongoDB collection for extraction mappings, using tenant-scoped
// connection when available or falling back to the static connection.
func (r *ExtractionMappingMongoDBRepository) getCollection(ctx context.Context) (*mongo.Collection, error) {
	db, err := libMongo.ResolveDatabase(ctx, r.connection, r.Database)
	if err != nil {
		return nil, fmt.Errorf("resolving mongodb connection: %w", err)
	}

	return db.Collection(strings.ToLower(constant.MongoCollectionExtractionMapping)), nil
}

// Create inserts a new ExtractionMapping document into MongoDB.
func (r *ExtractionMappingMongoDBRepository) Create(ctx context.Context, mapping *datasource.ExtractionMapping) error {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.extraction_mapping.create")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.job_id", mapping.JobID),
		attribute.String("app.request.report_id", mapping.ReportID),
	)

	coll, err := r.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	model := &ExtractionMappingMongoDBModel{}
	model.FromEntity(mapping)

	_, spanInsert := tracer.Start(ctx, "repository.extraction_mapping.create_exec")
	defer spanInsert.End()

	spanInsert.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.job_id", mapping.JobID),
	)

	_, err = coll.InsertOne(ctx, model)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to insert extraction mapping", err)
		return err
	}

	return nil
}

// FindByJobID retrieves an ExtractionMapping by its Fetcher job ID.
func (r *ExtractionMappingMongoDBRepository) FindByJobID(ctx context.Context, jobID string) (*datasource.ExtractionMapping, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.extraction_mapping.find_by_job_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.job_id", jobID),
	)

	coll, err := r.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	filter := bson.M{"job_id": jobID}

	var model ExtractionMappingMongoDBModel

	_, spanFind := tracer.Start(ctx, "repository.extraction_mapping.find_by_job_id_exec")
	defer spanFind.End()

	spanFind.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.job_id", jobID),
	)

	if err := coll.FindOne(ctx, filter).Decode(&model); err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find extraction mapping by job ID", err)
		return nil, err
	}

	return model.ToEntity(), nil
}

// FindByReportID retrieves an ExtractionMapping by its associated report ID.
func (r *ExtractionMappingMongoDBRepository) FindByReportID(ctx context.Context, reportID string) (*datasource.ExtractionMapping, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.extraction_mapping.find_by_report_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.report_id", reportID),
	)

	coll, err := r.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	filter := bson.M{"report_id": reportID}

	var model ExtractionMappingMongoDBModel

	_, spanFind := tracer.Start(ctx, "repository.extraction_mapping.find_by_report_id_exec")
	defer spanFind.End()

	spanFind.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.report_id", reportID),
	)

	if err := coll.FindOne(ctx, filter).Decode(&model); err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find extraction mapping by report ID", err)
		return nil, err
	}

	return model.ToEntity(), nil
}

// FindStalePending returns all ExtractionMappings with status=pending and created_at older than
// the given threshold duration. Used by the reconciler to detect stuck extractions.
func (r *ExtractionMappingMongoDBRepository) FindStalePending(ctx context.Context, threshold time.Duration) ([]*datasource.ExtractionMapping, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.extraction_mapping.find_stale_pending")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.threshold", threshold.String()),
	)

	coll, err := r.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	cutoff := time.Now().Add(-threshold)

	filter := bson.M{
		"status":     constant.ExtractionStatusPending,
		"created_at": bson.M{"$lt": cutoff},
	}

	_, spanFind := tracer.Start(ctx, "repository.extraction_mapping.find_stale_pending_exec")
	defer spanFind.End()

	spanFind.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.cutoff", cutoff.Format(time.RFC3339)),
	)

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find stale pending extraction mappings", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	var models []ExtractionMappingMongoDBModel
	if err := cursor.All(ctx, &models); err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to decode stale pending extraction mappings", err)
		return nil, err
	}

	results := make([]*datasource.ExtractionMapping, 0, len(models))
	for i := range models {
		results = append(results, models[i].ToEntity())
	}

	return results, nil
}

// FindStaleProcessing returns all ExtractionMappings with status=processing and created_at older than
// the given threshold duration. Used by the reconciler to recover mappings that were claimed by a
// worker but never completed (e.g., worker crash after AtomicClaimPending).
func (r *ExtractionMappingMongoDBRepository) FindStaleProcessing(ctx context.Context, threshold time.Duration) ([]*datasource.ExtractionMapping, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.extraction_mapping.find_stale_processing")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.threshold", threshold.String()),
	)

	coll, err := r.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	cutoff := time.Now().Add(-threshold)

	filter := bson.M{
		"status":     constant.ExtractionStatusProcessing,
		"created_at": bson.M{"$lt": cutoff},
	}

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find stale processing extraction mappings", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	var models []ExtractionMappingMongoDBModel
	if err := cursor.All(ctx, &models); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode stale processing extraction mappings", err)
		return nil, err
	}

	results := make([]*datasource.ExtractionMapping, 0, len(models))
	for i := range models {
		results = append(results, models[i].ToEntity())
	}

	return results, nil
}

// UpdateStatus updates the status and optional completedAt timestamp for an extraction mapping.
func (r *ExtractionMappingMongoDBRepository) UpdateStatus(ctx context.Context, jobID string, status string, completedAt *time.Time) error {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.extraction_mapping.update_status")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.job_id", jobID),
		attribute.String("app.request.status", status),
	)

	coll, err := r.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	filter := bson.M{"job_id": jobID}

	updateFields := bson.M{"status": status}
	if completedAt != nil {
		updateFields["completed_at"] = *completedAt
	}

	update := bson.M{"$set": updateFields}

	_, spanUpdate := tracer.Start(ctx, "repository.extraction_mapping.update_status_exec")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.job_id", jobID),
	)

	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update extraction mapping status", err)
		return err
	}

	if result.MatchedCount == 0 {
		err := fmt.Errorf("no extraction mapping found for job ID: %s", jobID)
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "Extraction mapping not found", err)

		return err
	}

	return nil
}
