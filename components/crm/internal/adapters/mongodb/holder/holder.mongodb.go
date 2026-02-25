// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v3/commons/crypto"
	libMongo "github.com/LerianStudio/lib-commons/v3/commons/mongo"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	mongoUtils "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Repository provides an interface for operations related to holder entities.
//
//go:generate mockgen --destination=holder.mongodb_mock.go --package=holder . Repository
type Repository interface {
	Create(ctx context.Context, collection string, input *mmodel.Holder) (*mmodel.Holder, error)
	Find(ctx context.Context, collection string, id uuid.UUID, includeDeleted bool) (*mmodel.Holder, error)
	FindAll(ctx context.Context, collection string, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Holder, error)
	Update(ctx context.Context, collection string, id uuid.UUID, input *mmodel.Holder, fieldsToRemove []string) (*mmodel.Holder, error)
	Delete(ctx context.Context, collection string, id uuid.UUID, hardDelete bool) error
}

// MongoDBRepository is a MongoDB-specific implementation of Repository
type MongoDBRepository struct {
	connection   *libMongo.MongoConnection
	Database     string
	DataSecurity *libCrypto.Crypto
}

// NewMongoDBRepository returns a new instance of MongoDBRepository using the given MongoDB connection
func NewMongoDBRepository(connection *libMongo.MongoConnection, dataSecurity *libCrypto.Crypto) (*MongoDBRepository, error) {
	r := &MongoDBRepository{
		connection:   connection,
		Database:     connection.Database,
		DataSecurity: dataSecurity,
	}

	if _, err := r.connection.GetDB(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB for holder repository: %w", err)
	}

	return r, nil
}

// Create inserts a holder into mongo.
func (hm *MongoDBRepository) Create(ctx context.Context, organizationID string, holder *mmodel.Holder) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_holder")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	}

	span.SetAttributes(attributes...)

	db, err := hm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(hm.Database)).Collection(strings.ToLower("holders_" + organizationID))

	err = createIndexes(coll)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create indexes", err)

		return nil, err
	}

	record := &MongoDBModel{}

	if err := record.FromEntity(holder, hm.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert holder to model", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_holder.insert")

	spanInsert.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&spanInsert, "app.request.repository_input", record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanInsert, "Failed to convert record to JSON string", err)
	}

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanInsert, "Failed to insert holder", err)

		if mongo.IsDuplicateKeyError(err) {
			if strings.Contains(err.Error(), "document") {
				return nil, pkg.ValidateBusinessError(cn.ErrDocumentAssociationError, reflect.TypeOf(mmodel.Holder{}).Name())
			}
		}

		return nil, err
	}

	spanInsert.End()

	result, err := record.ToEntity(hm.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert holder to model", err)

		return nil, err
	}

	return result, nil
}

// Find fetches a holder by its id
func (hm *MongoDBRepository) Find(ctx context.Context, organizationID string, id uuid.UUID, includeDeleted bool) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_holder")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	}

	span.SetAttributes(attributes...)

	db, err := hm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(hm.Database)).Collection(strings.ToLower("holders_" + organizationID))

	var record MongoDBModel

	filter := bson.D{
		{Key: "_id", Value: id},
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_holder.find")

	spanFind.SetAttributes(attributes...)

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to find holder", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())
		}

		return nil, err
	}

	spanFind.End()

	result, err := record.ToEntity(hm.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert holder to model", err)

		return nil, err
	}

	return result, nil
}

// FindAll get all holders that match the query filter
func (hm *MongoDBRepository) FindAll(ctx context.Context, organizationID string, query http.QueryHeader, includeDeleted bool) ([]*mmodel.Holder, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_holders")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	}

	span.SetAttributes(attributes...)

	db, err := hm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(hm.Database)).Collection(strings.ToLower("holders_" + organizationID))

	limit := int64(query.Limit)
	skip := int64(query.Page*query.Limit - query.Limit)
	opts := options.FindOptions{Limit: &limit, Skip: &skip}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_all_holders.find")

	spanFind.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&spanFind, "app.request.repository_filter", query)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to convert query to JSON string", err)
	}

	filter, err := hm.buildHolderFilter(query, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Invalid metadata value", err)
		return nil, err
	}

	cursor, err := coll.Find(ctx, filter, &opts)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to find holder", err)

		return nil, err
	}

	spanFind.End()

	var holders []*MongoDBModel

	for cursor.Next(ctx) {
		var holder MongoDBModel
		if err := cursor.Decode(&holder); err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to decode holder", err)

			return nil, err
		}

		holders = append(holders, &holder)
	}

	if err := cursor.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to iterate holders", err)

		return nil, err
	}

	if err := cursor.Close(ctx); err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to close cursor", err)

		return nil, err
	}

	results := make([]*mmodel.Holder, len(holders))
	for i, holder := range holders {
		results[i], err = holder.ToEntity(hm.DataSecurity)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to convert holder to model", err)

			return nil, err
		}
	}

	return results, nil
}

