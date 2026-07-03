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

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libZap "github.com/LerianStudio/lib-observability/zap"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
)

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName                                string `env:"ENV_NAME"`
	ServerAddress                          string `env:"SERVER_ADDRESS"`
	LogLevel                               string `env:"LOG_LEVEL"`
	OtelServiceName                        string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName                        string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion                     string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv                      string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint                string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry                        bool   `env:"ENABLE_TELEMETRY"`
	MongoURI                               string `env:"MONGO_URI"`
	MongoDBHost                            string `env:"MONGO_HOST"`
	MongoDBName                            string `env:"MONGO_NAME"`
	MongoDBUser                            string `env:"MONGO_USER"`
	MongoDBPassword                        string `env:"MONGO_PASSWORD"`
	MongoDBPort                            string `env:"MONGO_PORT"`
	MongoDBParameters                      string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                            int    `env:"MONGO_MAX_POOL_SIZE"`
	MongoTLSCACert                         string `env:"MONGO_TLS_CA_CERT"`
	HashSecretKey                          string `env:"LCRYPTO_HASH_SECRET_KEY"`
	EncryptSecretKey                       string `env:"LCRYPTO_ENCRYPT_SECRET_KEY"`
	AuthAddress                            string `env:"PLUGIN_AUTH_ADDRESS"`
	AuthEnabled                            bool   `env:"PLUGIN_AUTH_ENABLED"`
	MultiTenantEnabled                     bool   `env:"MULTI_TENANT_ENABLED"`
	MultiTenantURL                         string `env:"MULTI_TENANT_URL"`
	MultiTenantTimeout                     int    `env:"MULTI_TENANT_TIMEOUT"`                     // seconds (HTTP client timeout)
	MultiTenantIdleTimeoutSec              int    `env:"MULTI_TENANT_IDLE_TIMEOUT_SEC"`            // seconds before idle connection eviction
	MultiTenantMaxTenantPools              int    `env:"MULTI_TENANT_MAX_TENANT_POOLS"`            // max concurrent tenant pools
	MultiTenantCircuitBreakerThreshold     int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD"`   // failures before circuit opens
	MultiTenantCircuitBreakerTimeoutSec    int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC"` // seconds before circuit resets
	MultiTenantServiceAPIKey               string `env:"MULTI_TENANT_SERVICE_API_KEY"`
	MultiTenantConnectionsCheckIntervalSec int    `env:"MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC"` // seconds between tenant config revalidation checks
	MultiTenantCacheTTLSec                 int    `env:"MULTI_TENANT_CACHE_TTL_SEC" default:"120"`    // seconds for tenant config cache TTL (0 = disabled)
	MultiTenantRedisHost                   string `env:"MULTI_TENANT_REDIS_HOST"`
	MultiTenantRedisPort                   string `env:"MULTI_TENANT_REDIS_PORT"`
	MultiTenantRedisPassword               string `env:"MULTI_TENANT_REDIS_PASSWORD"`
	MultiTenantRedisTLS                    bool   `env:"MULTI_TENANT_REDIS_TLS"`
	ApplicationName                        string `env:"APPLICATION_NAME"`
	DeploymentMode                         string `env:"DEPLOYMENT_MODE"`
	Version                                string `env:"VERSION"`
	KMSVendor                              string `env:"KMS_VENDOR"`
	VaultAddr                              string `env:"KMS_VAULT_ADDR"`
	VaultRoleID                            string `env:"KMS_VAULT_ROLE_ID"`
	VaultSecretID                          string `env:"KMS_VAULT_SECRET_ID"`
	VaultAuthMethod                        string `env:"KMS_VAULT_AUTH_METHOD"`

	// --- Streaming (lib-streaming producer) ---
	// Default for all streaming knobs is OFF — a service with
	// STREAMING_ENABLED=false (or unset) injects a NoopEmitter and never
	// initialises the underlying transport. The pilot ships disabled-by-
	// default so existing deployments are not broken by the new dependency.
	// STREAMING_BROKERS, STREAMING_CLIENT_ID, STREAMING_CLOUDEVENTS_SOURCE,
	// STREAMING_COMPRESSION, STREAMING_REQUIRED_ACKS and STREAMING_BATCH_LINGER_MS
	// are consumed by libStreaming.LoadConfig() inside BuildStreamingEmitter, not
	// from this struct — so they have no field here.
	StreamingEnabled bool `env:"STREAMING_ENABLED"`

	// --- Streaming SASL/TLS auth ---
	// When STREAMING_SASL_MECHANISM is empty (default) the producer connects
	// without authentication, matching the existing behaviour for local/dev
	// brokers. When set, the value must be one of PLAIN, SCRAM-SHA-256,
	// SCRAM-SHA-512 (case-insensitive); USERNAME and PASSWORD are then
	// required and BuildStreamingEmitter wires the matching franz-go
	// sasl.Mechanism into the lib-streaming Builder.
	//
	// SASL without TLS is rejected by lib-streaming with
	// ErrPlaintextSASLNotAllowed. STREAMING_ALLOW_PLAINTEXT_SASL=true is the
	// explicit unsafe opt-in for local/dev brokers that do not terminate
	// TLS. It must NOT be set in production: SASL credentials cross the
	// network in cleartext.
	StreamingSASLMechanism      string `env:"STREAMING_SASL_MECHANISM"`
	StreamingSASLUsername       string `env:"STREAMING_SASL_USERNAME"`
	StreamingSASLPassword       string `env:"STREAMING_SASL_PASSWORD"`
	StreamingAllowPlaintextSASL bool   `env:"STREAMING_ALLOW_PLAINTEXT_SASL"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	Logger libLog.Logger
}

// InitServers initiates the HTTP server.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initializes the CRM service with optional dependency injection.
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	var logger libLog.Logger
	if opts != nil && opts.Logger != nil {
		logger = opts.Logger
	} else {
		var err error

		logger, err = libZap.New(libZap.Config{
			Environment:     resolveLoggerEnvironment(cfg.EnvName),
			Level:           cfg.LogLevel,
			OTelLibraryName: cfg.OtelLibraryName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize logger: %w", err)
		}
	}

	if cfg.MultiTenantEnabled {
		logger.Log(context.Background(), libLog.LevelInfo, "Multi-tenant mode ENABLED")
	} else {
		logger.Log(context.Background(), libLog.LevelInfo, "Running in SINGLE-TENANT MODE")
	}

	if cfg.MultiTenantEnabled && !cfg.AuthEnabled {
		return nil, fmt.Errorf("MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true")
	}

	// Build service discovery before the streaming emitter so a misconfigured
	// SD (fail-fast) returns before anything requiring drain is constructed.
	serviceDiscovery, serviceDiscoveryEnabled, err := buildServiceDiscovery(logger)
	if err != nil {
		return nil, err
	}

	// Parse the advertised port once so a malformed SERVER_ADDRESS fails fast at
	// wiring time rather than when the discovery runnable tries to register.
	serverPort, err := parseServerPort(cfg.ServerAddress)
	if err != nil {
		return nil, err
	}

	serviceDescriptor := buildCRMServiceDescriptor(serverPort)

	// Initialize KMS (encryption mode and optional Vault client).
	// Bound the KMS init (which may perform a Vault Login) so a hung Vault
	// cannot block startup indefinitely.
	kmsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	kms, err := initKMS(kmsCtx, cfg, logger)
	if err != nil {
		return nil, err
	}

	// Init Open telemetry to control logs and flows
	telemetry, err := libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// Register telemetry providers as process-global so that the otelzap bridge
	// (installed in the logger core) can forward log records to the OTLP exporter.
	if err := telemetry.ApplyGlobals(); err != nil {
		return nil, fmt.Errorf("failed to apply telemetry globals: %w", err)
	}

	// Extract the metrics factory ONCE (nil when telemetry disabled) and reuse it
	// for both encryption metrics and readyz metrics emission.
	var metricsFactory *metrics.MetricsFactory
	if telemetry != nil {
		metricsFactory = telemetry.MetricsFactory
	}

	mongoConnection, err := initMongoConnection(cfg, logger)
	if err != nil {
		return nil, err
	}

	// Create legacy crypto based on KMS mode.
	legacyCrypto, err := initLegacyCrypto(cfg, kms, logger)
	if err != nil {
		return nil, err
	}

	// Initialize encryption repositories for envelope mode only.
	// In legacy mode, these remain nil (not needed for legacy encryption).
	keysetRepo, registryRepo, auditRepo, auditWriter, err := initEncryptionRepos(kms, mongoConnection, logger)
	if err != nil {
		return nil, err
	}

	// Wire encryption services (ProtectionStateResolver, KeysetManager, EncryptionService, ProvisioningService).
	encryptionResult, err := buildEncryptionServices(cfg, kms, logger, keysetRepo, registryRepo, auditWriter, legacyCrypto, metricsFactory)
	if err != nil {
		return nil, err
	}

	// Create FieldEncryptor using FieldEncryptorAdapter wrapping EncryptionService.
	// EncryptionService is always available (legacy or envelope mode).
	fieldEncryptor := encryption.NewFieldEncryptorAdapter(encryptionResult.encryptionService)

	logger.Log(context.Background(), libLog.LevelInfo, "Encryption service initialized",
		libLog.String("mode", kms.Mode.String()))

	holderMongoDBRepository, err := holder.NewMongoDBRepository(mongoConnection, fieldEncryptor)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize holder repository: %w", err)
	}

	aliasMongoDBRepository, err := alias.NewMongoDBRepository(mongoConnection, fieldEncryptor)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize alias repository: %w", err)
	}

	// === Streaming producer (lib-streaming) ===
	// Built before the UseCase so the Emitter is available for injection.
	// When STREAMING_ENABLED=false (the documented default for this pilot)
	// the helper returns a NoopEmitter and a no-op closer, preserving full
	// backward compatibility with existing deployments.
	streamingEmitter, streamingClose, err := BuildStreamingEmitter(context.Background(), cfg, logger, telemetry)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize streaming emitter: %w", err)
	}

	// Drain the streaming producer on any partial-boot error return below. On
	// success the cleanup is disarmed (swapped to a no-op) because the Service
	// then owns the drain on SIGTERM. The defer covers every error path —
	// including ones added later — without each return closing manually.
	streamingCleanup := streamingClose

	defer func() { closeStreamingOnBootFailure(logger, streamingCleanup) }()

	useCases := &services.UseCase{
		HolderRepo: holderMongoDBRepository,
		AliasRepo:  aliasMongoDBRepository,
		Streaming:  streamingEmitter,
	}

	holderHandler := &in.HolderHandler{
		Service: useCases,
	}

	aliasHandler := &in.AliasHandler{
		Service: useCases,
	}

	// Create encryption handler only when provisioning service is available (envelope mode)
	encryptionHandler := newEncryptionHandler(encryptionResult.provisioningService, logger)

	// Create audit handler only when the read-side audit repository is available
	// (envelope mode). In legacy mode auditRepo is nil, so the handler is nil and
	// the protection audit route stays unregistered (404).
	auditHandler := newAuditHandler(auditRepo, logger)

	// Resolve the plugin-auth host via service discovery when auth is enabled,
	// degrading to the static PLUGIN_AUTH_ADDRESS on any resolve error so a
	// discovery outage never fails boot.
	sdResolveCtx, cancel := context.WithTimeout(context.Background(), serviceDiscoveryResolveTimeout)
	defer cancel()

	authHost := resolveAuthHost(sdResolveCtx, serviceDiscovery, cfg.AuthEnabled, cfg.AuthAddress)
	auth := middleware.NewAuthClient(authHost, cfg.AuthEnabled, nil)

	tenantMiddleware, eventListener, err := initTenantMiddleware(cfg, logger, telemetry)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tenant middleware: %w", err)
	}

	// Build readyz handler with MongoDB and Vault checkers (reuses the shared metricsFactory)
	readyzHandler, err := buildReadyzHandler(cfg, logger, mongoConnection, kms, metricsFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize readyz handler: %w", err)
	}

	httpApp := in.NewRouter(logger, telemetry, auth, tenantMiddleware, readyzHandler, holderHandler, aliasHandler, encryptionHandler, auditHandler)
	serverAPI := NewServer(cfg, httpApp, logger, telemetry, readyzHandler)

	streamingCleanup = noopStreamingCloser

	return &Service{
		Server:                  serverAPI,
		EventListener:           eventListener,
		EncryptionMode:          kms.Mode,
		VaultClient:             kms.VaultClient,
		KeysetRepo:              keysetRepo,
		RegistryRepo:            registryRepo,
		AuditRepo:               auditRepo,
		EncryptionService:       encryptionResult.encryptionService,
		ProvisioningService:     encryptionResult.provisioningService,
		ProtectionStateResolver: encryptionResult.protectionStateResolver,
		KeysetManager:           encryptionResult.keysetManager,
		StreamingEnabled:        cfg.StreamingEnabled,
		StreamingClose:          streamingClose,
		ServiceDiscovery:        serviceDiscovery,
		ServiceDiscoveryEnabled: serviceDiscoveryEnabled,
		ServiceDescriptor:       serviceDescriptor,
		Logger:                  logger,
	}, nil
}

// newEncryptionHandler returns an EncryptionHandler only when a provisioning
// service is available (envelope mode); otherwise it returns nil so the
// provisioning endpoints stay disabled in legacy mode.
func newEncryptionHandler(provisioningService encryption.ProvisioningService, logger libLog.Logger) *in.EncryptionHandler {
	if provisioningService == nil {
		return nil
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Encryption provisioning endpoints enabled")

	return &in.EncryptionHandler{ProvisioningService: provisioningService}
}

// newAuditHandler returns an AuditHandler only when a read-side audit repository
// is available (envelope mode). In legacy mode auditRepo is nil, so this returns
// nil and the protection audit route is left unregistered (404). That 404 is the
// correct "feature not applicable in legacy mode" behavior, not an error.
func newAuditHandler(auditRepo audit.Repository, logger libLog.Logger) *in.AuditHandler {
	if auditRepo == nil {
		return nil
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Protection audit endpoint enabled")

	return &in.AuditHandler{Service: encryption.NewAuditQueryService(auditRepo)}
}

// buildEncryptionServices wires the encryption services for the active KMS mode.
// In legacy mode the services use lib-commons crypto; in envelope mode they are
// wired with Tink and Vault dependencies and the envelope-only AuditWriter. It
// fails hard if envelope mode is requested but its dependencies are unavailable.
func buildEncryptionServices(
	cfg *Config,
	kms *KMSResult,
	logger libLog.Logger,
	keysetRepo mongoEncryption.KeysetRepository,
	registryRepo mongoEncryption.RegistryRepository,
	auditWriter encryption.AuditWriter,
	legacyCrypto encryption.LegacyCrypto,
	metricsFactory *metrics.MetricsFactory,
) (wireEncryptionServicesOutput, error) {
	encryptionResult := wireEncryptionServices(wireEncryptionServicesInput{
		mode:                 kms.Mode.String(),
		vaultClient:          kms.VaultClient,
		keysetRepo:           keysetRepo,
		registryRepo:         registryRepo,
		auditWriter:          auditWriter,
		legacyCrypto:         legacyCrypto,
		metricsFactory:       metricsFactory,
		multiTenant:          cfg.MultiTenantEnabled,
		allowGracefulDegrade: false, // Fail hard if envelope mode requested but dependencies unavailable
		legacyAESHexKey:      cfg.EncryptSecretKey,
		legacyHMACSecret:     cfg.HashSecretKey,
	})
	if encryptionResult.err != nil {
		return wireEncryptionServicesOutput{}, fmt.Errorf("failed to wire encryption services: %w", encryptionResult.err)
	}

	if encryptionResult.degradedToLegacy {
		logger.Log(context.Background(), libLog.LevelWarn, "Envelope encryption unavailable; degraded to legacy-only mode")
	}

	return encryptionResult, nil
}

// initLegacyCrypto builds the LegacyCrypto for the active KMS mode:
//   - envelope (KMS_VENDOR=hashicorp-vault): Tink-backed LegacyKeyMaterial, used
//     for both new encryption and reading legacy data during migration.
//   - legacy (KMS_VENDOR=none): lib-commons crypto directly (no Tink).
func initLegacyCrypto(cfg *Config, kms *KMSResult, logger libLog.Logger) (encryption.LegacyCrypto, error) {
	if kms.Mode.IsEnvelope() {
		legacyKeys, err := encryption.NewLegacyKeyMaterial(cfg.EncryptSecretKey, cfg.HashSecretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize legacy key material: %w", err)
		}

		return legacyKeys, nil
	}

	crypto := &libCrypto.Crypto{
		HashSecretKey:    cfg.HashSecretKey,
		EncryptSecretKey: cfg.EncryptSecretKey,
		Logger:           logger,
	}
	if err := crypto.InitializeCipher(); err != nil {
		return nil, fmt.Errorf("failed to initialize legacy crypto cipher: %w", err)
	}

	return crypto, nil
}

// initEncryptionRepos constructs the envelope-only encryption repositories, the
// read-side audit Repository, and a repository-backed AuditWriter. In legacy mode
// it returns nil for all of them: legacy encryption needs no keyset/registry
// repositories and has no provisioning service, so it needs neither an audit
// reader nor an audit writer. The audit repository is therefore envelope-only and
// never a no-op. A single auditRepo instance backs both the read path (returned
// directly) and the write path (wrapped by NewAuditWriter).
func initEncryptionRepos(
	kms *KMSResult,
	mongoConnection *libMongo.Client,
	logger libLog.Logger,
) (mongoEncryption.KeysetRepository, mongoEncryption.RegistryRepository, audit.Repository, encryption.AuditWriter, error) {
	if !kms.Mode.IsEnvelope() {
		return nil, nil, nil, nil, nil
	}

	keysetRepo, err := mongoEncryption.NewKeysetMongoDBRepository(mongoConnection)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize keyset repository: %w", err)
	}

	registryRepo, err := mongoEncryption.NewRegistryMongoDBRepository(mongoConnection)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize registry repository: %w", err)
	}

	auditRepo, err := audit.NewMongoDBRepository(mongoConnection)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize audit repository: %w", err)
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Encryption repositories initialized for envelope mode")

	return keysetRepo, registryRepo, auditRepo, encryption.NewAuditWriter(auditRepo, logger), nil
}

func initMongoConnection(cfg *Config, logger libLog.Logger) (*libMongo.Client, error) {
	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(cfg.MongoDBPort, cfg.MongoDBParameters, logger)

	hasStaticMongo := strings.TrimSpace(cfg.MongoURI) != "" || strings.TrimSpace(cfg.MongoDBHost) != ""
	if !hasStaticMongo {
		if cfg.MultiTenantEnabled {
			logger.Log(context.Background(), libLog.LevelInfo, "No static MongoDB configuration; multi-tenant mode will use tenant-specific connections")

			return nil, nil
		}

		return nil, fmt.Errorf("mongo configuration is required in single-tenant mode")
	}

	mongoURI, err := resolveMongoURI(cfg, mongoPort, mongoParameters)
	if err != nil {
		return nil, err
	}

	if cfg.MaxPoolSize <= 0 {
		logger.Log(context.Background(), libLog.LevelInfo, fmt.Sprintf("MaxPoolSize invalid (%d); defaulting to 100", cfg.MaxPoolSize))
		cfg.MaxPoolSize = 100
	}

	var tlsCfg *libMongo.TLSConfig
	if caCert := strings.TrimSpace(cfg.MongoTLSCACert); caCert != "" {
		tlsCfg = &libMongo.TLSConfig{CACertBase64: caCert}
	}

	mongoConnection, err := libMongo.NewClient(context.Background(), libMongo.Config{
		URI:         mongoURI,
		Database:    cfg.MongoDBName,
		MaxPoolSize: uint64(cfg.MaxPoolSize), // #nosec G115 -- guarded by <= 0 check above
		Logger:      logger,
		TLS:         tlsCfg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize mongodb client: %w", err)
	}

	return mongoConnection, nil
}

func resolveMongoURI(cfg *Config, mongoPort, mongoParameters string) (string, error) {
	rawURI := strings.TrimSpace(cfg.MongoURI)

	switch {
	case rawURI == "", rawURI == "mongodb", rawURI == "mongodb+srv":
		query, err := url.ParseQuery(mongoParameters)
		if err != nil {
			return "", fmt.Errorf("failed to parse mongodb parameters: %w", err)
		}

		scheme := rawURI
		if scheme == "" {
			scheme = "mongodb"
		}

		mongoURI, buildErr := libMongo.BuildURI(libMongo.URIConfig{
			Scheme:   scheme,
			Username: cfg.MongoDBUser,
			Password: cfg.MongoDBPassword,
			Host:     cfg.MongoDBHost,
			Port:     mongoPort,
			Query:    query,
		})
		if buildErr != nil {
			return "", fmt.Errorf("failed to build mongodb uri: %w", buildErr)
		}

		return mongoURI, nil
	case strings.Contains(rawURI, "://"):
		return rawURI, nil
	default:
		return "", fmt.Errorf("invalid MONGO_URI format: expected full URI or legacy scheme value")
	}
}

func resolveLoggerEnvironment(env string) libZap.Environment {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case string(libZap.EnvironmentProduction):
		return libZap.EnvironmentProduction
	case string(libZap.EnvironmentStaging):
		return libZap.EnvironmentStaging
	case string(libZap.EnvironmentUAT):
		return libZap.EnvironmentUAT
	case string(libZap.EnvironmentLocal):
		return libZap.EnvironmentLocal
	default:
		return libZap.EnvironmentDevelopment
	}
}

// buildReadyzHandler creates the ReadyzHandler with appropriate checkers based on configuration.
// In single-tenant mode, the MongoDB checker returns actual status.
// In multi-tenant mode (future), database checkers would return "n/a" globally.
// For envelope encryption mode, VaultChecker is added to monitor KMS availability.
func buildReadyzHandler(
	cfg *Config,
	logger libLog.Logger,
	mongoConnection *libMongo.Client,
	kms *KMSResult,
	metricsFactory *metrics.MetricsFactory,
) (*ReadyzHandler, error) {
	// Build Mongo URI for TLS detection
	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(cfg.MongoDBPort, cfg.MongoDBParameters, logger)

	mongoURI, err := resolveMongoURI(cfg, mongoPort, mongoParameters)
	if err != nil {
		// If we can't resolve the URI, use empty string (TLS detection will return false)
		logger.Log(context.Background(), libLog.LevelWarn,
			"Failed to resolve MongoDB URI for TLS detection, falling back to empty string",
			libLog.Err(err))

		mongoURI = ""
	}

	var checkers []DependencyChecker

	// tlsRelevantCheckers holds only real dependency checkers for TLS validation.
	// NAChecker is excluded since it represents a non-configured dependency.
	var tlsRelevantCheckers []DependencyChecker

	// MongoDB checker - returns actual status in single-tenant mode
	if mongoConnection != nil {
		mongoChecker := NewMongoChecker("mongo", mongoConnection, mongoURI)
		checkers = append(checkers, mongoChecker)
		tlsRelevantCheckers = append(tlsRelevantCheckers, mongoChecker)
	} else {
		// If no connection, add a skipped checker (excluded from TLS validation)
		tlsEnabled, _ := detectMongoTLS(mongoURI)
		checkers = append(checkers, NewNAChecker("mongo", "MongoDB client not configured", tlsEnabled))
	}

	// Vault checker - returns actual status in envelope encryption mode
	if kms != nil && kms.Mode.IsEnvelope() && kms.VaultClient != nil {
		vaultChecker := NewVaultChecker("vault", kms.VaultClient, cfg.VaultAddr)
		checkers = append(checkers, vaultChecker)
		tlsRelevantCheckers = append(tlsRelevantCheckers, vaultChecker)
	} else if kms != nil && kms.Mode.IsEnvelope() {
		// Envelope mode but no client - should not happen in production
		checkers = append(checkers, NewNAChecker("vault", "Vault client not configured", detectVaultTLS(cfg.VaultAddr)))
	} else {
		// Legacy mode - Vault is not applicable
		checkers = append(checkers, NewNAChecker("vault", "Legacy encryption mode (Vault not used)", false))
	}

	// Build TLS validation results from real dependency checkers only
	tlsResults := make([]TLSValidationResult, 0, len(tlsRelevantCheckers))
	for _, checker := range tlsRelevantCheckers {
		tlsResults = append(tlsResults, TLSValidationResult{
			Name:       checker.Name(),
			TLSEnabled: checker.TLSEnabled(),
		})
	}

	// ValidateSaaSTLS returns error ONLY for DEPLOYMENT_MODE=saas with insecure deps.
	// The error is returned to the caller to fail startup.
	if err := ValidateSaaSTLS(cfg.DeploymentMode, tlsResults); err != nil {
		return nil, err
	}

	// For BYOC mode, log a warning for insecure dependencies (recommended but not enforced)
	if IsTLSRecommended(cfg.DeploymentMode) {
		for _, result := range tlsResults {
			if !result.TLSEnabled {
				logger.Log(context.Background(), libLog.LevelWarn,
					"TLS recommended but not configured for dependency",
					libLog.String("dependency", result.Name),
					libLog.String("deployment_mode", cfg.DeploymentMode))
			}
		}
	}

	return NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         logger,
		Checkers:       checkers,
		Version:        cfg.Version,
		DeploymentMode: cfg.DeploymentMode,
		MetricsFactory: metricsFactory,
	}), nil
}
