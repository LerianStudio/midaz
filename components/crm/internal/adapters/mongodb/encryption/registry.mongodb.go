// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

const registryCollection = "organization_registry"

// RegistryRepository provides an interface for operations related to registry entities.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=registry.mongodb_mock.go --package=encryption . RegistryRepository
type RegistryRepository interface {
	Save(ctx context.Context, record *mmodel.OrganizationRegistryRecord) error
	Get(ctx context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error)
	Update(ctx context.Context, record *mmodel.OrganizationRegistryRecord, expectedRevision int64) error
}

// RegistryMongoDBRepository is a MongoDB-specific implementation of RegistryRepository.
type RegistryMongoDBRepository struct {
	connection *libMongo.Client
}

// NewRegistryMongoDBRepository returns a new instance of RegistryMongoDBRepository using the given MongoDB connection.
// In multi-tenant mode, connection may be nil — the per-request tenant context provides the database.
func NewRegistryMongoDBRepository(connection *libMongo.Client) (*RegistryMongoDBRepository, error) {
	r := &RegistryMongoDBRepository{
		connection: connection,
	}

	if connection != nil {
		if _, err := r.connection.Database(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to connect to MongoDB for registry repository: %w", err)
		}
	}

	return r, nil
}

func (r *RegistryMongoDBRepository) Save(ctx context.Context, record *mmodel.OrganizationRegistryRecord) error {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "mongodb.registry.save")
	defer span.End()

	if record == nil {
		return fmt.Errorf("registry record is required")
	}

	span.SetAttributes(attribute.String("app.request.organization_id", record.OrganizationID))

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)
		return err
	}

	if err := r.ensureIndexes(ctx, collection); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create registry indexes", err)
		return fmt.Errorf("create registry indexes: %w", err)
	}

	model := RegistryFromEntity(record)

	filter := bson.M{"organization_id": record.OrganizationID}
	update := bson.M{"$setOnInsert": model}

	result, err := collection.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to save organization registry", err)
		return fmt.Errorf("save organization registry: %w", err)
	}

	if result.MatchedCount > 0 {
		return mmodel.ErrRegistryAlreadyExists
	}

	return nil
}

func (r *RegistryMongoDBRepository) Get(ctx context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "mongodb.registry.get")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.organization_id", organizationID))

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)
		return nil, err
	}

	var model RegistryMongoDBModel

	if err := collection.FindOne(ctx, bson.M{"organization_id": organizationID}).Decode(&model); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, mmodel.ErrRegistryNotFound
		}

		libOpenTelemetry.HandleSpanError(span, "Failed to get organization registry", err)

		return nil, fmt.Errorf("get organization registry: %w", err)
	}

	return model.ToEntity(), nil
}

func (r *RegistryMongoDBRepository) Update(ctx context.Context, record *mmodel.OrganizationRegistryRecord, expectedRevision int64) error {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "mongodb.registry.update")
	defer span.End()

	if record == nil {
		return fmt.Errorf("registry record is required")
	}

	span.SetAttributes(
		attribute.String("app.request.organization_id", record.OrganizationID),
		attribute.Int64("app.request.expected_revision", expectedRevision),
	)

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)
		return err
	}

	// Create model from entity and set the new revision on the model, not on the input entity.
	// This prevents mutation of the caller's object if the database operation fails.
	model := RegistryFromEntity(record)
	model.Revision = expectedRevision + 1

	result, err := collection.ReplaceOne(ctx, bson.M{"organization_id": record.OrganizationID, "revision": expectedRevision}, model)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to update organization registry", err)
		return fmt.Errorf("update organization registry: %w", err)
	}

	if result.MatchedCount == 0 {
		return mmodel.ErrRegistryRevisionConflict
	}

	span.SetAttributes(attribute.Int64("db.rows_affected", result.ModifiedCount))

	return nil
}

// getDatabase resolves the MongoDB database for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific *mongo.Database into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *RegistryMongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
	if r.connection == nil {
		if db := tmcore.GetMBContext(ctx); db != nil {
			return db, nil
		}

		return nil, fmt.Errorf("no database connection available: multi-tenant context required but not present, and no static connection configured")
	}

	if db := tmcore.GetMBContext(ctx); db != nil {
		return db, nil
	}

	return r.connection.Database(ctx)
}

func (r *RegistryMongoDBRepository) collection(ctx context.Context) (*mongo.Collection, error) {
	db, err := r.getDatabase(ctx)
	if err != nil {
		return nil, err
	}

	return db.Collection(registryCollection), nil
}

// ensureIndexes ensures indexes exist for the registry collection.
// Uses per-database tracking to handle multi-tenant mode correctly.
// Retries on failure — indexes are only marked as done after successful creation.
func (r *RegistryMongoDBRepository) ensureIndexes(ctx context.Context, collection *mongo.Collection) error {
	key := collection.Database().Name() + ":" + registryCollection

	return globalIndexTracker.ensureOnce(key, func() error {
		return r.createIndexes(ctx, collection)
	})
}

// createIndexes ensures indexes exist for the registry collection.
func (r *RegistryMongoDBRepository) createIndexes(ctx context.Context, collection *mongo.Collection) error {
	indexModels := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "organization_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexModels)

	return err
}

var _ RegistryRepository = (*RegistryMongoDBRepository)(nil)
