// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"

	libMongo "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
)

// Repository provides an interface for operations related on mongo a metadata entities.
//
//go:generate mockgen --destination=template.mongodb.mock.go --package=template --copyright_file=../../../COPYRIGHT . Repository
type Repository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*Template, error)
	FindList(ctx context.Context, filters http.QueryHeader) ([]*Template, error)
	Count(ctx context.Context, filters http.QueryHeader) (int64, error)
	CountAll(ctx context.Context) (int64, error)
	Create(ctx context.Context, record *TemplateMongoDBModel) (*Template, error)
	Update(ctx context.Context, id uuid.UUID, updateFields *bson.M) error
	Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error
	FindOutputFormatByID(ctx context.Context, id uuid.UUID) (*string, error)
	FindOutputFormatByIDIncludeDeleted(ctx context.Context, id uuid.UUID) (*string, error)
	FindMappedFieldsAndOutputFormatByID(ctx context.Context, id uuid.UUID) (*string, map[string]map[string][]string, string, error)
}

// TemplateMongoDBRepository is a MongoDD-specific implementation of the PackageRepository.
type TemplateMongoDBRepository struct {
	connection *libMongo.MongoConnection
	Database   string
}

// Compile-time interface satisfaction check.
var _ Repository = (*TemplateMongoDBRepository)(nil)

// NewTemplateMongoDBRepository returns a new instance of TemplateMongoDBRepository using the given MongoDB connection.
func NewTemplateMongoDBRepository(mc *libMongo.MongoConnection) (*TemplateMongoDBRepository, error) {
	r := &TemplateMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}
	if _, err := r.connection.GetDB(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb for templates: %w", err)
	}

	return r, nil
}

// NewTemplateMongoDBRepositoryLazy returns a new repository without eagerly dialing MongoDB.
// This is used for multi-tenant manager mode, where tenant-scoped Mongo databases are resolved
// at request time and the static connection is only a fallback.
func NewTemplateMongoDBRepositoryLazy(mc *libMongo.MongoConnection) (*TemplateMongoDBRepository, error) {
	if mc == nil {
		return nil, fmt.Errorf("mongo connection is required")
	}

	return &TemplateMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}, nil
}

// getCollection returns the MongoDB collection for templates, using tenant-scoped connection when
// available (multi-tenant mode) or falling back to the static connection (single-tenant mode).
func (tm *TemplateMongoDBRepository) getCollection(ctx context.Context) (*mongo.Collection, error) {
	db, err := libMongo.ResolveDatabase(ctx, tm.connection, tm.Database)
	if err != nil {
		return nil, fmt.Errorf("resolving mongodb connection: %w", err)
	}

	return db.Collection(strings.ToLower(constant.MongoCollectionTemplate)), nil
}

// FindByID retrieves a template from the mongodb using the provided entity_id.
func (tm *TemplateMongoDBRepository) FindByID(ctx context.Context, id uuid.UUID) (*Template, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.find_by_id")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.template_id", id.String()),
	}

	span.SetAttributes(attributes...)

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	var record *TemplateMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "repository.template.find_by_id_exec")

	spanFindOne.SetAttributes(attributes...)

	filter := bson.M{"_id": id, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}

	if err = coll.
		FindOne(ctx, filter).
		Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(spanFindOne, "Failed to find template by entity", err)
		return nil, err
	}

	if nil == record {
		libOpentelemetry.HandleSpanError(span, "Template record is nil after decode", mongo.ErrNoDocuments)
		return nil, mongo.ErrNoDocuments
	}

	spanFindOne.End()

	return record.ToEntity(), nil
}

