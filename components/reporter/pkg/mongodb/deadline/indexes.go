// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package deadline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// EnsureIndexes creates the required MongoDB indexes for the deadline collection.
// This method intentionally uses the static (admin) database connection via
// dr.connection.GetDB, NOT the per-tenant getCollection helper. Index creation
// is a bootstrap operation that must target the default database for all tenants,
// not a specific tenant's database. Do NOT change this to use getCollection.
func (dr *DeadlineMongoDBRepository) EnsureIndexes(ctx context.Context) error {
	logger := dr.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.deadline.ensure_indexes")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.collection", constant.MongoCollectionDeadline),
	)
	logger.Log(ctx, log.LevelInfo, "Creating indexes for collection", log.String("collection", constant.MongoCollectionDeadline))

	db, err := dr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	coll := db.Database(strings.ToLower(dr.Database)).Collection(strings.ToLower(constant.MongoCollectionDeadline))

	dropCtx, dropCancel := context.WithTimeout(ctx, constant.MongoIndexDropTimeout)
	defer dropCancel()

	// Drop the old name-based unique index that was replaced by the template-scoped one.
	// MongoDB does not raise IndexOptionsConflict for indexes with different key sets, so
	// without this explicit drop both constraints would coexist and the old one would keep
	// blocking writes that satisfy the new constraint but violate the removed name uniqueness.
	if errDrop := coll.Indexes().DropOne(dropCtx, "idx_deadline_unique_name_type_duedate_freq"); errDrop != nil {
		if !isIgnorableDropIndexError(errDrop) {
			logger.Log(ctx, log.LevelWarn, "Could not drop legacy deadline unique index (non-fatal)",
				log.String("index", "idx_deadline_unique_name_type_duedate_freq"),
				log.Err(errDrop))
		}
	} else {
		logger.Log(ctx, log.LevelInfo, "Dropped legacy deadline unique index",
			log.String("index", "idx_deadline_unique_name_type_duedate_freq"))
	}

	// Drop the legacy template-scoped unique index if it was previously created
	// with a $type-based partial filter (incompatible with AWS DocumentDB). The
	// new shape uses $exists and will be (re)created by CreateMany below.
	if err := dropLegacyTemplateScopedUniqueIndex(dropCtx, coll, logger); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to migrate legacy template-scoped unique index", err)
		return err
	}

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "_id", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_deadline_id_deleted"),
		},

		{
			Keys: bson.D{
				{Key: "deleted_at", Value: 1},
				{Key: "due_date", Value: 1},
			},
			Options: options.Index().
				SetName("idx_deadline_list_main").
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},

		{
			Keys: bson.D{
				{Key: "deleted_at", Value: 1},
				{Key: "active", Value: 1},
				{Key: "due_date", Value: 1},
			},
			Options: options.Index().
				SetName("idx_deadline_active_due_date").
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},

		{
			Keys: bson.D{
				{Key: "deleted_at", Value: 1},
				{Key: "type", Value: 1},
				{Key: "due_date", Value: 1},
			},
			Options: options.Index().
				SetName("idx_deadline_type_due_date").
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},

		{
			Keys: bson.D{
				{Key: "type", Value: 1},
				{Key: "template_id", Value: 1},
				{Key: "due_date", Value: 1},
				{Key: "frequency", Value: 1},
			},
			Options: options.Index().
				SetName("idx_deadline_unique_type_templateid_duedate_freq").
				SetUnique(true).
				// $exists is used instead of $type: "binData" because AWS DocumentDB
				// rejects $type inside partialFilterExpression while accepting $exists.
				// Semantically equivalent here: template_id is only ever set to a UUID
				// (binary) value, so $exists:true and $type:binData index the same set.
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
					{Key: "template_id", Value: bson.D{{Key: "$exists", Value: true}}},
				}),
		},
	}

	ctx, cancel := context.WithTimeout(ctx, constant.MongoIndexCreateTimeout)
	defer cancel()

	logger.Log(ctx, log.LevelInfo, "Attempting to create indexes for collection", log.Int("index_count", len(indexes)), log.String("collection", constant.MongoCollectionDeadline))

	indexNames, err := coll.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			logger.Log(ctx, log.LevelInfo, "Indexes already exist (detected during creation)", log.String("collection", constant.MongoCollectionDeadline))
			return nil
		}

		if isIndexOptionsConflict(err) {
			logger.Log(ctx, log.LevelInfo, "Index definition drift detected, dropping conflicting indexes and recreating", log.String("collection", constant.MongoCollectionDeadline))

			if errRecreate := recreateIndexes(ctx, coll, indexes, logger); errRecreate != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to recreate indexes after conflict", errRecreate)
				return errRecreate
			}

			return nil
		}

		// Existing data has duplicates that prevent the new unique index from being built.
		// Soft-delete the older duplicates so the constraint can be enforced going forward.
		if strings.Contains(err.Error(), "DuplicateKey") || strings.Contains(err.Error(), "E11000") {
			logger.Log(ctx, log.LevelWarn, "Duplicate data detected while creating unique index — deduplicating before retry", log.String("collection", constant.MongoCollectionDeadline))

			if errDedup := deduplicateDeadlines(ctx, coll, logger); errDedup != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to deduplicate deadlines", errDedup)
				return errDedup
			}

			if _, errRetry := coll.Indexes().CreateMany(ctx, indexes); errRetry != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to create indexes after deduplication", errRetry)
				logger.Log(ctx, log.LevelError, "Failed to create indexes after deduplication", log.String("collection", constant.MongoCollectionDeadline), log.Err(errRetry))

				return errRetry
			}

			logger.Log(ctx, log.LevelInfo, "Indexes created after deduplication", log.String("collection", constant.MongoCollectionDeadline))

			return nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to create indexes", err)
		logger.Log(ctx, log.LevelError, "Failed to create indexes", log.String("collection", constant.MongoCollectionDeadline), log.Err(err))

		return err
	}

	logger.Log(ctx, log.LevelInfo, "Successfully created indexes for collection", log.Int("index_count", len(indexNames)), log.String("collection", constant.MongoCollectionDeadline), log.Any("index_names", indexNames))

	return nil
}

