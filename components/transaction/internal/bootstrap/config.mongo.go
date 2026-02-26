// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v3/commons/mongo"
	tmmongo "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/mongo"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoComponents holds MongoDB-related components initialized during bootstrap.
type mongoComponents struct {
	connection   *libMongo.MongoConnection // nil in multi-tenant mode
	metadataRepo mongodb.Repository
	mongoManager *tmmongo.Manager // nil in single-tenant mode; exposed for PR 5 middleware
}

// initMongo initializes MongoDB components.
// Dispatches to single-tenant or multi-tenant initialization based on Options.
func initMongo(opts *Options, cfg *Config, logger libLog.Logger) (*mongoComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initMultiTenantMongo(opts, logger)
	}

	return initSingleTenantMongo(cfg, logger)
}

// initMultiTenantMongo initializes MongoDB in multi-tenant mode.
func initMultiTenantMongo(opts *Options, logger libLog.Logger) (*mongoComponents, error) {
	logger.Info("Initializing multi-tenant MongoDB for transaction")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant MongoDB initialization")
	}

	mongoMgr := tmmongo.NewManager(
		opts.TenantClient,
		opts.TenantServiceName,
		tmmongo.WithModule(ApplicationName),
		tmmongo.WithLogger(logger),
	)

	placeholderConn := &libMongo.MongoConnection{
		Logger: logger,
	}

	return &mongoComponents{
		metadataRepo: mongodb.NewMetadataMongoDBRepository(placeholderConn),
		mongoManager: mongoMgr,
	}, nil
}

// initSingleTenantMongo initializes MongoDB in single-tenant mode.
func initSingleTenantMongo(cfg *Config, logger libLog.Logger) (*mongoComponents, error) {
	mongoURI := utils.EnvFallback(cfg.PrefixedMongoURI, cfg.MongoURI)
	mongoHost := utils.EnvFallback(cfg.PrefixedMongoDBHost, cfg.MongoDBHost)
	mongoName := utils.EnvFallback(cfg.PrefixedMongoDBName, cfg.MongoDBName)
	mongoUser := utils.EnvFallback(cfg.PrefixedMongoDBUser, cfg.MongoDBUser)
	mongoPassword := utils.EnvFallback(cfg.PrefixedMongoDBPassword, cfg.MongoDBPassword)
	mongoPortRaw := utils.EnvFallback(cfg.PrefixedMongoDBPort, cfg.MongoDBPort)
	mongoParametersRaw := utils.EnvFallback(cfg.PrefixedMongoDBParameters, cfg.MongoDBParameters)
	mongoPoolSize := utils.EnvFallbackInt(cfg.PrefixedMaxPoolSize, cfg.MaxPoolSize)

	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(mongoPortRaw, mongoParametersRaw, logger)

	mongoSource := libMongo.BuildConnectionString(
		mongoURI, mongoUser, mongoPassword, mongoHost, mongoPort, mongoParameters, logger)

	var mongoMaxPoolSize uint64 = 100
	if mongoPoolSize > 0 {
		mongoMaxPoolSize = uint64(mongoPoolSize)
	}

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               mongoName,
		Logger:                 logger,
		MaxPoolSize:            mongoMaxPoolSize,
	}

	metadataRepo := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	ensureMongoIndexes(mongoConnection, logger)

	return &mongoComponents{
		connection:   mongoConnection,
		metadataRepo: metadataRepo,
	}, nil
}

// ensureMongoIndexes creates the entity_id index on known collections.
// Only called in single-tenant mode (multi-tenant indexes are managed per-tenant).
func ensureMongoIndexes(conn *libMongo.MongoConnection, logger libLog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}},
		Options: options.Index().
			SetUnique(false),
	}

	collections := []string{"operation", "transaction", "operation_route", "transaction_route"}
	for _, collection := range collections {
		if err := conn.EnsureIndexes(ctx, collection, indexModel); err != nil {
			logger.Warnf("Failed to ensure indexes for collection %s: %v", collection, err)
		}
	}
}
