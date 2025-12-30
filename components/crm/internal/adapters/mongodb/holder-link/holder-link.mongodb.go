package holderlink

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
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	mongoUtils "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Repository provides an interface for operations related to holder link entities.
//
//go:generate mockgen --destination=holder-link.mock.go --package=holderlink . Repository
type Repository interface {
	Create(ctx context.Context, organizationID string, input *mmodel.HolderLink) (*mmodel.HolderLink, error)
	Find(ctx context.Context, organizationID string, id uuid.UUID, includeDeleted bool) (*mmodel.HolderLink, error)
	FindByAliasIDAndLinkType(ctx context.Context, organizationID string, aliasID uuid.UUID, linkType string, includeDeleted bool) (*mmodel.HolderLink, error)
	FindByAliasID(ctx context.Context, organizationID string, aliasID uuid.UUID, includeDeleted bool) ([]*mmodel.HolderLink, error)
	FindByHolderID(ctx context.Context, organizationID string, holderID uuid.UUID, includeDeleted bool) ([]*mmodel.HolderLink, error)
	FindAll(ctx context.Context, organizationID string, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.HolderLink, error)
	Update(ctx context.Context, organizationID string, id uuid.UUID, input *mmodel.HolderLink, fieldsToRemove []string) (*mmodel.HolderLink, error)
	Delete(ctx context.Context, organizationID string, id uuid.UUID, hardDelete bool) error
}

// MongoDBRepository is a MongoDB-specific implementation of Repository
type MongoDBRepository struct {
	connection *libMongo.MongoConnection
	Database   string
}

// NewMongoDBRepository returns a new instance of MongoDBRepository using the given MongoDB connection
func NewMongoDBRepository(connection *libMongo.MongoConnection) *MongoDBRepository {
	r := &MongoDBRepository{
		connection: connection,
		Database:   connection.Database,
	}
	if _, err := r.connection.GetDB(context.Background()); err != nil {
		panic("Failed to connect mongo")
	}

	return r
}

// Create inserts a holder link into mongo
func (hlm *MongoDBRepository) Create(ctx context.Context, organizationID string, holderLink *mmodel.HolderLink) (*mmodel.HolderLink, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_holder_link")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	}

	span.SetAttributes(attributes...)

	db, err := hlm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Database(strings.ToLower(hlm.Database)).Collection(strings.ToLower("holder_links_" + organizationID))

	err = createIndexes(ctx, coll)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create indexes", err)
		return nil, err
	}

	record := &MongoDBModel{}
	record.FromEntity(holderLink)

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_holder_link.insert")
	defer spanInsert.End()

	spanInsert.SetAttributes(attributes...)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&spanInsert, "app.request.repository_input", record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanInsert, "Failed to set span attributes", err)
	}

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanInsert, "Failed to insert holder link", err)

		if mongo.IsDuplicateKeyError(err) {
			errorType, isKnownError := getDuplicateKeyErrorType(err)
			if isKnownError {
				switch errorType {
				case "duplicate_holder_link":
					return nil, pkg.ValidateBusinessError(cn.ErrDuplicateHolderLink, reflect.TypeOf(mmodel.HolderLink{}).Name())
				case "primary_holder_exists":
					return nil, pkg.ValidateBusinessError(cn.ErrPrimaryHolderAlreadyExists, reflect.TypeOf(mmodel.HolderLink{}).Name())
				}
			}
		}

		return nil, err
	}

	result := record.ToEntity()

	return result, nil
}

// Find a holder link by id
func (hlm *MongoDBRepository) Find(ctx context.Context, organizationID string, id uuid.UUID, includeDeleted bool) (*mmodel.HolderLink, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_holder_link")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_link_id", id.String()),
	}

	span.SetAttributes(attributes...)

	db, err := hlm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Database(strings.ToLower(hlm.Database)).Collection(strings.ToLower("holder_links_" + organizationID))

	var record MongoDBModel

	filter := bson.D{
		{Key: "_id", Value: id},
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to find holder link", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(cn.ErrHolderLinkNotFound, reflect.TypeOf(mmodel.HolderLink{}).Name())
		}

		return nil, err
	}

	result := record.ToEntity()

	return result, nil
}

