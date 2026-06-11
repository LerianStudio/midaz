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

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	libLog "github.com/LerianStudio/lib-observability/log"
	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	crmservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgMongo "github.com/LerianStudio/midaz/v4/pkg/mongo"
)

// crmComponents holds the CRM (holder/alias) components wired into the unified
// ledger binary. connection and cipher are nil/zero-meaningful only in
// single-tenant mode; mongoManager is non-nil only in multi-tenant mode.
type crmComponents struct {
	connection        *libMongo.Client  // nil in multi-tenant mode
	cipher            *libCrypto.Crypto // holder/alias PII encryption (R7)
	holderHandler     *httpin.HolderHandler
	instrumentHandler *httpin.InstrumentHandler
	mongoManager      *tmmongo.Manager // nil in single-tenant mode; exposed for middleware/eviction wiring
}

// initCRM initializes the CRM holder/alias slice of the unified binary.
// Dispatches to single-tenant or multi-tenant initialization based on Options,
// mirroring initOnboardingMongo. The cipher is initialized from the carried
// LCRYPTO_* values in BOTH modes so existing holder/alias PII decrypts (R7).
func initCRM(opts *Options, cfg *Config, logger libLog.Logger) (*crmComponents, error) {
	cipher := &libCrypto.Crypto{
		HashSecretKey:    cfg.CrmHashSecretKey,
		EncryptSecretKey: cfg.CrmEncryptSecretKey,
		Logger:           logger,
	}

	if err := cipher.InitializeCipher(); err != nil {
		return nil, fmt.Errorf("failed to initialize CRM cipher: %w", err)
	}

	if opts != nil && opts.MultiTenantEnabled {
		return initCRMMultiTenant(opts, cfg, logger, cipher)
	}

	return initCRMSingleTenant(cfg, logger, cipher)
}

// initCRMMultiTenant builds a 3rd tenant-manager Mongo manager (module crm-api)
// reusing ledger's tenant client. Repos are built against a nil connection: the
// per-request tenant context provides the database via tmcore.GetMBContext.
func initCRMMultiTenant(opts *Options, cfg *Config, logger libLog.Logger, cipher *libCrypto.Crypto) (*crmComponents, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant MongoDB for CRM")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant CRM MongoDB initialization")
	}

	mongoOpts := []tmmongo.Option{
		tmmongo.WithModule(constant.ModuleCRM),
		tmmongo.WithLogger(logger),
	}

	if cfg.MultiTenantConnectionsCheckIntervalSec > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithConnectionsCheckInterval(
			time.Duration(cfg.MultiTenantConnectionsCheckIntervalSec)*time.Second,
		))
	}

	mongoMgr := tmmongo.NewManager(opts.TenantClient, opts.TenantServiceName, mongoOpts...)

	holderRepo, aliasRepo, err := buildCRMRepositories(nil, cipher)
	if err != nil {
		return nil, err
	}

	holderHandler, instrumentHandler := buildCRMHandlers(holderRepo, aliasRepo)

	return &crmComponents{
		cipher:            cipher,
		holderHandler:     holderHandler,
		instrumentHandler: instrumentHandler,
		mongoManager:      mongoMgr,
	}, nil
}

// initCRMSingleTenant builds a static Mongo client from the CrmPrefixed* config
// and wires the holder/alias repos, use cases and handlers against it.
func initCRMSingleTenant(cfg *Config, logger libLog.Logger, cipher *libCrypto.Crypto) (*crmComponents, error) {
	mongoSource, err := resolveCRMMongoURI(cfg, logger)
	if err != nil {
		return nil, err
	}

	var mongoMaxPoolSize uint64 = 100
	if cfg.CrmPrefixedMaxPoolSize > 0 {
		mongoMaxPoolSize = uint64(cfg.CrmPrefixedMaxPoolSize)
	}

	var tlsCfg *libMongo.TLSConfig
	if caCert := strings.TrimSpace(cfg.CrmPrefixedMongoTLSCACert); caCert != "" {
		tlsCfg = &libMongo.TLSConfig{CACertBase64: caCert}
	}

	mongoConnection, err := libMongo.NewClient(context.Background(), libMongo.Config{
		URI:         mongoSource,
		Database:    cfg.CrmPrefixedMongoDBName,
		Logger:      logger,
		MaxPoolSize: mongoMaxPoolSize,
		TLS:         tlsCfg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create CRM MongoDB client: %w", err)
	}

	holderRepo, aliasRepo, err := buildCRMRepositories(mongoConnection, cipher)
	if err != nil {
		return nil, err
	}

	holderHandler, instrumentHandler := buildCRMHandlers(holderRepo, aliasRepo)

	return &crmComponents{
		connection:        mongoConnection,
		cipher:            cipher,
		holderHandler:     holderHandler,
		instrumentHandler: instrumentHandler,
	}, nil
}

// resolveCRMMongoURI builds the CRM Mongo connection string. It accepts either
// a full URI (MONGO_CRM_URI containing "://", used verbatim) or a scheme value
// ("mongodb"/"mongodb+srv"/empty) assembled from the discrete host/port/params,
// preserving the flexibility the standalone CRM service had.
func resolveCRMMongoURI(cfg *Config, logger libLog.Logger) (string, error) {
	rawURI := strings.TrimSpace(cfg.CrmPrefixedMongoURI)
	if strings.Contains(rawURI, "://") {
		return rawURI, nil
	}

	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(cfg.CrmPrefixedMongoDBPort, cfg.CrmPrefixedMongoDBParameters, logger)

	mongoQuery, err := url.ParseQuery(mongoParameters)
	if err != nil {
		return "", fmt.Errorf("failed to parse CRM MongoDB query parameters: %w", err)
	}

	scheme := rawURI
	if scheme == "" {
		scheme = "mongodb"
	}

	mongoSource, err := libMongo.BuildURI(libMongo.URIConfig{
		Scheme:   scheme,
		Username: cfg.CrmPrefixedMongoDBUser,
		Password: cfg.CrmPrefixedMongoDBPassword,
		Host:     cfg.CrmPrefixedMongoDBHost,
		Port:     mongoPort,
		Database: cfg.CrmPrefixedMongoDBName,
		Query:    mongoQuery,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build CRM MongoDB URI: %w", err)
	}

	return mongoSource, nil
}

// buildCRMRepositories constructs the holder/alias Mongo repositories.
// connection is nil in multi-tenant mode (database resolved per-request).
func buildCRMRepositories(connection *libMongo.Client, cipher *libCrypto.Crypto) (*holder.MongoDBRepository, *instrument.MongoDBRepository, error) {
	holderRepo, err := holder.NewMongoDBRepository(connection, cipher)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize CRM holder repository: %w", err)
	}

	aliasRepo, err := instrument.NewMongoDBRepository(connection, cipher)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize CRM alias repository: %w", err)
	}

	return holderRepo, aliasRepo, nil
}

// buildCRMHandlers assembles the CRM use cases and HTTP handlers.
func buildCRMHandlers(holderRepo *holder.MongoDBRepository, aliasRepo *instrument.MongoDBRepository) (*httpin.HolderHandler, *httpin.InstrumentHandler) {
	useCases := &crmservices.UseCase{
		HolderRepo:     holderRepo,
		InstrumentRepo: aliasRepo,
	}

	return &httpin.HolderHandler{Service: useCases}, &httpin.InstrumentHandler{Service: useCases}
}
