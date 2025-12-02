package alias

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v2/commons/crypto"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	mongoUtils "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Repository provides an interface for operations related to alias entities.
//
//go:generate mockgen --destination=alias.mock.go --package=alias . Repository
type Repository interface {
	Create(ctx context.Context, organizationID string, input *mmodel.Alias) (*mmodel.Alias, error)
	Find(ctx context.Context, organizationID string, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Alias, error)
	Update(ctx context.Context, organizationID string, holderID, id uuid.UUID, input *mmodel.Alias, fieldsToRemove []string) (*mmodel.Alias, error)
	FindAll(ctx context.Context, organizationID string, holderID uuid.UUID, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Alias, error)
	Delete(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error
	Count(ctx context.Context, organizationID string, holderID uuid.UUID) (int64, error)
}

// MongoDBRepository is a MongoDB-specific implementation of Repository
type MongoDBRepository struct {
	connection   *libMongo.MongoConnection
	Database     string
	DataSecurity *libCrypto.Crypto
}

// NewMongoDBRepository returns a new instance of MongoDBRepository using the given MongoDB connection
func NewMongoDBRepository(connection *libMongo.MongoConnection, dataSecurity *libCrypto.Crypto) *MongoDBRepository {
	r := &MongoDBRepository{
		connection:   connection,
		Database:     connection.Database,
		DataSecurity: dataSecurity,
	}
	if _, err := r.connection.GetDB(context.Background()); err != nil {
		panic("Failed to connect mongo")
	}

	return r
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

	err := libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", alias)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to set span attributes", err)
	}

	db, err := am.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(am.Database)).Collection(strings.ToLower("aliases_" + organizationID))

	err = createIndexes(coll)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create indexes", err)

		return nil, err
	}

	ctx, spanCount := tracer.Start(ctx, "mongodb.create_alias.count_existing")
	defer spanCount.End()

	spanCount.SetAttributes(attributes...)

	record := &MongoDBModel{}

	if err := record.FromEntity(alias, am.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert alias to model", err)

		return nil, err
	}

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_alias.insert")
	defer spanInsert.End()

	spanInsert.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&spanInsert, "app.request.repository_input", record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanInsert, "Failed to set span attributes", err)
	}

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanInsert, "Failed to insert alias", err)

		if mongo.IsDuplicateKeyError(err) {
			if strings.Contains(err.Error(), "account_id") {
				return nil, pkg.ValidateBusinessError(cn.ErrAccountAlreadyAssociated, reflect.TypeOf(mmodel.Alias{}).Name())
			}
		}

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert alias to model", err)

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

	db, err := am.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(am.Database)).Collection(strings.ToLower("aliases_" + organizationID))

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
		libOpenTelemetry.HandleSpanError(&span, "Failed to find account", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
		}

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert alias to model", err)

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

	err := libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", alias)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to set span attributes", err)
	}

	db, err := am.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(am.Database)).Collection(strings.ToLower("aliases_" + organizationID))

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_alias.update_by_id")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&spanUpdate, "app.request.repository_input", alias)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to set span attributes", err)
	}

	aliasToUpdate := &MongoDBModel{}

	if err := aliasToUpdate.FromEntity(alias, am.DataSecurity); err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to convert alias to model", err)

		return nil, err
	}

	bsonData, err := bson.Marshal(aliasToUpdate)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to marshal alias", err)

		return nil, err
	}

	var updateDocument bson.M
	if err := bson.Unmarshal(bsonData, &updateDocument); err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to unmarshal alias", err)

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
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to update alias", err)

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
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to find alias after update", err)

		return nil, err
	}

	result, err := record.ToEntity(am.DataSecurity)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to convert alias to model", err)

		return nil, err
	}

	return result, nil
}

