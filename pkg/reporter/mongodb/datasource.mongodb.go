// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Repository defines an interface for querying data from MongoDB collections.
//
//go:generate mockgen --destination=datasource.mongodb.mock.go --package=mongodb --copyright_file=../../COPYRIGHT . Repository
type Repository interface {
	Query(ctx context.Context, collection string, fields []string, filter map[string][]any) ([]map[string]any, error)
	QueryWithAdvancedFilters(ctx context.Context, collection string, fields []string, filter map[string]model.FilterCondition) ([]map[string]any, error)
	GetDatabaseSchema(ctx context.Context) ([]CollectionSchema, error)
	GetDatabaseSchemaForOrganization(ctx context.Context, organizationID string) ([]CollectionSchema, error)
	GetDatabaseSchemaForCRM(ctx context.Context) ([]CollectionSchema, error)
	ListCollectionNames(ctx context.Context) ([]string, error)
	CloseConnection(ctx context.Context) error

	// Ping verifies connectivity with a minimal command
	// (db.runCommand({ping:1}) under the hood). Used by health checks to
	// avoid the cost of GetDatabaseSchema, which performs a full
	// collection-by-collection schema inference.
	Ping(ctx context.Context) error
}

// CollectionSchema represents the structure of a MongoDB collection.
type CollectionSchema struct {
	CollectionName string             `json:"collection_name"`
	Fields         []FieldInformation `json:"fields"`
}

// FieldInformation contains the details of a MongoDB field.
type FieldInformation struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

// ExternalDataSource provides an interface for interacting with a MongoDB database connection.
type ExternalDataSource struct {
	connection *MongoConnection
	Database   string
}

const unknownDataType = "unknown"

// Compile-time interface satisfaction check.
var _ Repository = (*ExternalDataSource)(nil)

// NewDataSourceRepository creates a new ExternalDataSource instance using the provided MongoDB connection string and database name.
func NewDataSourceRepository(mongoURI string, dbName string, logger log.Logger) (*ExternalDataSource, error) {
	mongoConnection := &MongoConnection{
		ConnectionStringSource: mongoURI,
		Database:               dbName,
		MaxPoolSize:            constant.MongoMaxPoolSizeExternal,
		Logger:                 logger,
	}

	// Use a bounded context for the eager connection check so that caller
	// deadlines are honored and we don't block indefinitely.
	initCtx, cancel := context.WithTimeout(context.Background(), constant.ConnectionTimeout)
	defer cancel()

	if _, err := mongoConnection.GetDB(initCtx); err != nil {
		logger.Log(initCtx, log.LevelError, "Failed to establish MongoDB connection", log.Err(err))
		return nil, fmt.Errorf("failed to establish MongoDB connection: %w", err)
	}

	return &ExternalDataSource{connection: mongoConnection, Database: dbName}, nil
}

// NewDataSourceRepositoryFromDatabase builds an ExternalDataSource over an
// already-resolved per-tenant *mongo.Database. It opens no connection and owns
// no pool: the database (and its client) are owned by the caller — in
// multi-tenant mode the lib-commons tenant manager — so CloseConnection here is
// a no-op (see CloseConnection). It exists so the manager's schema-discovery
// methods (GetDatabaseSchema, GetDatabaseSchemaForOrganization,
// GetDatabaseSchemaForCRM) run unchanged against a tenant-scoped database
// instead of the env-configured pool.
//
// The Database name is read off the resolved handle so collection lookups
// target the same database the tenant manager resolved. A nil database is
// rejected: a nil handle stored on the connection would nil-deref on the first
// schema read, defeating the fail-closed contract at the resolution seam.
func NewDataSourceRepositoryFromDatabase(db *mongo.Database, logger log.Logger) (*ExternalDataSource, error) {
	if db == nil {
		return nil, fmt.Errorf("mongodb database must not be nil")
	}

	dbName := db.Name()

	return &ExternalDataSource{
		connection: &MongoConnection{
			Database: dbName,
			Logger:   logger,
			DB:       db.Client(),
		},
		Database: dbName,
	}, nil
}

// Ping verifies connectivity using *mongo.Client.Ping, which issues a
// db.runCommand({ping:1}) — the canonical lightweight reachability probe
// for MongoDB. Replaces the previous GetDatabaseSchema-as-ping
// implementation that ran a per-collection field discovery on every
// health-check pass (~5s per datasource per cycle in production).
//
// Returns an error (rather than panicking) when the receiver or the
// connection wrapper are nil. The HealthChecker relies on this nil-safety
// contract to demote a transiently-disconnected datasource to Unavailable
// instead of crashing the worker process.
func (ds *ExternalDataSource) Ping(ctx context.Context) error {
	if ds == nil || ds.connection == nil {
		return fmt.Errorf("mongodb connection not initialized")
	}

	if ds.connection.DB == nil {
		return fmt.Errorf("mongodb connection not initialized")
	}

	return ds.connection.DB.Ping(ctx, nil)
}

// GetDatabase returns the *mongo.Database for this datasource, reusing the
// pooled client established at construction. The embedded extraction engine's
// single-tenant resolver uses it to hand the connector a live, host-owned
// database handle rather than opening a second client. It returns an error when
// the connection is not initialized.
func (ds *ExternalDataSource) GetDatabase(ctx context.Context) (*mongo.Database, error) {
	if ds == nil || ds.connection == nil {
		return nil, fmt.Errorf("mongodb connection not initialized")
	}

	client, err := ds.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	return client.Database(ds.Database), nil
}

// CloseConnection closes the connection with MongoDB.
func (ds *ExternalDataSource) CloseConnection(ctx context.Context) error {
	if ds.connection.DB != nil {
		ds.connection.Logger.Log(context.Background(), log.LevelInfo, "Closing MongoDB connection...")

		if err := ds.connection.Close(); err != nil {
			ds.connection.Logger.Log(context.Background(), log.LevelError, "Error closing MongoDB connection", log.Err(err))
			return err
		}

		ds.connection.Logger.Log(context.Background(), log.LevelInfo, "MongoDB connection closed successfully.")
	}

	return nil
}

// convertBsonToMap converts a bson.M to map[string]any recursively.
func convertBsonToMap(bsonDoc bson.M) map[string]any {
	result := make(map[string]any)
	for k, v := range bsonDoc {
		result[k] = convertBsonValue(v)
	}

	return result
}

// convertBsonValue converts a BSON value to its Go equivalent recursively.
func convertBsonValue(value any) any {
	switch v := value.(type) {
	case bson.M:
		return convertBsonToMap(v)
	case bson.A:
		result := make([]any, len(v))
		for i, elem := range v {
			result[i] = convertBsonValue(elem)
		}

		return result
	case bson.D:
		doc := make(map[string]any)
		for _, elem := range v {
			doc[elem.Key] = convertBsonValue(elem.Value)
		}

		return doc
	case bson.DateTime:
		return v.Time()
	case bson.ObjectID:
		return v.Hex()
	case bson.Binary:
		if len(v.Data) == constant.MongoUUIDByteLength {
			u, err := uuid.FromBytes(v.Data)
			if err == nil {
				return u.String()
			}
		}

		return hex.EncodeToString(v.Data)
	case nil:
		return nil
	default:
		return v
	}
}