// recreateIndexes attempts to create each index individually, dropping and recreating
// only the ones that conflict. Non-conflicting indexes (including unique constraints)
// remain in place throughout, avoiding windows where concurrent writes could bypass them.
func recreateIndexes(ctx context.Context, coll *mongo.Collection, indexes []mongo.IndexModel, logger log.Logger) error {
	for _, idx := range indexes {
		name := indexName(idx)
		if name == nil {
			continue
		}

		if _, err := coll.Indexes().CreateOne(ctx, idx); err == nil {
			continue
		} else if !isIndexOptionsConflict(err) &&
			!strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create index %q: %w", *name, err)
		}

		// Only this specific index conflicted — drop and recreate it
		logger.Log(ctx, log.LevelInfo, "Dropping conflicting index for recreation", log.String("index", *name))

		dropCtx, dropCancel := context.WithTimeout(ctx, constant.MongoIndexDropTimeout)

		if err := coll.Indexes().DropOne(dropCtx, *name); err != nil {
			if !isIgnorableDropIndexError(err) {
				dropCancel()
				return fmt.Errorf("failed to drop index %q: %w", *name, err)
			}
		}

		dropCancel()

		if _, err := coll.Indexes().CreateOne(ctx, idx); err != nil {
			return fmt.Errorf("failed to recreate index %q: %w", *name, err)
		}

		logger.Log(ctx, log.LevelInfo, "Successfully recreated index", log.String("index", *name))
	}

	return nil
}