// FindByAliasIDAndLinkType finds a holder link by alias ID and link type
func (hlm *MongoDBRepository) FindByAliasIDAndLinkType(ctx context.Context, organizationID string, aliasID uuid.UUID, linkType string, includeDeleted bool) (*mmodel.HolderLink, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_holder_link_by_alias_and_type")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.alias_id", aliasID.String()),
		attribute.String("app.request.link_type", linkType),
	}

	span.SetAttributes(attributes...)

	db, err := hlm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Database(strings.ToLower(hlm.Database)).Collection(strings.ToLower("holder_links_" + organizationID))

	var record MongoDBModel

	filter := bson.D{
		{Key: "alias_id", Value: aliasID},
		{Key: "link_type", Value: linkType},
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to find holder link by alias and type", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		return nil, err
	}

	result := record.ToEntity()

	return result, nil
}

// FindByAliasID finds all holder links by alias ID
func (hlm *MongoDBRepository) FindByAliasID(ctx context.Context, organizationID string, aliasID uuid.UUID, includeDeleted bool) ([]*mmodel.HolderLink, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_holder_links_by_alias_id")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.alias_id", aliasID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := hlm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Database(strings.ToLower(hlm.Database)).Collection(strings.ToLower("holder_links_" + organizationID))

	filter := bson.D{
		{Key: "alias_id", Value: aliasID},
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_holder_links_by_alias_id.find")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to find holder links by alias id", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	var holderLinks []*mmodel.HolderLink

	for cursor.Next(ctx) {
		var record MongoDBModel
		if err := cursor.Decode(&record); err != nil {
			libOpenTelemetry.HandleSpanError(&spanFind, "Failed to decode holder link", err)
			return nil, err
		}

		holderLinks = append(holderLinks, record.ToEntity())
	}

	if err := cursor.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to iterate holder links", err)
		return nil, err
	}

	return holderLinks, nil
}

// FindAll returns all holder links matching the filter
func (hlm *MongoDBRepository) FindAll(ctx context.Context, organizationID string, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.HolderLink, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_holder_links")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	}

	span.SetAttributes(attributes...)

	db, err := hlm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Database(strings.ToLower(hlm.Database)).Collection(strings.ToLower("holder_links_" + organizationID))

	filterDoc, err := buildHolderLinkFilter(filter, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to build filter", err)
		return nil, err
	}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_all_holder_links.find")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	limit := int64(filter.Limit)
	skip := int64(filter.Page*filter.Limit - filter.Limit)
	opts := options.FindOptions{Limit: &limit, Skip: &skip}

	cursor, err := coll.Find(ctx, filterDoc, &opts)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to find holder links", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	var holderLinks []*mmodel.HolderLink

	for cursor.Next(ctx) {
		var record MongoDBModel
		if err := cursor.Decode(&record); err != nil {
			libOpenTelemetry.HandleSpanError(&spanFind, "Failed to decode holder link", err)
			return nil, err
		}

		holderLinks = append(holderLinks, record.ToEntity())
	}

	if err := cursor.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to iterate holder links", err)
		return nil, err
	}

	return holderLinks, nil
}

