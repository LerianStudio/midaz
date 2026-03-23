// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/adapters/mongodb/onboarding"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoComponents holds MongoDB-related components initialized during bootstrap.
type mongoComponents struct {
	connection   *libMongo.Client // nil in multi-tenant mode
	metadataRepo mongodb.Repository
	mongoManager *tmmongo.Manager // nil in single-tenant mode; exposed for PR 5 middleware
}

// initMongo initializes MongoDB components.
// Dispatches to single-tenant or multi-tenant initialization based on Options.
func initMongo(opts *Options, cfg *Config, logger libLog.Logger) (*mongoComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initMultiTenantMongo(opts, cfg, logger)
	}

	return initSingleTenantMongo(cfg, logger)
}

// initMultiTenantMongo initializes MongoDB in multi-tenant mode.
// Creates a tmmongo.Manager that resolves per-tenant databases via Tenant Manager.
// The Manager is NOT injected into the repository — the middleware (PR 5) uses it
// to inject *mongo.Database into context, and the repository reads from context.
func initMultiTenantMongo(opts *Options, cfg *Config, logger libLog.Logger) (*mongoComponents, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant MongoDB for onboarding")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant MongoDB initialization")
	}

	mongoOpts := []tmmongo.Option{
		tmmongo.WithModule(ApplicationName),
		tmmongo.WithLogger(logger),
	}

	if cfg.MultiTenantSettingsCheckIntervalSec > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithSettingsCheckInterval(
			time.Duration(cfg.MultiTenantSettingsCheckIntervalSec)*time.Second,
		))
	}

	mongoMgr := tmmongo.NewManager(
		opts.TenantClient,
		opts.TenantServiceName,
		mongoOpts...,
	)

	return &mongoComponents{
		metadataRepo: mongodb.NewMetadataMongoDBRepository(nil),
		mongoManager: mongoMgr,
	}, nil
}

// initSingleTenantMongo initializes MongoDB in single-tenant mode.
// Uses the existing static MongoConnection with env-var-based configuration.
func initSingleTenantMongo(cfg *Config, logger libLog.Logger) (*mongoComponents, error) {
	mongoURI := envFallback(cfg.PrefixedMongoURI, cfg.MongoURI)
	mongoHost := envFallback(cfg.PrefixedMongoDBHost, cfg.MongoDBHost)
	mongoName := envFallback(cfg.PrefixedMongoDBName, cfg.MongoDBName)
	mongoUser := envFallback(cfg.PrefixedMongoDBUser, cfg.MongoDBUser)
	mongoPassword := envFallback(cfg.PrefixedMongoDBPassword, cfg.MongoDBPassword)
	mongoPortRaw := envFallback(cfg.PrefixedMongoDBPort, cfg.MongoDBPort)
	mongoParametersRaw := envFallback(cfg.PrefixedMongoDBParameters, cfg.MongoDBParameters)
	mongoPoolSize := envFallbackInt(cfg.PrefixedMaxPoolSize, cfg.MaxPoolSize)

	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(mongoPortRaw, mongoParametersRaw, logger)

	mongoQuery, err := url.ParseQuery(mongoParameters)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MongoDB query parameters: %w", err)
	}

	mongoSource, err := libMongo.BuildURI(libMongo.URIConfig{
		Scheme:   mongoURI,
		Username: mongoUser,
		Password: mongoPassword,
		Host:     mongoHost,
		Port:     mongoPort,
		Database: mongoName,
		Query:    mongoQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build MongoDB URI: %w", err)
	}

	var mongoMaxPoolSize uint64 = 100
	if mongoPoolSize > 0 {
		mongoMaxPoolSize = uint64(mongoPoolSize)
	}

	mongoConnection, err := libMongo.NewClient(context.Background(), libMongo.Config{
		URI:         mongoSource,
		Database:    mongoName,
		Logger:      logger,
		MaxPoolSize: mongoMaxPoolSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MongoDB client: %w", err)
	}

	metadataRepo := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	// Ensure indexes for known collections (only in single-tenant mode)
	ensureMongoIndexes(mongoConnection, logger)

	return &mongoComponents{
		connection:   mongoConnection,
		metadataRepo: metadataRepo,
	}, nil
}

// ensureMongoIndexes creates the entity_id index on known collections.
// Only called in single-tenant mode (multi-tenant indexes are managed per-tenant).
func ensureMongoIndexes(conn *libMongo.Client, logger libLog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}},
		Options: options.Index().
			SetUnique(false),
	}

	collections := []string{"organization", "ledger", "segment", "account", "portfolio", "asset", "account_type"}
	for _, collection := range collections {
		if err := conn.EnsureIndexes(ctx, collection, indexModel); err != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to ensure indexes for collection %s: %v", collection, err))
		}
	}
}
