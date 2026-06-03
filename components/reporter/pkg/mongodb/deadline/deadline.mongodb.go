// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package deadline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/ctxutil"
	"github.com/LerianStudio/reporter/pkg/net/http"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libMongo "github.com/LerianStudio/reporter/pkg/mongodb"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// applyStatusFilter translates a computed status into MongoDB query conditions.
// Status is not stored in MongoDB — it is derived from due_date and delivered_at.
//   - "delivered": delivered_at is not nil
//   - "overdue": due_date < now AND delivered_at is nil
//   - "pending": due_date >= now AND delivered_at is nil
//
// Uses $and to avoid overwriting due_date filters set by start_date/end_date.
func applyStatusFilter(queryFilter bson.M, status string) {
	now := time.Now()

	switch status {
	case StatusDelivered:
		queryFilter["delivered_at"] = bson.M{"$ne": nil}
	case StatusOverdue:
		queryFilter["delivered_at"] = nil

		existing, hasAndConditions := queryFilter["$and"].(bson.A)
		if !hasAndConditions {
			existing = bson.A{}
		}

		queryFilter["$and"] = append(existing, bson.M{"due_date": bson.M{"$lt": now}})
	case StatusPending:
		queryFilter["delivered_at"] = nil

		existing, hasAndConditions := queryFilter["$and"].(bson.A)
		if !hasAndConditions {
			existing = bson.A{}
		}

		queryFilter["$and"] = append(existing, bson.M{"due_date": bson.M{"$gte": now}})
	}
}

// DeadlineMongoDBRepository is a MongoDB-specific implementation of the Repository.
type DeadlineMongoDBRepository struct {
	connection *libMongo.MongoConnection
	Database   string
}

// Compile-time interface satisfaction check.
var _ Repository = (*DeadlineMongoDBRepository)(nil)

// NewDeadlineMongoDBRepository returns a new instance of DeadlineMongoDBRepository using the given MongoDB connection.
func NewDeadlineMongoDBRepository(mc *libMongo.MongoConnection) (*DeadlineMongoDBRepository, error) {
	r := &DeadlineMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}
	if _, err := r.connection.GetDB(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb for deadlines: %w", err)
	}

	return r, nil
}

// NewDeadlineMongoDBRepositoryLazy returns a new repository without eagerly dialing MongoDB.
// This is used for multi-tenant manager mode, where tenant-scoped Mongo databases are resolved
// at request time and the static connection is only a fallback.
func NewDeadlineMongoDBRepositoryLazy(mc *libMongo.MongoConnection) (*DeadlineMongoDBRepository, error) {
	if mc == nil {
		return nil, fmt.Errorf("mongo connection is required")
	}

	return &DeadlineMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}, nil
}

// getCollection returns the MongoDB collection for deadlines, using tenant-scoped connection when
// available (multi-tenant mode) or falling back to the static connection (single-tenant mode).
func (dr *DeadlineMongoDBRepository) getCollection(ctx context.Context) (*mongo.Collection, error) {
	db, err := libMongo.ResolveDatabase(ctx, dr.connection, dr.Database)
	if err != nil {
		return nil, fmt.Errorf("resolving mongodb connection: %w", err)
	}

	return db.Collection(strings.ToLower(constant.MongoCollectionDeadline)), nil
}

// FindByID retrieves a deadline from MongoDB using the provided entity_id.
func (dr *DeadlineMongoDBRepository) FindByID(ctx context.Context, id uuid.UUID) (*Deadline, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.find_by_id")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.deadline_id", id.String()),
	}

	span.SetAttributes(attributes...)

	coll, err := dr.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	var record *DeadlineMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "repository.deadline.find_by_id_exec")

	spanFindOne.SetAttributes(attributes...)

	filter := bson.M{"_id": id, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}

	if err = coll.
		FindOne(ctx, filter).
		Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(spanFindOne, "Failed to find deadline by entity", err)
		return nil, err
	}

	if nil == record {
		libOpentelemetry.HandleSpanError(span, "Deadline record is nil after decode", err)
		return nil, mongo.ErrNoDocuments
	}

	spanFindOne.End()

	return record.ToEntity(), nil
}

