// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// tempEntityIDIndexName serves entity_id lookups while entity_id_1 is rebuilt unique.
const tempEntityIDIndexName = "entity_id_1__id_1_tmp"

// restoreIndexTimeout bounds rebuilding the non-unique entity_id_1 after a failed unique build.
const restoreIndexTimeout = 30 * time.Second

// indexNotFoundCode answers a drop of a missing index on DocumentDB and older MongoDB; 8.x answers ok.
const indexNotFoundCode = 27

var errMetadataRaced = errors.New("metadata written during the dedupe")

// DedupeEntityIDAndMakeUnique folds each entity's metadata documents in coll into the oldest one,
// then makes entity_id_1 unique. raced reports a document written during the run, by a fold or by
// the unique build: coll keeps the index it had and waits for a re-run.
func DedupeEntityIDAndMakeUnique(ctx context.Context, coll *mongo.Collection) (folded int, raced bool, err error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.dedupe_entity_id")
	defer span.End()

	folded, raced, err = foldDuplicates(ctx, coll)
	if err == nil && !raced {
		err = makeEntityIDIndexUnique(ctx, coll)
		raced = mongo.IsDuplicateKeyError(err)
	}

	switch {
	case raced:
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Metadata written during the dedupe", errors.Join(err, errMetadataRaced))

		return folded, true, nil
	case err != nil:
		libOpentelemetry.HandleSpanError(span, "Failed to dedupe metadata", err)
	}

	return folded, false, err
}

func foldDuplicates(ctx context.Context, coll *mongo.Collection) (folded int, raced bool, err error) {
	cur, err := coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$entity_id"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		{{Key: "$match", Value: bson.D{{Key: "count", Value: bson.D{{Key: "$gt", Value: 1}}}}}},
	}, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return 0, false, err
	}

	var duplicated []struct {
		EntityID string `bson:"_id"`
	}

	if err := cur.All(ctx, &duplicated); err != nil {
		return 0, false, err
	}

	for _, group := range duplicated {
		docs, err := readEntity(ctx, coll, group.EntityID)
		if err != nil {
			return folded, raced, err
		}

		if len(docs) < 2 {
			continue
		}

		written, err := foldEntity(ctx, coll, docs)
		if err != nil {
			return folded, raced, err
		}

		if written {
			folded++
		}

		raced = raced || !written
	}

	return folded, raced, nil
}

func readEntity(ctx context.Context, coll *mongo.Collection, entityID string) ([]MetadataMongoDBModel, error) {
	cur, err := coll.Find(ctx, bson.D{{Key: "entity_id", Value: entityID}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}

	var docs []MetadataMongoDBModel

	err = cur.All(ctx, &docs)

	return docs, err
}

// foldEntity writes the fold of docs over the oldest, then deletes the others, every write guarded
// on the _id and updated_at it read: a later write is never overwritten nor deleted, and reports
// false for a re-run. The fold lands before any delete, so an interrupted pass loses nothing.
func foldEntity(ctx context.Context, coll *mongo.Collection, docs []MetadataMongoDBModel) (bool, error) {
	kept := mergeMetadataDocuments(coll.Name(), docs)

	set := bson.D{{Key: "metadata", Value: kept.Data}, {Key: "updated_at", Value: kept.UpdatedAt}}
	if !kept.CreatedAt.IsZero() {
		set = append(set, bson.E{Key: "created_at", Value: kept.CreatedAt})
	}

	if kept.EntityName != "" {
		set = append(set, bson.E{Key: "entity_name", Value: kept.EntityName})
	}

	written, err := coll.UpdateOne(ctx, unchanged(docs[0]), bson.D{{Key: "$set", Value: set}})
	if err != nil || written.MatchedCount == 0 {
		return false, err
	}

	others := make(bson.A, 0, len(docs)-1)
	for _, doc := range docs[1:] {
		others = append(others, unchanged(doc))
	}

	deleted, err := coll.DeleteMany(ctx, bson.D{{Key: "$or", Value: others}})
	if err != nil {
		return false, err
	}

	return deleted.DeletedCount == int64(len(others)), nil
}

func unchanged(doc MetadataMongoDBModel) bson.D {
	return bson.D{{Key: "_id", Value: doc.ID}, {Key: "updated_at", Value: doc.UpdatedAt}}
}

// mergeMetadataDocuments folds one entity's documents, oldest _id first, into that oldest one:
// every key survives and the latest update wins it, but on transactions and operations a reserved
// key from a completed (named) document wins. It keeps the name, earliest creation, latest update.
func mergeMetadataDocuments(collection string, docs []MetadataMongoDBModel) MetadataMongoDBModel {
	byUpdate := slices.Clone(docs)
	slices.SortStableFunc(byUpdate, func(a, b MetadataMongoDBModel) int { return a.UpdatedAt.Compare(b.UpdatedAt) })

	kept := docs[0]
	kept.Data = JSON{}

	for _, doc := range byUpdate {
		maps.Copy(kept.Data, doc.Data)

		if kept.EntityName == "" {
			kept.EntityName = doc.EntityName
		}

		if !doc.CreatedAt.IsZero() && (kept.CreatedAt.IsZero() || doc.CreatedAt.Before(kept.CreatedAt)) {
			kept.CreatedAt = doc.CreatedAt
		}

		if doc.UpdatedAt.After(kept.UpdatedAt) {
			kept.UpdatedAt = doc.UpdatedAt
		}
	}

	if collection != strings.ToLower(constant.EntityTransaction) && collection != strings.ToLower(constant.EntityOperation) {
		return kept
	}

	for _, doc := range byUpdate {
		if doc.EntityName == "" {
			continue
		}

		for key, value := range doc.Data {
			if constant.IsReservedMetadataKey(key) {
				kept.Data[key] = value
			}
		}
	}

	return kept
}

// makeEntityIDIndexUnique leaves coll with a unique entity_id_1 and no temporary index.
func makeEntityIDIndexUnique(ctx context.Context, coll *mongo.Collection) error {
	unique, err := EnsureUniqueEntityIDIndex(ctx, coll)
	if err == nil && !unique {
		err = rebuildUnique(ctx, coll)
	}

	if err != nil {
		return err
	}

	err = coll.Indexes().DropOne(ctx, tempEntityIDIndexName)
	if se := mongo.ServerError(nil); errors.As(err, &se) && se.HasErrorCode(indexNotFoundCode) {
		return nil
	}

	return err
}

// rebuildUnique swaps the non-unique entity_id_1 for a unique one (DocumentDB and MongoDB < 6.0
// cannot convert in place) while the temporary index serves lookups. A failed build restores the
// non-unique index even when ctx is done, keeping the temporary one if that restore fails.
func rebuildUnique(ctx context.Context, coll *mongo.Collection) error {
	indexes := coll.Indexes()
	keys := bson.D{{Key: "entity_id", Value: 1}}

	if _, err := indexes.CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName(tempEntityIDIndexName),
	}); err != nil {
		return err
	}

	if err := indexes.DropOne(ctx, entityIDIndexName); err != nil {
		return err
	}

	_, err := indexes.CreateOne(ctx, mongo.IndexModel{Keys: keys, Options: options.Index().SetUnique(true)})
	if err == nil {
		return nil
	}

	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreIndexTimeout)
	defer cancel()

	if _, restoreErr := indexes.CreateOne(restoreCtx, mongo.IndexModel{Keys: keys}); restoreErr != nil {
		return fmt.Errorf("restore %s after %v: %w", entityIDIndexName, err, restoreErr)
	}

	return errors.Join(err, indexes.DropOne(restoreCtx, tempEntityIDIndexName))
}