func (hm *MongoDBRepository) buildHolderFilter(query http.QueryHeader, includeDeleted bool) (bson.D, error) {
	filter := bson.D{}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	if query.ExternalID != nil && *query.ExternalID != "" {
		filter = append(filter, bson.E{Key: "external_id", Value: *query.ExternalID})
	}

	if query.Document != nil && *query.Document != "" {
		documentHash := hm.DataSecurity.GenerateHash(query.Document)
		filter = append(filter, bson.E{Key: "search.document", Value: documentHash})
	}

	if query.Metadata != nil {
		for k, v := range *query.Metadata {
			safeValue, err := http.ValidateMetadataValue(v)
			if err != nil {
				return nil, err
			}

			filter = append(filter, bson.E{Key: k, Value: safeValue})
		}
	}

	return filter, nil
}

// Update a holder by id
func (hm *MongoDBRepository) Update(ctx context.Context, organizationID string, id uuid.UUID, holder *mmodel.Holder, fieldsToRemove []string) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_holder")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
		attribute.StringSlice("app.request.fields_to_remove", fieldsToRemove),
	}

	span.SetAttributes(attributes...)

	db, err := hm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(hm.Database)).Collection(strings.ToLower("holders_" + organizationID))

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_holder.update_by_id")

	spanUpdate.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&spanUpdate, "app.request.repository_input", holder)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to convert holder to JSON string", err)
	}

	holderToUpdate := &MongoDBModel{}

	if err := holderToUpdate.FromEntity(holder, hm.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert holder to model", err)

		return nil, err
	}

	bsonData, err := bson.Marshal(holderToUpdate)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to marshal holder", err)

		return nil, err
	}

	var updateDocument bson.M
	if err := bson.Unmarshal(bsonData, &updateDocument); err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to unmarshal holder", err)

		return nil, err
	}

	update := mongoUtils.BuildDocumentToPatch(updateDocument, fieldsToRemove)

	updateResult, err := coll.UpdateByID(ctx, id, update)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to update holder", err)

		return nil, err
	}

	if updateResult.MatchedCount == 0 {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Holder not found", cn.ErrHolderNotFound)

		return nil, pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())
	}

	spanUpdate.End()

	var record MongoDBModel

	ctx, spanFind := tracer.Start(ctx, "mongodb.update_holder.find_by_id")

	spanFind.SetAttributes(attributes...)

	err = coll.FindOne(ctx, bson.M{"_id": id}).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to find holder after update", err)

		return nil, err
	}

	spanFind.End()

	result, err := record.ToEntity(hm.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert holder to model", err)

		return nil, err
	}

	return result, nil
}

// Delete a holder from mongodb by its ID
func (hm *MongoDBRepository) Delete(ctx context.Context, organizationID string, id uuid.UUID, hardDelete bool) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_holder")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	}

	span.SetAttributes(attributes...)

	db, err := hm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return err
	}

	opts := options.Delete()

	coll := db.Database(strings.ToLower(hm.Database)).Collection(strings.ToLower("holders_" + organizationID))

	ctx, spanDelete := tracer.Start(ctx, "mongodb.delete_holder.delete_one")
	defer spanDelete.End()

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "deleted_at", Value: nil},
	}

	if hardDelete {
		deleted, err := coll.DeleteOne(ctx, filter, opts)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&spanDelete, "Failed to delete holder", err)

			return err
		}

		spanDelete.End()

		if deleted.DeletedCount == 0 {
			return pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())
		}
	} else {
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "deleted_at", Value: time.Now()},
			}},
		}

		updateResult, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&spanDelete, "Failed to delete holder", err)

			return err
		}

		if updateResult.MatchedCount == 0 {
			return pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())
		}
	}

	logger.Infoln("Deleted a document with id: ", id.String())

	return nil
}

// createIndexes creates indexes for specific fields, if it not exists
func createIndexes(collection *mongo.Collection) error {
	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "search.document", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{
				{Key: "external_id", Value: 1},
			},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{
				{Key: "search.document", Value: 1},
				{Key: "external_id", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateMany(ctx, indexModels)

	return err
}
