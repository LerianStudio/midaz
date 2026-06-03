// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	libLog "github.com/LerianStudio/lib-observability/log"
	feesmongo "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
)

// feesMongoComponents holds the fee/billing-package Mongo slice of the unified
// ledger binary. connection is the static fee Mongo connection wrapper used for
// index-ensure and as the single-tenant data source; in multi-tenant mode the
// per-request tenant DB is resolved via tmcore.GetMBContext and mongoManager is
// non-nil for tenant-manager DB resolution + eviction wiring.
//
// Unlike CRM, the fee repos have no nil-connection MT path: their constructors
// call GetDB + EnsureIndexes eagerly, so the static connection MUST point at a
// reachable Mongo even in MT mode (mirroring the standalone fees service, which
// always built one MongoConnection and overrode it per-request via context).
type feesMongoComponents struct {
	connection         *feesmongo.MongoConnection
	packageRepo        pack.Repository
	billingPackageRepo billing_package.Repository
	mongoManager       *tmmongo.Manager // nil in single-tenant mode
}

// initFeesMongo initializes the fee/billing-package Mongo slice. It builds a
// static fee Mongo connection from the FeesPrefixed* config, constructs the
// pack + billing_package repositories (whose constructors ensure the 11 compound
// indexes on startup), and — in multi-tenant mode — additionally builds a fee
// tenant-manager Mongo manager keyed on constant.ModuleFees for per-request DB
// resolution.
func initFeesMongo(opts *Options, cfg *Config, logger libLog.Logger) (*feesMongoComponents, error) {
	connection, err := buildFeesMongoConnection(cfg, logger)
	if err != nil {
		return nil, err
	}

	// Constructing the repos validates the connection (GetDB) and ensures the
	// compound indexes (pack=7, billing_package=4) on the static connection's DB.
	packageRepo, err := pack.NewPackageMongoDBRepository(connection, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize fee package repository: %w", err)
	}

	billingPackageRepo, err := billing_package.NewBillingPackageMongoDBRepository(connection, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize billing package repository: %w", err)
	}

	components := &feesMongoComponents{
		connection:         connection,
		packageRepo:        packageRepo,
		billingPackageRepo: billingPackageRepo,
	}

	if opts != nil && opts.MultiTenantEnabled {
		mongoMgr, mgrErr := buildFeesMongoManager(opts, cfg, logger)
		if mgrErr != nil {
			return nil, mgrErr
		}

		components.mongoManager = mongoMgr
	}

	return components, nil
}

// buildFeesMongoManager builds the fee tenant-manager Mongo manager. It reuses
// ledger's tenant client and is keyed on constant.ModuleFees ("plugin-fees") so
// tenant-manager resolves the fee module DB for the merged binary. The per-tenant
// DB lands on the request context (tmcore) via the CRM-style route-scoped
// middleware in buildUnifiedRouteSetup.
func buildFeesMongoManager(opts *Options, cfg *Config, logger libLog.Logger) (*tmmongo.Manager, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant MongoDB for fees")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant fees MongoDB initialization")
	}

	mongoOpts := []tmmongo.Option{
		tmmongo.WithModule(constant.ModuleFees),
		tmmongo.WithLogger(logger),
	}

	if cfg.MultiTenantConnectionsCheckIntervalSec > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithConnectionsCheckInterval(
			time.Duration(cfg.MultiTenantConnectionsCheckIntervalSec)*time.Second,
		))
	}

	return tmmongo.NewManager(opts.TenantClient, opts.TenantServiceName, mongoOpts...), nil
}

// buildFeesMongoConnection builds the static fee Mongo connection wrapper from
// the FeesPrefixed* config. The repos lazily dial it via GetDB.
func buildFeesMongoConnection(cfg *Config, logger libLog.Logger) (*feesmongo.MongoConnection, error) {
	mongoSource, err := resolveFeesMongoURI(cfg, logger)
	if err != nil {
		return nil, err
	}

	var mongoMaxPoolSize uint64 = 100
	if cfg.FeesPrefixedMaxPoolSize > 0 {
		mongoMaxPoolSize = uint64(cfg.FeesPrefixedMaxPoolSize)
	}

	return &feesmongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.FeesPrefixedMongoDBName,
		Logger:                 logger,
		MaxPoolSize:            mongoMaxPoolSize,
		TLSCACert:              strings.TrimSpace(cfg.FeesPrefixedMongoTLSCACert),
	}, nil
}

// resolveFeesMongoURI builds the fee Mongo connection string. It accepts either
// a full URI (MONGO_FEES_URI containing "://", used verbatim) or a scheme value
// ("mongodb"/"mongodb+srv"/empty) assembled from the discrete host/port/params,
// mirroring resolveCRMMongoURI.
func resolveFeesMongoURI(cfg *Config, logger libLog.Logger) (string, error) {
	rawURI := strings.TrimSpace(cfg.FeesPrefixedMongoURI)
	if strings.Contains(rawURI, "://") {
		return rawURI, nil
	}

	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(cfg.FeesPrefixedMongoDBPort, cfg.FeesPrefixedMongoDBParameters, logger)

	mongoQuery, err := url.ParseQuery(mongoParameters)
	if err != nil {
		return "", fmt.Errorf("failed to parse fees MongoDB query parameters: %w", err)
	}

	scheme := rawURI
	if scheme == "" {
		scheme = "mongodb"
	}

	mongoSource, err := libMongo.BuildURI(libMongo.URIConfig{
		Scheme:   scheme,
		Username: cfg.FeesPrefixedMongoDBUser,
		Password: cfg.FeesPrefixedMongoDBPassword,
		Host:     cfg.FeesPrefixedMongoDBHost,
		Port:     mongoPort,
		Database: cfg.FeesPrefixedMongoDBName,
		Query:    mongoQuery,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build fees MongoDB URI: %w", err)
	}

	return mongoSource, nil
}
