// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

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

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	mongoUtils "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Repository provides an interface for operations related to alias entities.
//
//go:generate mockgen --destination=alias.mongodb_mock.go --package=alias . Repository
type Repository interface {
	Create(ctx context.Context, organizationID string, input *mmodel.Alias) (*mmodel.Alias, error)
	Find(ctx context.Context, organizationID string, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Alias, error)
	Update(ctx context.Context, organizationID string, holderID, id uuid.UUID, input *mmodel.Alias, fieldsToRemove []string) (*mmodel.Alias, error)
	FindAll(ctx context.Context, organizationID string, holderID uuid.UUID, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Alias, error)
	Delete(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error
	DeleteRelatedParty(ctx context.Context, organizationID string, holderID, aliasID, relatedPartyID uuid.UUID) error
	Count(ctx context.Context, organizationID string, holderID uuid.UUID) (int64, error)
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
	}

	return r, nil
}

// getDatabase resolves the MongoDB database for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific *mongo.Database into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (am *MongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
	if am.connection == nil {
		// Check tenant context when static connection is nil (multi-tenant mode without static fallback)
		if db := tmcore.GetMBContext(ctx); db != nil {
			return db, nil
		}

		return nil, fmt.Errorf("no database connection available: multi-tenant context required but not present, and no static connection configured")
	}

	if db := tmcore.GetMBContext(ctx); db != nil {
		return db, nil
	}

	return am.connection.Database(ctx)
}

// Create inserts an alias into mongo
func (am *MongoDBRepository) Create(ctx context.Context, organizationID string, alias *mmodel.Alias) (*mmodel.Alias, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", alias.HolderID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	err = createIndexes(ctx, coll)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create indexes", err)

		return nil, err
	}

	ctx, spanCount := tracer.Start(ctx, "mongodb.create_alias.count_existing")
	defer spanCount.End()

	spanCount.SetAttributes(attributes...)

	record := &MongoDBModel{}

	if err := record.FromEntity(alias, am.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_alias.insert")
	defer spanInsert.End()

	spanInsert.SetAttributes(attributes...)

	spanInsert.SetAttributes(
		attribute.Bool("app.request.repository_input.has_metadata", len(record.Metadata) > 0),
		attribute.Bool("app.request.repository_input.has_banking_details", record.BankingDetails != nil),
		attribute.Bool("app.request.repository_input.has_regulatory_fields", record.RegulatoryFields != nil),
		attribute.Int("app.request.repository_input.related_parties_count", len(record.RelatedParties)),
	)

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanInsert, "Failed to insert alias", err)

		if mongo.IsDuplicateKeyError(err) {
			if strings.Contains(err.Error(), "account_id") {
				return nil, pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, reflect.TypeOf(mmodel.Alias{}).Name())
			}
		}

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
}

// Find an alias by holder and alias id
func (am *MongoDBRepository) Find(ctx context.Context, organizationID string, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Alias, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	var record MongoDBModel

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "holder_id", Value: holderID},
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to find account", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
		}

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
}

func (am *MongoDBRepository) Update(ctx context.Context, organizationID string, holderID, id uuid.UUID, alias *mmodel.Alias, fieldsToRemove []string) (*mmodel.Alias, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.StringSlice("app.request.fields_to_remove", fieldsToRemove),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_alias.update_by_id")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromValue(spanUpdate, "app.request.repository_input", alias, nil)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to set span attributes", err)
	}

	aliasToUpdate := &MongoDBModel{}

	if err := aliasToUpdate.FromEntity(alias, am.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to convert alias to model", err)

		return nil, err
	}

	bsonData, err := bson.Marshal(aliasToUpdate)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to marshal alias", err)

		return nil, err
	}

	var updateDocument bson.M
	if err := bson.Unmarshal(bsonData, &updateDocument); err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to unmarshal alias", err)

		return nil, err
	}

	update := mongoUtils.BuildDocumentToPatch(updateDocument, fieldsToRemove)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "holder_id", Value: holderID},
		{Key: "deleted_at", Value: nil},
	}

	updateResult, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanUpdate, "Failed to update alias", err)

		return nil, err
	}

	if updateResult.MatchedCount == 0 {
		return nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
	}

	var record MongoDBModel

	ctx, spanFind := tracer.Start(ctx, "mongodb.update_alias.find_by_id")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanFind, "Failed to find alias after update", err)

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(spanFind, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
}

// Delete remove an alias
func (am *MongoDBRepository) Delete(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	opts := options.Delete()

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	ctx, spanDelete := tracer.Start(ctx, "mongodb.delete_alias.delete_one")

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "holder_id", Value: holderID},
		{Key: "deleted_at", Value: nil},
	}

	if hardDelete {
		deleted, err := coll.DeleteOne(ctx, filter, opts)
		if err != nil {
			libOpenTelemetry.HandleSpanError(spanDelete, "Failed to delete alias", err)

			return err
		}

		spanDelete.End()

		if deleted.DeletedCount == 0 {
			return pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
		}
	} else {
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "deleted_at", Value: time.Now()},
			}},
		}

		updateResult, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			libOpenTelemetry.HandleSpanError(spanDelete, "Failed to delete alias", err)

			return err
		}

		if updateResult.MatchedCount == 0 {
			return pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
		}
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintln("Deleted a document with id: ", id.String(), " (hard delete: ", hardDelete, ")"))

	return nil
}
