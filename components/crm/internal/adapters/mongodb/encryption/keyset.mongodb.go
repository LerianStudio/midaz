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

const keysetCollection = "organization_keyset"

// KeysetRepository provides an interface for operations related to keyset entities.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=keyset.mongodb_mock.go --package=encryption . KeysetRepository
type KeysetRepository interface {
	Save(ctx context.Context, keyset *mmodel.OrganizationKeyset) error
	Get(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error)
	GetByVersion(ctx context.Context, organizationID string, version int) (*mmodel.OrganizationKeyset, error)
	GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error)
	Update(ctx context.Context, keyset *mmodel.OrganizationKeyset, expectedRevision int64) error
}

// KeysetMongoDBRepository is a MongoDB-specific implementation of KeysetRepository.
type KeysetMongoDBRepository struct {
	connection *libMongo.Client
}

// NewKeysetMongoDBRepository returns a new instance of KeysetMongoDBRepository using the given MongoDB connection.
// In multi-tenant mode, connection may be nil — the per-request tenant context provides the database.
func NewKeysetMongoDBRepository(connection *libMongo.Client) (*KeysetMongoDBRepository, error) {
	r := &KeysetMongoDBRepository{
		connection: connection,
	}

	if connection != nil {
		if _, err := r.connection.Database(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to connect to MongoDB for keyset repository: %w", err)
		}
	}

	return r, nil
}

func (r *KeysetMongoDBRepository) Save(ctx context.Context, keyset *mmodel.OrganizationKeyset) error {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "mongodb.keyset.save")
	defer span.End()

	if keyset == nil {
		return fmt.Errorf("keyset is required")
	}

	tenantID := extractTenantID(ctx)

	span.SetAttributes(
		attribute.String("app.request.tenant_id", tenantID),
		attribute.String("app.request.organization_id", keyset.OrganizationID),
	)

	if err := keyset.Validate(); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Keyset validation failed", err)
		return err
	}

	if keyset.Revision == 0 {
		keyset.Revision = 1
	}

	// Ensure tenant_id is set on the keyset
	if keyset.TenantID == "" {
		keyset.TenantID = tenantID
	}

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)
		return err
	}

	if err := r.ensureIndexes(ctx, collection); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create keyset indexes", err)
		return fmt.Errorf("create keyset indexes: %w", err)
	}

	model := KeysetFromEntity(keyset)

	// Database isolation handles multi-tenancy - filter by organization_id and
	// version so each keyset version is an independent document (storage is
	// version-keyed).
	filter := bson.M{"organization_id": keyset.OrganizationID, "version": keyset.Version}
	update := bson.M{"$setOnInsert": model}

	result, err := collection.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to save organization keyset", err)
		return fmt.Errorf("save organization keyset: %w", err)
	}

	if result.MatchedCount > 0 {
		return mmodel.ErrKeysetAlreadyExists
	}

	return nil
}

func (r *KeysetMongoDBRepository) Get(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "mongodb.keyset.get")
	defer span.End()

	tenantID := extractTenantID(ctx)

	span.SetAttributes(
		attribute.String("app.request.tenant_id", tenantID),
		attribute.String("app.request.organization_id", organizationID),
	)

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)
		return nil, err
	}

	var model KeysetMongoDBModel

	// Database isolation handles multi-tenancy - filter by organization_id only
	filter := bson.M{"organization_id": organizationID}

	if err := collection.FindOne(ctx, filter).Decode(&model); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, mmodel.ErrKeysetNotFound
		}

		libOpenTelemetry.HandleSpanError(span, "Failed to get organization keyset", err)

		return nil, fmt.Errorf("get organization keyset: %w", err)
	}

	return model.ToEntity(), nil
}

// GetByVersion returns the keyset document for an organization at an exact version.
func (r *KeysetMongoDBRepository) GetByVersion(ctx context.Context, organizationID string, version int) (*mmodel.OrganizationKeyset, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "mongodb.keyset.get_by_version")
	defer span.End()

	tenantID := extractTenantID(ctx)

	span.SetAttributes(
		attribute.String("app.request.tenant_id", tenantID),
		attribute.String("app.request.organization_id", organizationID),
		attribute.Int("app.request.version", version),
	)

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)
		return nil, err
	}

	var model KeysetMongoDBModel

	// Database isolation handles multi-tenancy - filter by organization_id and version.
	filter := bson.M{"organization_id": organizationID, "version": version}

	if err := collection.FindOne(ctx, filter).Decode(&model); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, mmodel.ErrKeysetNotFound
		}

		libOpenTelemetry.HandleSpanError(span, "Failed to get organization keyset by version", err)

		return nil, fmt.Errorf("get organization keyset by version: %w", err)
	}

	return model.ToEntity(), nil
}