// FindList retrieves all templates from the mongodb using the provided filters.
func (tm *TemplateMongoDBRepository) FindList(ctx context.Context, filters http.QueryHeader) ([]*Template, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.find_list")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", filters, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert filters to JSON string", err)
	}

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	queryFilter := buildTemplateQueryFilter(filters)

	limit := int64(filters.Limit)
	skip := int64(filters.Page*filters.Limit - filters.Limit)
	opts := options.Find().SetLimit(limit).SetSkip(skip)

	ctx, spanFind := tracer.Start(ctx, "repository.template.find_list_exec")

	spanFind.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanFind, "app.request.repository_filter", filters, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to convert filters to JSON string", err)
	}

	cur, err := coll.Find(ctx, queryFilter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find templates", err)
		return nil, err
	}

	spanFind.End()

	var results []*TemplateMongoDBModel

	for cur.Next(ctx) {
		var record TemplateMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode template", err)
			return nil, err
		}

		results = append(results, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate templates", err)
		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to close cursor", err)
		return nil, err
	}

	templates := make([]*Template, 0, len(results))
	for i := range results {
		templates = append(templates, results[i].ToEntity())
	}

	return templates, nil
}

// Count returns the total number of non-deleted templates matching the given filters.
func (tm *TemplateMongoDBRepository) Count(ctx context.Context, filters http.QueryHeader) (int64, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.count")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return 0, err
	}

	queryFilter := buildTemplateQueryFilter(filters)

	ctx, spanCount := tracer.Start(ctx, "repository.template.count_exec")
	spanCount.SetAttributes(attribute.String("app.request.request_id", reqID))

	total, err := coll.CountDocuments(ctx, queryFilter)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanCount, "Failed to count templates", err)
		return 0, err
	}

	spanCount.End()

	return total, nil
}

// CountAll returns the total number of non-deleted templates.
func (tm *TemplateMongoDBRepository) CountAll(ctx context.Context) (int64, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.count_all")
	defer span.End()

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return 0, err
	}

	filter := bson.M{
		"deleted_at": bson.D{{Key: "$eq", Value: nil}},
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count all templates", err)
		return 0, err
	}

	return total, nil
}

func buildTemplateQueryFilter(filters http.QueryHeader) bson.M {
	queryFilter := bson.M{}

	if !commons.IsNilOrEmpty(&filters.OutputFormat) {
		queryFilter["output_format"] = filters.OutputFormat
	}

	if !filters.CreatedAt.IsZero() {
		end := filters.CreatedAt.Add(constant.HoursPerDay * time.Hour)
		queryFilter["created_at"] = bson.M{
			"$gte": filters.CreatedAt,
			"$lt":  end,
		}
	}

	if !commons.IsNilOrEmpty(&filters.Description) {
		queryFilter["description"] = bson.M{
			"$regex":   regexp.QuoteMeta(filters.Description),
			"$options": "i",
		}
	}

	queryFilter["deleted_at"] = bson.D{{Key: "$eq", Value: nil}}

	return queryFilter
}

// FindOutputFormatByID retrieves outputFormat of a template provided entity_id.
func (tm *TemplateMongoDBRepository) FindOutputFormatByID(ctx context.Context, id uuid.UUID) (*string, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.find_output_format_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.template_id", id.String()),
	)

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	var record struct {
		OutputFormat string `bson:"output_format"`
	}

	opts := options.FindOne().SetProjection(bson.M{
		"output_format": 1,
		"_id":           0,
	})

	filter := bson.M{"_id": id, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}

	if err = coll.
		FindOne(ctx, filter, opts).
		Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find template output_format by entity", err)
		return nil, err
	}

	return &record.OutputFormat, nil
}

// FindOutputFormatByIDIncludeDeleted retrieves outputFormat of a template including soft-deleted.
// Used by report download to resolve the output format even after the template is deleted.
func (tm *TemplateMongoDBRepository) FindOutputFormatByIDIncludeDeleted(ctx context.Context, id uuid.UUID) (*string, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.find_output_format_by_id_include_deleted")
	defer span.End()

	coll, err := tm.getCollection(ctx)
	if err != nil {
		return nil, err
	}

	var record struct {
		OutputFormat string `bson:"output_format"`
	}

	opts := options.FindOne().SetProjection(bson.M{"output_format": 1, "_id": 0})

	if err = coll.FindOne(ctx, bson.M{"_id": id}, opts).Decode(&record); err != nil {
		return nil, err
	}

	return &record.OutputFormat, nil
}

