// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package extraction

import (
	"context"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// EnsureIndexes creates the required MongoDB indexes for the extraction_mapping
// collection. This method uses the static (admin) database connection for
// bootstrap-time index creation, not a per-tenant database.
func (r *ExtractionMappingMongoDBRepository) EnsureIndexes(ctx context.Context) error {
	logger := r.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.extraction_mapping.ensure_indexes")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.collection", constant.MongoCollectionExtractionMapping),
	)

	logger.Log(ctx, log.LevelDebug, "Creating indexes for collection",
		log.String("collection", constant.MongoCollectionExtractionMapping))

	coll, err := r.getCollection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get collection", err)
		return err
	}

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "job_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_extraction_job_id").
				SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "report_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_extraction_report_id"),
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_extraction_stale_pending"),
		},
	}

	ctx, cancel := context.WithTimeout(ctx, constant.MongoIndexCreateTimeout)
	defer cancel()

	logger.Log(ctx, log.LevelDebug, "Attempting to create indexes for collection",
		log.Int("index_count", len(indexes)),
		log.String("collection", constant.MongoCollectionExtractionMapping))

	indexNames, err := coll.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		if strings.Contains(err.Error(), "IndexOptionsConflict") ||
			strings.Contains(err.Error(), "already exists") {
			logger.Log(ctx, log.LevelDebug, "Indexes already exist (detected during creation)",
				log.String("collection", constant.MongoCollectionExtractionMapping))

			return nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to create indexes", err)
		logger.Log(ctx, log.LevelError, "Failed to create indexes",
			log.String("collection", constant.MongoCollectionExtractionMapping),
			log.Err(err))

		return err
	}

	logger.Log(ctx, log.LevelDebug, "Successfully created indexes for collection",
		log.Int("index_count", len(indexNames)),
		log.String("collection", constant.MongoCollectionExtractionMapping),
		log.Any("index_names", indexNames))

	return nil
}