// GetActive returns the highest-version keyset document for an organization.
func (r *KeysetMongoDBRepository) GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "mongodb.keyset.get_active")
	defer span.End()

	tenantID := extractTenantID(ctx)

	span.SetAttributes(
		attribute.String("app.request.tenant_id", tenantID),
		attribute.String("app.request.organization_id", organizationID),
	)

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)
		return nil, err
	}

	var model KeysetMongoDBModel

	// Database isolation handles multi-tenancy - filter by organization_id, select
	// the highest version document (descending sort, single result).
	filter := bson.M{"organization_id": organizationID}
	opts := options.FindOne().SetSort(bson.D{{Key: "version", Value: -1}})

	if err := collection.FindOne(ctx, filter, opts).Decode(&model); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, mmodel.ErrKeysetNotFound
		}

		libOpenTelemetry.HandleSpanError(span, "Failed to get active organization keyset", err)

		return nil, fmt.Errorf("get active organization keyset: %w", err)
	}

	return model.ToEntity(), nil
}

func (r *KeysetMongoDBRepository) Update(ctx context.Context, keyset *mmodel.OrganizationKeyset, expectedRevision int64) error {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // consistent with codebase pattern

	ctx, span := tracer.Start(ctx, "mongodb.keyset.update")
	defer span.End()

	if keyset == nil {
		return fmt.Errorf("keyset is required")
	}

	tenantID := extractTenantID(ctx)

	span.SetAttributes(
		attribute.String("app.request.tenant_id", tenantID),
		attribute.String("app.request.organization_id", keyset.OrganizationID),
		attribute.Int64("app.request.expected_revision", expectedRevision),
	)

	if err := keyset.Validate(); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Keyset validation failed", err)
		return err
	}

	// Ensure tenant_id is set on the keyset
	if keyset.TenantID == "" {
		keyset.TenantID = tenantID
	}

	collection, err := r.collection(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get collection", err)
		return err
	}

	// Create model from entity and set the new revision on the model, not on the input entity.
	// This prevents mutation of the caller's object if the database operation fails.
	model := KeysetFromEntity(keyset)
	model.Revision = expectedRevision + 1

	// Database isolation handles multi-tenancy - filter by organization_id, version
	// and revision so the optimistic update targets the exact version document.
	filter := bson.M{
		"organization_id": keyset.OrganizationID,
		"version":         keyset.Version,
		"revision":        expectedRevision,
	}

	result, err := collection.ReplaceOne(ctx, filter, model)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to update organization keyset", err)
		return fmt.Errorf("update organization keyset: %w", err)
	}

	if result.MatchedCount == 0 {
		return mmodel.ErrKeysetRevisionConflict
	}

	span.SetAttributes(attribute.Int64("db.rows_affected", result.ModifiedCount))

	return nil
}

// getDatabase resolves the MongoDB database for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific *mongo.Database into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *KeysetMongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
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

func (r *KeysetMongoDBRepository) collection(ctx context.Context) (*mongo.Collection, error) {
	db, err := r.getDatabase(ctx)
	if err != nil {
		return nil, err
	}

	return db.Collection(keysetCollection), nil
}

// ensureIndexes ensures indexes exist for the keyset collection.
// Uses per-database tracking to handle multi-tenant mode correctly.
// Retries on failure — indexes are only marked as done after successful creation.
func (r *KeysetMongoDBRepository) ensureIndexes(ctx context.Context, collection *mongo.Collection) error {
	key := collection.Database().Name() + ":" + keysetCollection

	return globalIndexTracker.ensureOnce(key, func() error {
		return r.createIndexes(ctx, collection)
	})
}

// createIndexes ensures indexes exist for the keyset collection.
func (r *KeysetMongoDBRepository) createIndexes(ctx context.Context, collection *mongo.Collection) error {
	indexModels := []mongo.IndexModel{
		{
			// Compound unique index on tenant_id + organization_id + version for
			// proper tenant isolation and version-keyed keyset storage.
			Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "organization_id", Value: 1}, {Key: "version", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexModels)

	return err
}

// extractTenantID extracts tenant ID from context or returns "default" for single-tenant mode.
func extractTenantID(ctx context.Context) string {
	if tenantID := tmcore.GetTenantIDContext(ctx); tenantID != "" {
		return tenantID
	}

	return "default"
}

var _ KeysetRepository = (*KeysetMongoDBRepository)(nil)
