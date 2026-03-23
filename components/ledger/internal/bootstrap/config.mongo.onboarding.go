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
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// onboardingMongoComponents holds MongoDB-related components for the onboarding domain.
type onboardingMongoComponents struct {
	connection   *libMongo.Client // nil in multi-tenant mode
	metadataRepo mongodb.Repository
	mongoManager *tmmongo.Manager // nil in single-tenant mode; exposed for middleware wiring
}

// initOnboardingMongo initializes MongoDB components for the onboarding domain.
// Dispatches to single-tenant or multi-tenant initialization based on Options.
func initOnboardingMongo(opts *Options, cfg *Config, logger libLog.Logger) (*onboardingMongoComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initOnboardingMultiTenantMongo(opts, cfg, logger)
	}

	return initOnboardingSingleTenantMongo(cfg, logger)
}

// initOnboardingMultiTenantMongo initializes MongoDB in multi-tenant mode for onboarding.
func initOnboardingMultiTenantMongo(opts *Options, cfg *Config, logger libLog.Logger) (*onboardingMongoComponents, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant MongoDB for onboarding")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant MongoDB initialization")
	}

	mongoOpts := []tmmongo.Option{
		tmmongo.WithModule("onboarding"),
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

	return &onboardingMongoComponents{
		metadataRepo: mongodb.NewMetadataMongoDBRepository(nil),
		mongoManager: mongoMgr,
	}, nil
}

// initOnboardingSingleTenantMongo initializes MongoDB in single-tenant mode for onboarding.
func initOnboardingSingleTenantMongo(cfg *Config, logger libLog.Logger) (*onboardingMongoComponents, error) {
	mongoURI := utils.EnvFallback(cfg.OnbPrefixedMongoURI, cfg.MongoURI)
	mongoHost := utils.EnvFallback(cfg.OnbPrefixedMongoDBHost, cfg.MongoDBHost)
	mongoName := utils.EnvFallback(cfg.OnbPrefixedMongoDBName, cfg.MongoDBName)
	mongoUser := utils.EnvFallback(cfg.OnbPrefixedMongoDBUser, cfg.MongoDBUser)
	mongoPassword := utils.EnvFallback(cfg.OnbPrefixedMongoDBPassword, cfg.MongoDBPassword)
	mongoPortRaw := utils.EnvFallback(cfg.OnbPrefixedMongoDBPort, cfg.MongoDBPort)
	mongoParametersRaw := utils.EnvFallback(cfg.OnbPrefixedMongoDBParameters, cfg.MongoDBParameters)
	mongoPoolSize := utils.EnvFallbackInt(cfg.OnbPrefixedMaxPoolSize, cfg.MaxPoolSize)

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
	ensureOnboardingMongoIndexes(mongoConnection, logger)

	return &onboardingMongoComponents{
		connection:   mongoConnection,
		metadataRepo: metadataRepo,
	}, nil
}

// ensureOnboardingMongoIndexes creates the entity_id index on known onboarding collections.
// Only called in single-tenant mode (multi-tenant indexes are managed per-tenant).
func ensureOnboardingMongoIndexes(conn *libMongo.Client, logger libLog.Logger) {
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