// FindList retrieves all deadlines from MongoDB using the provided filters.
func (dr *DeadlineMongoDBRepository) FindList(ctx context.Context, filters http.QueryHeader) ([]*Deadline, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.find_list")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", filters, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert filters to JSON string", err)
	}

	coll, err := dr.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	queryFilter := bson.M{}

	queryFilter["deleted_at"] = bson.D{{Key: "$eq", Value: nil}}

	if filters.Active != nil {
		queryFilter["active"] = *filters.Active
	}

	if filters.Type != "" {
		queryFilter["type"] = filters.Type
	}

	applyStatusFilter(queryFilter, filters.Status)

	if !filters.StartDate.IsZero() || !filters.EndDate.IsZero() {
		dueDateFilter := bson.M{}

		if !filters.StartDate.IsZero() {
			dueDateFilter["$gte"] = filters.StartDate
		}

		if !filters.EndDate.IsZero() {
			// Add 1 day to make endDate inclusive
			dueDateFilter["$lt"] = filters.EndDate.Add(constant.HoursPerDay * time.Hour)
		}

		queryFilter["due_date"] = dueDateFilter
	}

	limit := int64(filters.Limit)
	skip := int64(filters.Page*filters.Limit - filters.Limit)
	opts := options.Find().SetLimit(limit).SetSkip(skip)

	ctx, spanFind := tracer.Start(ctx, "repository.deadline.find_list_exec")

	spanFind.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanFind, "app.request.repository_filter", filters, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to convert filters to JSON string", err)
	}

	cur, err := coll.Find(ctx, queryFilter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find deadlines", err)
		return nil, err
	}

	defer cur.Close(ctx)

	spanFind.End()

	var results []*DeadlineMongoDBModel

	for cur.Next(ctx) {
		var record DeadlineMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode deadline", err)
			return nil, err
		}

		results = append(results, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate deadlines", err)
		return nil, err
	}

	deadlines := make([]*Deadline, 0, len(results))
	for i := range results {
		deadlines = append(deadlines, results[i].ToEntity())
	}

	return deadlines, nil
}

// Count returns the total number of non-deleted deadlines matching the given filters.
func (dr *DeadlineMongoDBRepository) Count(ctx context.Context, filters http.QueryHeader) (int64, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.count")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
	)

	coll, err := dr.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return 0, err
	}

	queryFilter := bson.M{}
	queryFilter["deleted_at"] = bson.D{{Key: "$eq", Value: nil}}

	if filters.Active != nil {
		queryFilter["active"] = *filters.Active
	}

	if filters.Type != "" {
		queryFilter["type"] = filters.Type
	}

	applyStatusFilter(queryFilter, filters.Status)

	if !filters.StartDate.IsZero() || !filters.EndDate.IsZero() {
		dueDateFilter := bson.M{}

		if !filters.StartDate.IsZero() {
			dueDateFilter["$gte"] = filters.StartDate
		}

		if !filters.EndDate.IsZero() {
			dueDateFilter["$lt"] = filters.EndDate.Add(constant.HoursPerDay * time.Hour)
		}

		queryFilter["due_date"] = dueDateFilter
	}

	ctx, spanCount := tracer.Start(ctx, "repository.deadline.count_exec")
	spanCount.SetAttributes(
		attribute.String("app.request.request_id", reqID),
	)

	total, err := coll.CountDocuments(ctx, queryFilter)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanCount, "Failed to count deadlines", err)
		return 0, err
	}

	spanCount.End()

	return total, nil
}

// Create inserts a new deadline entity into MongoDB.
func (dr *DeadlineMongoDBRepository) Create(ctx context.Context, record *DeadlineMongoDBModel) (*Deadline, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.create")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", record, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert deadline record to JSON string", err)
	}

	coll, err := dr.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "repository.deadline.create_exec")

	spanInsert.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanInsert, "app.request.repository_input", record, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to convert deadline record to JSON string", err)
	}

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to insert deadline", err)

		return nil, err
	}

	spanInsert.End()

	return record.ToEntity(), nil
}

