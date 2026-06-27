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
	"github.com/LerianStudio/lib-observability/metrics"
	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	mongoAudit "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	crmservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgMongo "github.com/LerianStudio/midaz/v4/pkg/mongo"
)

// crmComponents holds the CRM (holder/instrument) components wired into the unified
// ledger binary. connection is non-nil only in single-tenant mode; mongoManager is
// non-nil only in multi-tenant mode. encryption carries the wired field-encryption
// surface (FieldEncryptor injected into the repos, plus the envelope-only services
// consumed by the encryption/audit HTTP handlers and readyz).
type crmComponents struct {
	connection        *libMongo.Client // nil in multi-tenant mode
	encryption        *crmEncryption
	holderHandler     *httpin.HolderHandler
	instrumentHandler *httpin.InstrumentHandler
	encryptionHandler *httpin.EncryptionHandler // nil in legacy mode
	auditHandler      *httpin.AuditHandler      // nil in legacy mode
	mongoManager      *tmmongo.Manager          // nil in single-tenant mode; exposed for middleware/eviction wiring
}

// initCRM initializes the CRM holder/instrument slice of the unified binary.
// Dispatches to single-tenant or multi-tenant initialization based on Options,
// mirroring initOnboardingMongo.
//
// metricsFactory may be nil at this stage of bootstrap (telemetry is wired later);
// the protection-metrics seam threaded into the encryption services is nil-safe and
// degrades to a no-op emitter.
func initCRM(opts *Options, cfg *Config, metricsFactory *metrics.MetricsFactory, logger libLog.Logger) (*crmComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initCRMMultiTenant(opts, cfg, metricsFactory, logger)
	}

	return initCRMSingleTenant(cfg, metricsFactory, logger)
}

// initCRMMultiTenant builds a 3rd tenant-manager Mongo manager (module crm-api)
// reusing ledger's tenant client. Repos are built against a nil connection: the
// per-request tenant context provides the database via tmcore.GetMBContext.
func initCRMMultiTenant(opts *Options, cfg *Config, metricsFactory *metrics.MetricsFactory, logger libLog.Logger) (*crmComponents, error) {
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

	crmEnc, err := initCRMEncryption(context.Background(), cfg, nil, metricsFactory, logger)
	if err != nil {
		return nil, err
	}

	holderRepo, instrumentRepo, err := buildCRMRepositories(nil, crmEnc.fieldEncryptor)
	if err != nil {
		return nil, err
	}

	holderHandler, instrumentHandler := buildCRMHandlers(holderRepo, instrumentRepo)

	return &crmComponents{
		encryption:        crmEnc,
		holderHandler:     holderHandler,
		instrumentHandler: instrumentHandler,
		encryptionHandler: newEncryptionHandler(crmEnc.provisioningService),
		auditHandler:      newAuditHandler(crmEnc.auditRepo),
		mongoManager:      mongoMgr,
	}, nil
}

// initCRMSingleTenant builds a static Mongo client from the CrmPrefixed* config
// and wires the holder/instrument repos, use cases and handlers against it.
func initCRMSingleTenant(cfg *Config, metricsFactory *metrics.MetricsFactory, logger libLog.Logger) (*crmComponents, error) {
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

	crmEnc, err := initCRMEncryption(context.Background(), cfg, mongoConnection, metricsFactory, logger)
	if err != nil {
		return nil, err
	}

	holderRepo, instrumentRepo, err := buildCRMRepositories(mongoConnection, crmEnc.fieldEncryptor)
	if err != nil {
		return nil, err
	}

	holderHandler, instrumentHandler := buildCRMHandlers(holderRepo, instrumentRepo)

	return &crmComponents{
		connection:        mongoConnection,
		encryption:        crmEnc,
		holderHandler:     holderHandler,
		instrumentHandler: instrumentHandler,
		encryptionHandler: newEncryptionHandler(crmEnc.provisioningService),
		auditHandler:      newAuditHandler(crmEnc.auditRepo),
	}, nil
}

// newEncryptionHandler builds the encryption provisioning HTTP handler when a
// provisioning service is available (envelope mode). In legacy mode the service is
// nil, so this returns nil and the routes stay unregistered.
func newEncryptionHandler(provisioningService encryption.ProvisioningService) *httpin.EncryptionHandler {
	if provisioningService == nil {
		return nil
	}

	return &httpin.EncryptionHandler{ProvisioningService: provisioningService}
}

// newAuditHandler builds the protection-audit HTTP handler when an audit repository
// is available (envelope mode). In legacy mode the repository is nil, so this
// returns nil and the route stays unregistered.
func newAuditHandler(auditRepo mongoAudit.Repository) *httpin.AuditHandler {
	if auditRepo == nil {
		return nil
	}

	return &httpin.AuditHandler{Service: encryption.NewAuditQueryService(auditRepo)}
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

// buildCRMRepositories constructs the holder/instrument Mongo repositories backed
// by the shared FieldEncryptor. connection is nil in multi-tenant mode (database
// resolved per-request).
func buildCRMRepositories(connection *libMongo.Client, fieldEncryptor encryption.FieldEncryptor) (*holder.MongoDBRepository, *instrument.MongoDBRepository, error) {
	holderRepo, err := holder.NewMongoDBRepository(connection, fieldEncryptor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize CRM holder repository: %w", err)
	}

	instrumentRepo, err := instrument.NewMongoDBRepository(connection, fieldEncryptor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize CRM instrument repository: %w", err)
	}

	return holderRepo, instrumentRepo, nil
}

// buildCRMHandlers assembles the CRM use cases and HTTP handlers.
func buildCRMHandlers(holderRepo *holder.MongoDBRepository, instrumentRepo *instrument.MongoDBRepository) (*httpin.HolderHandler, *httpin.InstrumentHandler) {
	useCases := &crmservices.UseCase{
		HolderRepo:     holderRepo,
		InstrumentRepo: instrumentRepo,
	}

	return &httpin.HolderHandler{Service: useCases}, &httpin.InstrumentHandler{Service: useCases}
}