// deduplicateDeadlines soft-deletes older duplicates that share the same
// (type, template_id, due_date, frequency) tuple, keeping exactly one survivor
// per group determined by a deterministic sort (_id descending, which for UUID v7
// equals most-recently-created). Only operates on non-deleted documents that have
// a template_id set (matching the partial filter of the unique index).
//
// The survivor is selected by ID — not by created_at — so that ties on timestamp
// never leave any duplicates un-removed and never block the index-creation retry.
func deduplicateDeadlines(ctx context.Context, coll *mongo.Collection, logger log.Logger) error {
	// Sort before grouping so $first picks the highest _id (most recent for UUID v7).
	// Match the SAME document set that the unique partial index covers
	// (idx_deadline_unique_type_templateid_duedate_freq → template_id: {$exists: true})
	// so a DuplicateKey/E11000 cannot leave non-binData or null template_id
	// duplicates behind, which would re-trigger the CreateMany retry failure.
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "deleted_at", Value: nil},
			{Key: "template_id", Value: bson.D{{Key: "$exists", Value: true}}},
		}}},
		{{Key: "$sort", Value: bson.D{
			{Key: "_id", Value: -1},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "type", Value: "$type"},
				{Key: "template_id", Value: "$template_id"},
				{Key: "due_date", Value: "$due_date"},
				{Key: "frequency", Value: "$frequency"},
			}},
			{Key: "allIDs", Value: bson.D{{Key: "$push", Value: "$_id"}}},
			{Key: "keepID", Value: bson.D{{Key: "$first", Value: "$_id"}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$match", Value: bson.D{
			{Key: "count", Value: bson.D{{Key: "$gt", Value: 1}}},
		}}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return fmt.Errorf("failed to aggregate duplicates: %w", err)
	}

	defer cursor.Close(ctx)

	now := time.Now()

	var totalRemoved int64

	for cursor.Next(ctx) {
		var group struct {
			AllIDs []any `bson:"allIDs"`
			KeepID any   `bson:"keepID"`
		}

		if err := cursor.Decode(&group); err != nil {
			return fmt.Errorf("failed to decode duplicate group during deduplication: %w", err)
		}

		// Soft-delete every ID in the group EXCEPT the single survivor.
		// Using $nin with [keepID] ensures exactly one document survives even when
		// multiple duplicates share the same created_at timestamp.
		result, errUpdate := coll.UpdateMany(ctx,
			bson.D{
				{Key: "_id", Value: bson.D{
					{Key: "$in", Value: group.AllIDs},
					{Key: "$nin", Value: []any{group.KeepID}},
				}},
				{Key: "deleted_at", Value: nil},
			},
			bson.D{{Key: "$set", Value: bson.D{{Key: "deleted_at", Value: now}}}},
		)
		if errUpdate != nil {
			return fmt.Errorf("failed to soft-delete duplicates: %w", errUpdate)
		}

		totalRemoved += result.ModifiedCount
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("cursor error while deduplicating: %w", err)
	}

	logger.Log(ctx, log.LevelInfo, "Deadline deduplication complete",
		log.Any("soft_deleted_count", totalRemoved))

	return nil
}

// dropLegacyTemplateScopedUniqueIndex inspects the existing
// idx_deadline_unique_type_templateid_duedate_freq index and drops it when it
// still carries the legacy $type-based partialFilterExpression. The new
// $exists-based shape is then (re)created by the regular CreateMany path.
//
// Idempotent and safe to run on every bootstrap:
//   - If the index does not exist (fresh DB), nothing happens.
//   - If it exists in the current shape ($exists), nothing happens.
//   - Only the legacy shape ($type) is dropped.
func dropLegacyTemplateScopedUniqueIndex(ctx context.Context, coll *mongo.Collection, logger log.Logger) error {
	const indexName = "idx_deadline_unique_type_templateid_duedate_freq"

	cur, err := coll.Indexes().List(ctx)
	if err != nil {
		if isIgnorableDropIndexError(err) {
			return nil
		}

		return fmt.Errorf("list deadline indexes: %w", err)
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var spec bson.M
		if err := cur.Decode(&spec); err != nil {
			return fmt.Errorf("decode index spec: %w", err)
		}

		name, _ := spec["name"].(string)
		if name != indexName {
			continue
		}

		if !isLegacyTemplateScopedFilter(spec) {
			return nil
		}

		if dropErr := coll.Indexes().DropOne(ctx, indexName); dropErr != nil {
			if isIgnorableDropIndexError(dropErr) {
				return nil
			}

			return fmt.Errorf("drop legacy %q: %w", indexName, dropErr)
		}

		logger.Log(ctx, log.LevelInfo,
			"Dropped legacy template-scoped unique index for DocumentDB-compatible recreation",
			log.String("index", indexName))

		return nil
	}

	return cur.Err()
}

