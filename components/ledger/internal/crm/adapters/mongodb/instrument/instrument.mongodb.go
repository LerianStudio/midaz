// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package instrument

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/dupkey"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	mongoUtils "github.com/LerianStudio/midaz/v4/pkg/mongo"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Repository provides an interface for operations related to instrument entities.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=instrument.mongodb_mock.go --package=instrument . Repository
type Repository interface {
	Create(ctx context.Context, organizationID string, input *mmodel.Instrument) (*mmodel.Instrument, error)
	Find(ctx context.Context, organizationID string, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Instrument, error)
	Update(ctx context.Context, organizationID string, holderID, id uuid.UUID, input *mmodel.Instrument, fieldsToRemove []string) (*mmodel.Instrument, error)
	FindAll(ctx context.Context, organizationID string, holderID uuid.UUID, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Instrument, error)
	Delete(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error
	DeleteRelatedParty(ctx context.Context, organizationID string, holderID, instrumentID, relatedPartyID uuid.UUID) error
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
func (am *MongoDBRepository) Create(ctx context.Context, organizationID string, alias *mmodel.Instrument) (*mmodel.Instrument, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

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
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	err = createIndexes(ctx, coll)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create indexes", err)

		return nil, err
	}

	record := &MongoDBModel{}

	if err := record.FromEntity(alias, am.DataSecurity); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	_, spanInsert := tracer.Start(ctx, "mongodb.create_alias.insert")
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
		if indexName, ok := dupkey.ClassifyDuplicateKey(err); ok {
			if strings.HasPrefix(indexName, "account_id") || strings.HasPrefix(indexName, "ledger_id_1_account_id") {
				businessErr := pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, cn.EntityInstrument)
				libOpentelemetry.HandleSpanBusinessErrorEvent(spanInsert, "Account already associated with an alias", businessErr)

				return nil, businessErr
			}
		}

		libOpentelemetry.HandleSpanError(spanInsert, "Failed to insert alias", err)

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
}

// Find an alias by holder and alias id
func (am *MongoDBRepository) Find(ctx context.Context, organizationID string, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Instrument, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

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
		if errors.Is(err, mongo.ErrNoDocuments) {
			businessErr := pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, cn.EntityInstrument)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Alias not found", businessErr)

			return nil, businessErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to find account", err)

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
}

func (am *MongoDBRepository) Update(ctx context.Context, organizationID string, holderID, id uuid.UUID, alias *mmodel.Instrument, fieldsToRemove []string) (*mmodel.Instrument, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
		attribute.StringSlice("app.request.fields_to_remove", fieldsToRemove),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	_, spanUpdate := tracer.Start(ctx, "mongodb.update_alias.update_by_id")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	spanUpdate.SetAttributes(
		attribute.Bool("app.request.repository_input.has_metadata", len(alias.Metadata) > 0),
		attribute.Bool("app.request.repository_input.has_banking_details", alias.BankingDetails != nil),
		attribute.Bool("app.request.repository_input.has_regulatory_fields", alias.RegulatoryFields != nil),
		attribute.Int("app.request.repository_input.related_parties_count", len(alias.RelatedParties)),
	)

	aliasToUpdate := &MongoDBModel{}

	if err := aliasToUpdate.FromEntity(alias, am.DataSecurity); err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to convert alias to model", err)

		return nil, err
	}

	bsonData, err := bson.Marshal(aliasToUpdate)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to marshal alias", err)

		return nil, err
	}

	var updateDocument bson.M
	if err := bson.Unmarshal(bsonData, &updateDocument); err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to unmarshal alias", err)

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
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update alias", err)

		return nil, err
	}

	if updateResult.MatchedCount == 0 {
		businessErr := pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, cn.EntityInstrument)
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "Alias not found", businessErr)

		return nil, businessErr
	}

	var record MongoDBModel

	_, spanFind := tracer.Start(ctx, "mongodb.update_alias.find_by_id")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find alias after update", err)

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
}

// Delete remove an alias
func (am *MongoDBRepository) Delete(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	}

	span.SetAttributes(attributes...)

	db, err := am.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	opts := options.DeleteOne()

	coll := db.Collection(strings.ToLower("aliases_" + organizationID))

	_, spanDelete := tracer.Start(ctx, "mongodb.delete_alias.delete_one")
	defer spanDelete.End()

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "holder_id", Value: holderID},
		{Key: "deleted_at", Value: nil},
	}

	if hardDelete {
		deleted, err := coll.DeleteOne(ctx, filter, opts)
		if err != nil {
			libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete alias", err)

			return err
		}

		if deleted.DeletedCount == 0 {
			businessErr := pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, cn.EntityInstrument)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanDelete, "Alias not found", businessErr)

			return businessErr
		}
	} else {
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "deleted_at", Value: time.Now()},
			}},
		}

		updateResult, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete alias", err)

			return err
		}

		if updateResult.MatchedCount == 0 {
			businessErr := pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, cn.EntityInstrument)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanDelete, "Alias not found", businessErr)

			return businessErr
		}
	}

	return nil
}
