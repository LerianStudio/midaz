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

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v4/commons/crypto"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
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
	connection   *libMongo.Client
	DataSecurity *libCrypto.Crypto
}

// NewMongoDBRepository returns a new instance of MongoDBRepository using the given MongoDB connection.
// In multi-tenant mode, connection may be nil — the per-request tenant context provides the database.
func NewMongoDBRepository(connection *libMongo.Client, dataSecurity *libCrypto.Crypto) (*MongoDBRepository, error) {
	r := &MongoDBRepository{
		DataSecurity: dataSecurity,
	}

	if connection != nil {
		r.connection = connection

		if _, err := r.connection.Database(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to connect to MongoDB for holder repository: %w", err)
		}
	}

	return r, nil
}

// getDatabase resolves the MongoDB database for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific *mongo.Database into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (hm *MongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
	if hm.connection == nil {
		// Check tenant context when static connection is nil (multi-tenant mode without static fallback)
		if db := tmcore.GetMongoFromContext(ctx); db != nil {
			return db, nil
		}

		return nil, fmt.Errorf("no database connection available: multi-tenant context required but not present, and no static connection configured")
	}

	if db := tmcore.GetMongoFromContext(ctx); db != nil {
		return db, nil
	}

	return hm.connection.Database(ctx)
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

	db, err := hm.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("holders_" + organizationID))

	err = createIndexes(ctx, coll)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create indexes", err)

		return nil, err
	}

	record := &MongoDBModel{}

	if err := record.FromEntity(holder, hm.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_holder.insert")

	spanInsert.SetAttributes(attributes...)

	spanInsert.SetAttributes(
		attribute.Bool("app.request.repository_input.has_metadata", len(record.Metadata) > 0),
		attribute.Bool("app.request.repository_input.has_external_id", record.ExternalID != nil),
		attribute.Bool("app.request.repository_input.has_contact", record.Contact != nil),
		attribute.Bool("app.request.repository_input.has_addresses", record.Addresses != nil),
		attribute.Bool("app.request.repository_input.has_natural_person", record.NaturalPerson != nil),
		attribute.Bool("app.request.repository_input.has_legal_person", record.LegalPerson != nil),
	)

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanInsert, "Failed to insert holder", err)

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
		libOpenTelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

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

	db, err := hm.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("holders_" + organizationID))

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
		libOpenTelemetry.HandleSpanError(span, "Failed to find holder", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())
		}

		return nil, err
	}

	spanFind.End()

	result, err := record.ToEntity(hm.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

		return nil, err
	}

	return result, nil
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

	db, err := hm.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("holders_" + organizationID))

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_holder.update_by_id")

	spanUpdate.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromValue(spanUpdate, "app.request.repository_input", holder, nil)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to convert holder to JSON string", err)
	}

	holderToUpdate := &MongoDBModel{}

	if err := holderToUpdate.FromEntity(holder, hm.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

		return nil, err
	}

	bsonData, err := bson.Marshal(holderToUpdate)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to marshal holder", err)

		return nil, err
	}

	var updateDocument bson.M
	if err := bson.Unmarshal(bsonData, &updateDocument); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to unmarshal holder", err)

		return nil, err
	}

	update := mongoUtils.BuildDocumentToPatch(updateDocument, fieldsToRemove)

	updateResult, err := coll.UpdateByID(ctx, id, update)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to update holder", err)

		return nil, err
	}

	if updateResult.MatchedCount == 0 {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Holder not found", cn.ErrHolderNotFound)

		return nil, pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())
	}

	spanUpdate.End()

	var record MongoDBModel

	ctx, spanFind := tracer.Start(ctx, "mongodb.update_holder.find_by_id")

	spanFind.SetAttributes(attributes...)

	err = coll.FindOne(ctx, bson.M{"_id": id}).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanFind, "Failed to find holder after update", err)

		return nil, err
	}

	spanFind.End()

	result, err := record.ToEntity(hm.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

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

	db, err := hm.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	opts := options.Delete()

	coll := db.Collection(strings.ToLower("holders_" + organizationID))

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
			libOpenTelemetry.HandleSpanError(spanDelete, "Failed to delete holder", err)

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
			libOpenTelemetry.HandleSpanError(spanDelete, "Failed to delete holder", err)

			return err
		}

		if updateResult.MatchedCount == 0 {
			return pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())
		}
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintln("Deleted a document with id: ", id.String()))

	return nil
}