// FindAll accounts by holder id and filter
func (am *MongoDBRepository) FindAll(ctx context.Context, organizationID string, holderID uuid.UUID, query http.QueryHeader, includeDeleted bool) ([]*mmodel.Alias, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_aliases")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	}

	span.SetAttributes(attributes...)

	err := libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", query)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to set span attributes", err)
	}

	db, err := am.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(am.Database)).Collection(strings.ToLower("aliases_" + organizationID))

	limit := int64(query.Limit)
	skip := int64(query.Page*query.Limit - query.Limit)
	opts := options.FindOptions{Limit: &limit, Skip: &skip}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_all_alias.find")

	spanFind.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&spanFind, "app.request.repository_filter", query)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to set span attributes", err)
	}

	filter, err := am.buildAliasFilter(query, holderID, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Invalid metadata value", err)
		return nil, err
	}

	cursor, err := coll.Find(ctx, filter, &opts)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to find aliases", err)

		return nil, err
	}

	spanFind.End()

	var aliases []*MongoDBModel

	for cursor.Next(ctx) {
		var holder MongoDBModel
		if err := cursor.Decode(&holder); err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to decode aliases", err)

			return nil, err
		}

		aliases = append(aliases, &holder)
	}

	if err := cursor.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to iterate aliases", err)

		return nil, err
	}

	if err := cursor.Close(ctx); err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to close cursor", err)

		return nil, err
	}

	results := make([]*mmodel.Alias, len(aliases))
	for i, alias := range aliases {
		results[i], err = alias.ToEntity(am.DataSecurity)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to convert alias to model", err)

			return nil, err
		}
	}

	return results, nil
}

func (am *MongoDBRepository) buildAliasFilter(query http.QueryHeader, holderID uuid.UUID, includeDeleted bool) (bson.D, error) {
	filter := bson.D{}

	if holderID != uuid.Nil {
		filter = append(filter, bson.E{Key: "holder_id", Value: holderID})
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	if !libCommons.IsNilOrEmpty(query.AccountID) {
		filter = append(filter, bson.E{Key: "account_id", Value: *query.AccountID})
	}

	if !libCommons.IsNilOrEmpty(query.LedgerID) {
		filter = append(filter, bson.E{Key: "ledger_id", Value: *query.LedgerID})
	}

	if !libCommons.IsNilOrEmpty(query.Document) {
		documentHash := am.DataSecurity.GenerateHash(query.Document)
		filter = append(filter, bson.E{Key: "search.document", Value: documentHash})
	}

	if !libCommons.IsNilOrEmpty(query.BankingDetailsAccount) {
		bankingDetailsAccountHash := am.DataSecurity.GenerateHash(query.BankingDetailsAccount)
		filter = append(filter, bson.E{Key: "search.banking_details_account", Value: bankingDetailsAccountHash})
	}

	if !libCommons.IsNilOrEmpty(query.BankingDetailsIban) {
		bankingDetailsIbanHash := am.DataSecurity.GenerateHash(query.BankingDetailsIban)
		filter = append(filter, bson.E{Key: "search.banking_details_iban", Value: bankingDetailsIbanHash})
	}

	if !libCommons.IsNilOrEmpty(query.BankingDetailsBranch) {
		filter = append(filter, bson.E{Key: "banking_details.branch", Value: *query.BankingDetailsBranch})
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

	db, err := am.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return err
	}

	opts := options.Delete()

	coll := db.Database(strings.ToLower(am.Database)).Collection(strings.ToLower("aliases_" + organizationID))

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
			libOpenTelemetry.HandleSpanError(&spanDelete, "Failed to delete alias", err)

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
			libOpenTelemetry.HandleSpanError(&spanDelete, "Failed to delete alias", err)

			return err
		}

		if updateResult.MatchedCount == 0 {
			return pkg.ValidateBusinessError(cn.ErrAliasNotFound, reflect.TypeOf(mmodel.Alias{}).Name())
		}
	}

	logger.Infoln("Deleted a document with id: ", id.String(), " (hard delete: ", hardDelete, ")")

	return nil
}

func (am *MongoDBRepository) Count(ctx context.Context, organizationID string, holderID uuid.UUID) (int64, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_alias")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := am.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)

		return 0, err
	}

	coll := db.Database(strings.ToLower(am.Database)).Collection(strings.ToLower("aliases_" + organizationID))

	ctx, spanCount := tracer.Start(ctx, "mongodb.find_all_alias.find")
	defer spanCount.End()

	spanCount.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "holder_id", Value: holderID},
		{Key: "deleted_at", Value: nil},
	}

	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanCount, "Failed to count aliases by holder", err)

		return 0, err
	}

	return count, nil
}

// createIndexes creates indexes for specific fields, if it not exists
func createIndexes(collection *mongo.Collection) error {
	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "_id", Value: 1},
				{Key: "holder_id", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{
				{Key: "holder_id", Value: 1},
			},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{{Key: "ledger_id", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{{Key: "account_id", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{{Key: "search.document", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{{Key: "search.banking_details_account", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{{Key: "search.banking_details_iban", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{
				{Key: "ledger_id", Value: 1},
				{Key: "account_id", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{
				{Key: "holder_link_id", Value: 1},
			},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateMany(ctx, indexModels)

	return err
}
