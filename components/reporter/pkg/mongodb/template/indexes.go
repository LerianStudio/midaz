// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template

import (
	"context"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	libMongo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// EnsureIndexes creates the required MongoDB indexes for the template collection.
// This method intentionally uses the static (admin) database connection via
// tm.connection.GetDB, NOT the per-tenant getCollection helper. Index creation
// is a bootstrap operation that must target the default database for all tenants,
// not a specific tenant's database. Do NOT change this to use getCollection.
func (tm *TemplateMongoDBRepository) EnsureIndexes(ctx context.Context) error {
	logger := tm.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.ensure_indexes")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.collection", constant.MongoCollectionTemplate),
	)
	logger.Log(ctx, log.LevelInfo, "Creating indexes for collection", log.String("collection", constant.MongoCollectionTemplate))

	db, err := tm.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	coll := db.Database(strings.ToLower(tm.Database)).Collection(strings.ToLower(constant.MongoCollectionTemplate))

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "_id", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_template_id_deleted"),
		},

		{
			Keys: bson.D{
				{Key: "deleted_at", Value: 1},
				{Key: "created_at", Value: -1},
			},
			Options: options.Index().
				SetName("idx_template_list_main").
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},

		{
			Keys: bson.D{
				{Key: "deleted_at", Value: 1},
				{Key: "output_format", Value: 1},
				{Key: "created_at", Value: -1},
			},
			Options: options.Index().
				SetName("idx_template_format").
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},

		{
			Keys: bson.D{
				{Key: "description", Value: "text"},
			},
			Options: options.Index().
				SetName("idx_template_description_text").
				SetWeights(bson.D{
					{Key: "description", Value: constant.MongoTextSearchWeight},
				}),
		},
	}

	ctx, cancel := context.WithTimeout(ctx, constant.MongoIndexCreateTimeout)
	defer cancel()

	logger.Log(ctx, log.LevelInfo, "Attempting to create indexes for collection", log.Int("index_count", len(indexes)), log.String("collection", constant.MongoCollectionTemplate))

	indexNames, err := coll.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		// Check if error is due to indexes already existing
		if libMongo.IsIndexAlreadyExistsError(err) {
			logger.Log(ctx, log.LevelInfo, "Indexes already exist (detected during creation)", log.String("collection", constant.MongoCollectionTemplate))
			return nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to create indexes", err)
		logger.Log(ctx, log.LevelError, "Failed to create indexes", log.String("collection", constant.MongoCollectionTemplate), log.Err(err))

		return err
	}

	logger.Log(ctx, log.LevelInfo, "Successfully created indexes for collection", log.Int("index_count", len(indexNames)), log.String("collection", constant.MongoCollectionTemplate), log.Any("index_names", indexNames))

	return nil
}

// DropIndexes removes all custom indexes for the templates collection.
func (tm *TemplateMongoDBRepository) DropIndexes(ctx context.Context) error {
	logger := tm.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.template.drop_indexes")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.collection", constant.MongoCollectionTemplate),
	)
	logger.Log(ctx, log.LevelWarn, "Dropping all custom indexes for collection", log.String("collection", constant.MongoCollectionTemplate))

	db, err := tm.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	coll := db.Database(strings.ToLower(tm.Database)).Collection(strings.ToLower(constant.MongoCollectionTemplate))

	ctx, cancel := context.WithTimeout(ctx, constant.MongoIndexDropTimeout)
	defer cancel()

	if err := coll.Indexes().DropAll(ctx); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to drop indexes", err)
		logger.Log(ctx, log.LevelError, "Failed to drop indexes", log.String("collection", constant.MongoCollectionTemplate), log.Err(err))

		return err
	}

	logger.Log(ctx, log.LevelInfo, "Successfully dropped all custom indexes for collection", log.String("collection", constant.MongoCollectionTemplate))

	return nil
}