// Update updates a holder link by id
func (hlm *MongoDBRepository) Update(ctx context.Context, organizationID string, id uuid.UUID, holderLink *mmodel.HolderLink, fieldsToRemove []string) (*mmodel.HolderLink, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_holder_link")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_link_id", id.String()),
		attribute.StringSlice("app.request.fields_to_remove", fieldsToRemove),
	}

	span.SetAttributes(attributes...)

	db, err := hlm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Database(strings.ToLower(hlm.Database)).Collection(strings.ToLower("holder_links_" + organizationID))

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_holder_link.update_by_id")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	holderLinkToUpdate := &MongoDBModel{}
	holderLinkToUpdate.FromEntity(holderLink)

	bsonData, err := bson.Marshal(holderLinkToUpdate)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to marshal holder link", err)
		return nil, err
	}

	var updateDocument bson.M
	if err := bson.Unmarshal(bsonData, &updateDocument); err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to unmarshal holder link", err)
		return nil, err
	}

	update := mongoUtils.BuildDocumentToPatch(updateDocument, fieldsToRemove)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "deleted_at", Value: nil},
	}

	updateResult, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to update holder link", err)

		if mongo.IsDuplicateKeyError(err) {
			errorType, isKnownError := getDuplicateKeyErrorType(err)
			if isKnownError {
				switch errorType {
				case "duplicate_holder_link":
					return nil, pkg.ValidateBusinessError(cn.ErrDuplicateHolderLink, reflect.TypeOf(mmodel.HolderLink{}).Name())
				case "primary_holder_exists":
					return nil, pkg.ValidateBusinessError(cn.ErrPrimaryHolderAlreadyExists, reflect.TypeOf(mmodel.HolderLink{}).Name())
				}
			}
		}

		return nil, err
	}

	if updateResult.MatchedCount == 0 {
		return nil, pkg.ValidateBusinessError(cn.ErrHolderLinkNotFound, reflect.TypeOf(mmodel.HolderLink{}).Name())
	}

	var record MongoDBModel

	ctx, spanFind := tracer.Start(ctx, "mongodb.update_holder_link.find_by_id")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to find holder link after update", err)
		return nil, err
	}

	result := record.ToEntity()

	return result, nil
}

// Delete soft deletes or hard deletes a holder link by id
func (hlm *MongoDBRepository) Delete(ctx context.Context, organizationID string, id uuid.UUID, hardDelete bool) error {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_holder_link")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_link_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	}

	span.SetAttributes(attributes...)

	db, err := hlm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)
		return err
	}

	coll := db.Database(strings.ToLower(hlm.Database)).Collection(strings.ToLower("holder_links_" + organizationID))

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "deleted_at", Value: nil},
	}

	if hardDelete {
		ctxDelete, spanDelete := tracer.Start(ctx, "mongodb.delete_holder_link.hard_delete")
		defer spanDelete.End()

		spanDelete.SetAttributes(attributes...)

		result, err := coll.DeleteOne(ctxDelete, filter)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&spanDelete, "Failed to hard delete holder link", err)
			return err
		}

		if result.DeletedCount == 0 {
			return pkg.ValidateBusinessError(cn.ErrHolderLinkNotFound, reflect.TypeOf(mmodel.HolderLink{}).Name())
		}

		return nil
	}

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.delete_holder_link.soft_delete")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"deleted_at": now,
			"updated_at": now,
		},
	}

	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanUpdate, "Failed to soft delete holder link", err)
		return err
	}

	if result.MatchedCount == 0 {
		return pkg.ValidateBusinessError(cn.ErrHolderLinkNotFound, reflect.TypeOf(mmodel.HolderLink{}).Name())
	}

	return nil
}