// isLegacyTemplateScopedFilter reports whether the given decoded index spec
// uses the legacy partialFilterExpression shape {template_id: {$type: ...}}.
// Anything else (including the new {template_id: {$exists: true}} shape or a
// missing partial filter) is treated as non-legacy and left untouched.
//
// Nested documents must be read representation-agnostically: the mongo-driver
// decoder default-decodes nested documents to bson.D (ordered slice), not
// bson.M. Asserting bson.M alone would silently miss the legacy shape and skip
// the required index cleanup, so both forms are handled.
func isLegacyTemplateScopedFilter(spec bson.M) bool {
	pfe, ok := asNestedDoc(spec["partialFilterExpression"])
	if !ok {
		return false
	}

	templateClause, ok := asNestedDoc(pfe["template_id"])
	if !ok {
		return false
	}

	_, hasType := templateClause["$type"]

	return hasType
}

// indexName extracts the configured index name from a mongo.IndexModel.
// IndexModel.Options is an *options.IndexOptionsBuilder whose values are only
// reachable by applying its option funcs to a fresh IndexOptions, so the name
// set via SetName is recovered here rather than read off a struct field.
func indexName(idx mongo.IndexModel) *string {
	if idx.Options == nil {
		return nil
	}

	opts := options.IndexOptions{}

	for _, set := range idx.Options.List() {
		if set == nil {
			continue
		}

		if err := set(&opts); err != nil {
			return nil
		}
	}

	return opts.Name
}

// asNestedDoc normalizes a decoded nested document value into a bson.M,
// accepting either a bson.M (mongo-driver v1 / explicit map) or a bson.D
// (mongo-driver v2 default for nested documents). It reports false for any
// other type, including a nil value or a non-document.
func asNestedDoc(v any) (bson.M, bool) {
	switch doc := v.(type) {
	case bson.M:
		return doc, true
	case bson.D:
		m := make(bson.M, len(doc))
		for _, e := range doc {
			m[e.Key] = e.Value
		}

		return m, true
	default:
		return nil, false
	}
}

// isIgnorableDropIndexError reports whether err can be safely treated as
// "already gone" during index cleanup. Two server errors are ignored:
//
//   - NamespaceNotFound (code 26): the collection does not exist yet
//     (fresh DB on first boot).
//   - IndexNotFound (code 27): the index has already been dropped or
//     was never present.
//
// Prefer the typed mongo.CommandError discriminator; the string fallback
// covers driver wrappers or wire-protocol formats that do not surface the
// command error structure.
func isIgnorableDropIndexError(err error) bool {
	if err == nil {
		return false
	}

	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == 26 || cmdErr.Code == 27
	}

	msg := err.Error()

	return strings.Contains(msg, "IndexNotFound") ||
		strings.Contains(msg, "index not found") ||
		strings.Contains(msg, "NamespaceNotFound") ||
		strings.Contains(msg, "ns not found") ||
		strings.Contains(msg, "ns does not exist")
}

// isIndexOptionsConflict reports whether err is a Mongo IndexOptionsConflict
// (code 85) or IndexKeySpecsConflict (code 86), which the bootstrap treats
// as drift requiring an explicit drop + recreate.
func isIndexOptionsConflict(err error) bool {
	if err == nil {
		return false
	}

	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == 85 || cmdErr.Code == 86
	}

	msg := err.Error()

	return strings.Contains(msg, "IndexOptionsConflict") ||
		strings.Contains(msg, "IndexKeySpecsConflict")
}
