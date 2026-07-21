// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/dupkey"
	encryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

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

// Repository provides an interface for operations related to holder entities.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=holder.mongodb_mock.go --package=holder . Repository
type Repository interface {
	Create(ctx context.Context, collection string, input *mmodel.Holder) (*mmodel.Holder, error)
	Find(ctx context.Context, collection string, id uuid.UUID, includeDeleted bool) (*mmodel.Holder, error)
	FindAll(ctx context.Context, collection string, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Holder, error)
	Update(ctx context.Context, collection string, id uuid.UUID, input *mmodel.Holder, fieldsToRemove []string) (*mmodel.Holder, error)
	Delete(ctx context.Context, collection string, id uuid.UUID, hardDelete bool) error
}

// MongoDBRepository is a MongoDB-specific implementation of Repository
type MongoDBRepository struct {
	connection     *libMongo.Client
	FieldEncryptor encryption.FieldEncryptor
}

// NewMongoDBRepository returns a new instance of MongoDBRepository using the given MongoDB connection.
// In multi-tenant mode, connection may be nil — the per-request tenant context provides the database.
func NewMongoDBRepository(connection *libMongo.Client, fieldEncryptor encryption.FieldEncryptor) (*MongoDBRepository, error) {
	if fieldEncryptor == nil {
		return nil, fmt.Errorf("holder repository requires a non-nil FieldEncryptor")
	}

	r := &MongoDBRepository{
		FieldEncryptor: fieldEncryptor,
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
		if db := tmcore.GetMBContext(ctx); db != nil {
			return db, nil
		}

		return nil, fmt.Errorf("no database connection available: multi-tenant context required but not present, and no static connection configured")
	}

	if db := tmcore.GetMBContext(ctx); db != nil {
		return db, nil
	}

	return hm.connection.Database(ctx)
}

// Create inserts a holder into mongo.
func (hm *MongoDBRepository) Create(ctx context.Context, organizationID string, holder *mmodel.Holder) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_holder")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	}

	span.SetAttributes(attributes...)

	db, err := hm.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("holders_" + organizationID))

	err = ensureIndexes(ctx, coll)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create indexes", err)

		return nil, err
	}

	// Build encryption context for this holder
	encryptionCtx := encryption.EncryptionContext{
		TenantID:       encryption.ExtractTenantID(ctx),
		OrganizationID: organizationID,
		RecordID:       holder.ID.String(),
	}

	record := &MongoDBModel{}

	if err := record.FromEntity(ctx, holder, hm.FieldEncryptor, encryptionCtx); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

		return nil, err
	}

	_, spanInsert := tracer.Start(ctx, "mongodb.create_holder.insert")
	defer spanInsert.End()

	spanInsert.SetAttributes(attributes...)

	spanInsert.SetAttributes(repositoryInputAttributes(record)...)

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		if indexName, ok := dupkey.ClassifyDuplicateKey(err); ok {
			if strings.HasPrefix(indexName, "search.document") {
				businessErr := pkg.ValidateBusinessError(cn.ErrDocumentAssociationError, cn.EntityHolder)
				libOpentelemetry.HandleSpanBusinessErrorEvent(spanInsert, "Holder document already associated", businessErr)

				return nil, businessErr
			}
		}

		libOpentelemetry.HandleSpanError(spanInsert, "Failed to insert holder", err)

		return nil, err
	}

	result, err := record.ToEntity(ctx, hm.FieldEncryptor, encryptionCtx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

		return nil, err
	}

	return result, nil
}

// Find fetches a holder by its id
func (hm *MongoDBRepository) Find(ctx context.Context, organizationID string, id uuid.UUID, includeDeleted bool) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

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
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

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

	_, spanFind := tracer.Start(ctx, "mongodb.find_holder.find")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	err = coll.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			businessErr := pkg.ValidateBusinessError(cn.ErrHolderNotFound, cn.EntityHolder)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanFind, "Holder not found", businessErr)

			return nil, businessErr
		}

		libOpentelemetry.HandleSpanError(spanFind, "Failed to find holder", err)

		return nil, err
	}

	// Build encryption context for this holder
	encryptionCtx := encryption.EncryptionContext{
		TenantID:       encryption.ExtractTenantID(ctx),
		OrganizationID: organizationID,
		RecordID:       id.String(),
	}

	result, err := record.ToEntity(ctx, hm.FieldEncryptor, encryptionCtx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

		return nil, err
	}

	return result, nil
}

