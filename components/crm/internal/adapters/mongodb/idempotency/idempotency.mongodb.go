// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package idempotency provides the Mongo-backed repository for CRM
// idempotency records used by the idempotency guard wrapping mutating
// endpoints (create-alias, close-alias).
//
// Storage layout per tenant database:
//
//	collection: idempotency
//	unique index: (tenant_id, idempotency_key)
//	TTL index:    expires_at — Mongo sweeps expired records (default 24h)
//
// Per-request tenant isolation is inherited from the standard CRM pattern:
// the multi-tenant middleware attaches a *mongo.Database handle to the
// request context via tmcore.ContextWithMB, and every repository call goes
// through getDatabase(ctx) which prefers that handle over the static
// connection.
package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// collectionName is the single collection used across all tenants (each tenant
// has its own Mongo database, so collection names do not need tenant scoping).
const collectionName = "idempotency"

// DefaultTTL is the retention window for idempotency records. Matches the
// Ledger-side account-registration retention to keep replay semantics aligned
// across the saga.
const DefaultTTL = 24 * time.Hour

// Record is the persisted form of an idempotency claim. TenantID is stored
// explicitly so lookups remain correct even if a future refactor collapses
// multiple tenants into one database.
type Record struct {
	TenantID         string    `bson:"tenant_id"`
	IdempotencyKey   string    `bson:"idempotency_key"`
	RequestHash      string    `bson:"request_hash"`
	ResponseDocument []byte    `bson:"response_document"`
	CreatedAt        time.Time `bson:"created_at"`
	ExpiresAt        time.Time `bson:"expires_at"`
}

// Repository is the surface consumed by the idempotency guard in
// components/crm/internal/services. It is intentionally tiny: the guard owns
// the state machine; this repository owns durability.
//
//go:generate mockgen --destination=idempotency.mongodb_mock.go --package=idempotency . Repository
type Repository interface {
	// Find returns the stored record for (tenantID, key), or (nil, nil) when
	// no record exists. Infrastructure failures surface as non-nil error.
	Find(ctx context.Context, tenantID, key string) (*Record, error)

	// Store inserts a new idempotency record. A duplicate-key error from Mongo
	// is surfaced so the caller can distinguish between a fresh write and a
	// race with a concurrent request.
	Store(ctx context.Context, rec *Record) error

	// EnsureIndexes creates the unique index and TTL index. Safe to call
	// multiple times — Mongo CreateIndexes is idempotent on identical keys.
	EnsureIndexes(ctx context.Context) error
}

// MongoDBRepository implements Repository against Mongo.
//
// In multi-tenant mode connection may be nil — the request context supplies
// the tenant-scoped database via tmcore.GetMBContext.
type MongoDBRepository struct {
	connection *libMongo.Client
}

// NewMongoDBRepository returns a configured repository. connection may be nil
// in multi-tenant mode.
func NewMongoDBRepository(connection *libMongo.Client) *MongoDBRepository {
	return &MongoDBRepository{connection: connection}
}

// getDatabase mirrors the pattern in the alias/holder adapters: prefer the
// tenant database injected by the middleware, fall back to the static
// connection when single-tenant.
func (r *MongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
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

// Find returns the stored record or (nil, nil) when not found.
func (r *MongoDBRepository) Find(ctx context.Context, tenantID, key string) (*Record, error) {
	_, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.idempotency.find")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.tenant_id", tenantID),
	)

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return nil, err
	}

	coll := db.Collection(collectionName)

	var rec Record

	filter := bson.D{
		{Key: "tenant_id", Value: tenantID},
		{Key: "idempotency_key", Value: key},
	}

	err = coll.FindOne(ctx, filter).Decode(&rec)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		libOpenTelemetry.HandleSpanError(span, "Failed to find idempotency record", err)

		return nil, err
	}

	return &rec, nil
}

// Store inserts a new record. Duplicate-key errors are surfaced to the caller
// so it can treat concurrent writers distinctly from fresh successes.
func (r *MongoDBRepository) Store(ctx context.Context, rec *Record) error {
	_, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.idempotency.store")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.tenant_id", rec.TenantID),
	)

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	coll := db.Collection(collectionName)

	if _, err := coll.InsertOne(ctx, rec); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to insert idempotency record", err)
		return err
	}

	return nil
}

// EnsureIndexes creates the unique and TTL indexes. Called lazily from the
// guard on first use so tests and single-tenant bootstrap do not need a
// dedicated provisioning step.
func (r *MongoDBRepository) EnsureIndexes(ctx context.Context) error {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "mongodb.idempotency.ensure_indexes")
	defer span.End()

	db, err := r.getDatabase(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get database", err)
		return err
	}

	coll := db.Collection(collectionName)

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "tenant_id", Value: 1},
				{Key: "idempotency_key", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := coll.Indexes().CreateMany(ctx, indexes); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create idempotency indexes", err)
		return err
	}

	return nil
}