// Update a deadline entity in MongoDB.
func (dr *DeadlineMongoDBRepository) Update(ctx context.Context, id uuid.UUID, updateFields *bson.M) error {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.update")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.deadline_id", id.String()),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", updateFields, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert deadline record to JSON string", err)
	}

	coll, err := dr.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	opts := options.UpdateOne().SetUpsert(false)

	ctx, spanUpdate := tracer.Start(ctx, "repository.deadline.update_exec")

	spanUpdate.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanUpdate, "app.request.repository_input", updateFields, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to convert deadline record to JSON string", err)
	}

	filter := bson.M{"_id": id, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}

	result, err := coll.UpdateOne(ctx, filter, updateFields, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update deadline", err)
		return err
	}

	if result.MatchedCount == 0 {
		spanUpdate.End()
		return mongo.ErrNoDocuments
	}

	spanUpdate.End()

	return nil
}

// Delete performs a soft delete on a deadline entity by setting deleted_at.
func (dr *DeadlineMongoDBRepository) Delete(ctx context.Context, id uuid.UUID) error {
	logger := dr.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.delete")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.deadline_id", id.String()),
	}

	span.SetAttributes(attributes...)

	coll, err := dr.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	ctx, spanDelete := tracer.Start(ctx, "repository.deadline.delete_exec")

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "deleted_at", Value: nil},
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "deleted_at", Value: time.Now()},
		}},
	}

	updateResult, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanDelete, "Failed to soft delete deadline", err)

		return err
	}

	if updateResult.MatchedCount == 0 {
		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.MongoCollectionDeadline)
	}

	spanDelete.End()

	logger.Log(ctx, log.LevelInfo, fmt.Sprint("Deleted a deadline with id: ", id.String()))

	return nil
}

// DeleteByTemplateID soft-deletes all deadlines linked to the given templateID by setting
// deleted_at on every document where template_id matches and deleted_at is nil.
// Returns the number of documents affected.
func (dr *DeadlineMongoDBRepository) DeleteByTemplateID(ctx context.Context, templateID uuid.UUID) (int64, error) {
	logger := dr.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.delete_by_template_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.template_id", templateID.String()),
	)

	coll, err := dr.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return 0, err
	}

	filter := bson.D{
		{Key: "template_id", Value: templateID},
		{Key: "deleted_at", Value: nil},
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "deleted_at", Value: time.Now()},
		}},
	}

	result, err := coll.UpdateMany(ctx, filter, update)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to soft delete deadlines by template_id", err)

		return 0, err
	}

	span.SetAttributes(attribute.Int64("app.request.deadlines_deleted", result.ModifiedCount))
	logger.Log(ctx, log.LevelInfo, "Soft-deleted deadlines by template_id",
		log.String("template_id", templateID.String()),
		log.Any("count", result.ModifiedCount),
	)

	return result.ModifiedCount, nil
}

// FindActiveNotifiable returns all deadlines that are active, not delivered, and not deleted,
// sorted by due_date ascending. A repository-side safety cap (maxNotifiableFetch) is applied
// to prevent unbounded memory usage; callers are responsible for truncating results after
// applying the notification-window filter and urgency sort.
func (dl *DeadlineMongoDBRepository) FindActiveNotifiable(ctx context.Context) ([]*Deadline, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.find_active_notifiable")
	defer span.End()

	reqID := ctxutil.HeaderIDFromContext(ctx)
	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
	)

	coll, err := dl.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	filter := bson.M{
		"active":       true,
		"delivered_at": nil,
		"deleted_at":   nil,
	}

	// maxNotifiableFetch is a repository-side safety cap independent of the
	// user-facing limit. It prevents unbounded memory spikes when the active
	// deadline collection is large. The caller applies the final limit after
	// notification-window filtering and urgency sorting.
	const maxNotifiableFetch = 1000

	opts := options.Find().
		SetSort(bson.D{{Key: "due_date", Value: 1}}).
		SetLimit(maxNotifiableFetch)

	ctx, spanExec := tracer.Start(ctx, "repository.deadline.find_active_notifiable_exec")

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to find active notifiable deadlines", err)
		spanExec.End()

		return nil, err
	}

	spanExec.End()

	var results []DeadlineMongoDBModel
	if err = cursor.All(ctx, &results); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode deadlines", err)
		return nil, err
	}

	deadlines := make([]*Deadline, 0, len(results))
	for i := range results {
		deadlines = append(deadlines, results[i].ToEntity())
	}

	return deadlines, nil
}