// Update a holder by id
func (hm *MongoDBRepository) Update(ctx context.Context, organizationID string, id uuid.UUID, holder *mmodel.Holder, fieldsToRemove []string) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

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
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower("holders_" + organizationID))

	_, spanUpdate := tracer.Start(ctx, "mongodb.update_holder.update_by_id")
	defer spanUpdate.End()

	spanUpdate.SetAttributes(attributes...)

	// Build encryption context for this holder
	encryptionCtx := encryption.EncryptionContext{
		TenantID:       encryption.ExtractTenantID(ctx),
		OrganizationID: organizationID,
		RecordID:       id.String(),
	}

	holderToUpdate := &MongoDBModel{}

	if err := holderToUpdate.FromEntity(ctx, holder, hm.FieldEncryptor, encryptionCtx); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

		return nil, err
	}

	spanUpdate.SetAttributes(repositoryInputAttributes(holderToUpdate)...)

	bsonData, err := bson.Marshal(holderToUpdate)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal holder", err)

		return nil, err
	}

	var updateDocument bson.M
	if err := bson.Unmarshal(bsonData, &updateDocument); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal holder", err)

		return nil, err
	}

	update := mongoUtils.BuildDocumentToPatch(updateDocument, fieldsToRemove)

	updateResult, err := coll.UpdateByID(ctx, id, update)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update holder", err)

		return nil, err
	}

	if updateResult.MatchedCount == 0 {
		businessErr := pkg.ValidateBusinessError(cn.ErrHolderNotFound, cn.EntityHolder)
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdate, "Holder not found", businessErr)

		return nil, businessErr
	}

	var record MongoDBModel

	_, spanFind := tracer.Start(ctx, "mongodb.update_holder.find_by_id")
	defer spanFind.End()

	spanFind.SetAttributes(attributes...)

	err = coll.FindOne(ctx, bson.M{"_id": id}).Decode(&record)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find holder after update", err)

		return nil, err
	}

	result, err := record.ToEntity(ctx, hm.FieldEncryptor, encryptionCtx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert holder to model", err)

		return nil, err
	}

	return result, nil
}

// Delete a holder from mongodb by its ID
func (hm *MongoDBRepository) Delete(ctx context.Context, organizationID string, id uuid.UUID, hardDelete bool) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

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
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	opts := options.DeleteOne()

	coll := db.Collection(strings.ToLower("holders_" + organizationID))

	_, spanDelete := tracer.Start(ctx, "mongodb.delete_holder.delete_one")
	defer spanDelete.End()

	spanDelete.SetAttributes(attributes...)

	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "deleted_at", Value: nil},
	}

	if hardDelete {
		deleted, err := coll.DeleteOne(ctx, filter, opts)
		if err != nil {
			libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete holder", err)

			return err
		}

		if deleted.DeletedCount == 0 {
			businessErr := pkg.ValidateBusinessError(cn.ErrHolderNotFound, cn.EntityHolder)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanDelete, "Holder not found", businessErr)

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
			libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete holder", err)

			return err
		}

		if updateResult.MatchedCount == 0 {
			businessErr := pkg.ValidateBusinessError(cn.ErrHolderNotFound, cn.EntityHolder)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanDelete, "Holder not found", businessErr)

			return businessErr
		}
	}

	return nil
}

// repositoryInputAttributes derives non-sensitive presence indicators from an
// already-encrypted holder model for span telemetry. It returns only boolean
// has_* attributes and never serializes field values, so plaintext PII cannot
// leak onto a span. Both Create and Update route through this helper so their
// span attributes cannot drift apart.
func repositoryInputAttributes(m *MongoDBModel) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Bool("app.request.repository_input.has_metadata", len(m.Metadata) > 0),
		attribute.Bool("app.request.repository_input.has_external_id", m.ExternalID != nil),
		attribute.Bool("app.request.repository_input.has_contact", m.Contact != nil),
		attribute.Bool("app.request.repository_input.has_addresses", m.Addresses != nil),
		attribute.Bool("app.request.repository_input.has_natural_person", m.NaturalPerson != nil),
		attribute.Bool("app.request.repository_input.has_legal_person", m.LegalPerson != nil),
	}
}