// Create inserts a new package entity into mongo.
func (tm *TemplateMongoDBRepository) Create(ctx context.Context, record *TemplateMongoDBModel) (*Template, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.create")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", record, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert template record to JSON string", err)
	}

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "repository.template.create_exec")

	spanInsert.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanInsert, "app.request.repository_input", record, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to convert template record to JSON string", err)
	}

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanInsert, "Failed to insert template", err)

		return nil, err
	}

	spanInsert.End()

	return record.ToEntity(), nil
}

// Update a template entity into mongodb.
func (tm *TemplateMongoDBRepository) Update(ctx context.Context, id uuid.UUID, updateFields *bson.M) error {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.update")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", updateFields, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert template record to JSON string", err)
	}

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	opts := options.UpdateOne().SetUpsert(false)

	ctx, spanUpdate := tracer.Start(ctx, "repository.template.update_exec")

	spanUpdate.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromValue(spanUpdate, "app.request.repository_input", updateFields, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to convert template record from entity to JSON string", err)
	}

	filter := bson.M{"_id": id}

	result, err := coll.UpdateOne(ctx, filter, updateFields, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update template", err)
		return err
	}

	if result.MatchedCount == 0 {
		spanUpdate.End()
		return mongo.ErrNoDocuments
	}

	spanUpdate.End()

	return nil
}

// Delete a template entity into mongodb with soft delete or not.
func (tm *TemplateMongoDBRepository) Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	logger := tm.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.delete")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.template_id", id.String()),
	}

	span.SetAttributes(attributes...)

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	opts := options.DeleteOne()

	ctx, spanDelete := tracer.Start(ctx, "repository.template.delete_exec")

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "deleted_at", Value: nil},
	}

	if hardDelete {
		deleted, err := coll.DeleteOne(ctx, filter, opts)
		if err != nil {
			libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete template", err)

			return err
		}

		spanDelete.End()

		if deleted.DeletedCount == 0 {
			return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.MongoCollectionTemplate)
		}
	} else {
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "deleted_at", Value: time.Now()},
			}},
		}

		updateResult, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			libOpentelemetry.HandleSpanError(spanDelete, "Failed to soft delete template", err)

			return err
		}

		if updateResult.MatchedCount == 0 {
			return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.MongoCollectionTemplate)
		}
	}

	spanDelete.End()

	logger.Log(ctx, log.LevelInfo, "Deleted a template", log.String("id", id.String()), log.Bool("hard_delete", hardDelete))

	return nil
}

// FindMappedFieldsAndOutputFormatByID find mapped fields of template and output format.
func (tm *TemplateMongoDBRepository) FindMappedFieldsAndOutputFormatByID(ctx context.Context, id uuid.UUID) (*string, map[string]map[string][]string, string, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.find_mapped_fields_and_output_format_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.template_id", id.String()),
	)

	coll, err := tm.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, nil, "", err
	}

	var record struct {
		OutputFormat string                         `bson:"output_format"`
		Description  string                         `bson:"description"`
		MappedFields map[string]map[string][]string `bson:"mapped_fields"`
	}

	opts := options.FindOne().SetProjection(bson.M{
		"output_format": 1,
		"description":   1,
		"mapped_fields": 1,
		"_id":           0,
	})

	filter := bson.M{"_id": id, "deleted_at": bson.D{{Key: "$eq", Value: nil}}}

	if err = coll.
		FindOne(ctx, filter, opts).
		Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find template output_format and mapped_fields by entity ID", err)
		return nil, nil, "", err
	}

	return &record.OutputFormat, record.MappedFields, record.Description, nil
}