// buildHolderLinkFilter builds the MongoDB filter based on query parameters
func buildHolderLinkFilter(query http.QueryHeader, includeDeleted bool) (bson.D, error) {
	filter := bson.D{}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	if query.HolderID != nil && *query.HolderID != "" {
		holderID, err := uuid.Parse(*query.HolderID)
		if err != nil {
			return nil, pkg.ValidateBusinessError(cn.ErrInvalidQueryParameter, reflect.TypeOf(mmodel.HolderLink{}).Name(), "holder_id")
		}

		filter = append(filter, bson.E{Key: "holder_id", Value: holderID})
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

// getDuplicateKeyErrorType determines the type of duplicate key error based on KeyPattern
// It uses named indexes to identify which constraint was violated
func getDuplicateKeyErrorType(err error) (string, bool) {
	var writeException mongo.WriteException
	if !errors.As(err, &writeException) {
		return "", false
	}

	for _, writeErr := range writeException.WriteErrors {
		if writeErr.Code == 11000 || writeErr.Code == 11001 {
			if result, found := checkErrorByIndexName(writeErr.Message); found {
				return result, true
			}

			return checkErrorByKeyPatternFromMessage(writeErr.Message)
		}
	}

	return "", false
}

// checkErrorByIndexName determines error type by checking which index name appears in the error message
func checkErrorByIndexName(errMsg string) (string, bool) {
	if strings.Contains(errMsg, "alias_id_link_type_unique") {
		return "duplicate_holder_link", true
	}

	if strings.Contains(errMsg, "alias_id_primary_holder_unique") {
		return "primary_holder_exists", true
	}

	return "", false
}

// checkErrorByKeyPatternFromMessage extracts KeyPattern from error message and determines the error type
func checkErrorByKeyPatternFromMessage(errMsg string) (string, bool) {
	dupKeyIndex := strings.Index(errMsg, "dup key:")
	if dupKeyIndex < 0 {
		return "", false
	}

	dupKeySection := errMsg[dupKeyIndex:]

	hasAliasID := strings.Contains(dupKeySection, "alias_id")
	hasLinkType := strings.Contains(dupKeySection, "link_type")

	if hasAliasID && hasLinkType {
		return "duplicate_holder_link", true
	}

	if hasAliasID {
		if strings.Contains(errMsg, string(mmodel.LinkTypePrimaryHolder)) {
			return "primary_holder_exists", true
		}

		if strings.Contains(errMsg, "alias_id_primary_holder_unique") {
			return "primary_holder_exists", true
		}
	}

	return "", false
}

// createIndexes creates indexes for specific fields, if it not exists
func createIndexes(ctx context.Context, collection *mongo.Collection) error {
	indexModels := []mongo.IndexModel{
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
			Keys: bson.D{
				{Key: "alias_id", Value: 1},
			},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		{
			Keys: bson.D{
				{Key: "alias_id", Value: 1},
				{Key: "link_type", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetName("alias_id_link_type_unique").
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
				}),
		},
		// Unique index: only one PRIMARY_HOLDER per alias
		{
			Keys: bson.D{
				{Key: "alias_id", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetName("alias_id_primary_holder_unique").
				SetPartialFilterExpression(bson.D{
					{Key: "deleted_at", Value: nil},
					{Key: "link_type", Value: string(mmodel.LinkTypePrimaryHolder)},
				}),
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateMany(ctx, indexModels)

	return err
}

// FindByHolderID finds all holder links by holder ID
func (hlm *MongoDBRepository) FindByHolderID(ctx context.Context, organizationID string, holderID uuid.UUID, includeDeleted bool) ([]*mmodel.HolderLink, error) {
	_, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_holder_links_by_holder_id")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := hlm.connection.GetDB(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Database(strings.ToLower(hlm.Database)).Collection(strings.ToLower("holder_links_" + organizationID))

	filter := bson.D{
		{Key: "holder_id", Value: holderID},
	}

	if !includeDeleted {
		filter = append(filter, bson.E{Key: "deleted_at", Value: nil})
	}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_holder_links_by_holder_id.find")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to find holder links by holder id", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	var holderLinks []*mmodel.HolderLink

	for cursor.Next(ctx) {
		var record MongoDBModel
		if err := cursor.Decode(&record); err != nil {
			libOpenTelemetry.HandleSpanError(&spanFind, "Failed to decode holder link", err)
			return nil, err
		}

		holderLinks = append(holderLinks, record.ToEntity())
	}

	if err := cursor.Err(); err != nil {
		libOpenTelemetry.HandleSpanError(&spanFind, "Failed to iterate holder links", err)
		return nil, err
	}

	return holderLinks, nil
}
