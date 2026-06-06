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

	workerRedis "github.com/LerianStudio/midaz/v4/components/reporter-worker/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v4/components/reporter-worker/internal/services"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/auth"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	pkgCrypto "github.com/LerianStudio/midaz/v4/pkg/reporter/crypto"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"
	mongoDB "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	reportData "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/multitenant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/pdf"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/readyz"
	libRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"
	reportSeaweedFS "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/report"
	templateSeaweedFS "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/template"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/storage"

	libRabbitMQ "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	clog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// workerDependencies holds the shared infrastructure components created during
// worker initialization and reused by both single-tenant and multi-tenant paths.
type workerDependencies struct {
	ctx                context.Context
	telemetry          *libOtel.Telemetry
	tracer             trace.Tracer
	mongoConnection    *mongoDB.MongoConnection
	reportRepo         *reportData.ReportMongoDBRepository
	healthChecker      *pkg.HealthChecker
	pdfPool            *pdf.WorkerPool
	service            *services.UseCase
	storageClient      storage.ObjectStorage
	dataSourceProvider datasource.DataSourceProvider
	// readyzMetrics is the OTel emitter for the canonical /readyz metric
	// set. Built once at bootstrap and shared between the HealthServer
	// (which emits per-check histogram + counter) and Gate 7's RunSelfProbe
	// (which will emit selfprobe_result via Metrics.EmitSelfProbeResult).
	readyzMetrics *readyz.Metrics
}

// initWorkerDependencies creates all shared infrastructure (telemetry, storage,
// MongoDB, datasources, PDF pool, health checker) and wires them into a UseCase
// service. Resources are registered in the CleanupManager for graceful shutdown.
func initWorkerDependencies(cfg *Config, logger clog.Logger, cleanups *CleanupManager) (*workerDependencies, error) {
	telemetry, err := initWorkerTelemetry(cfg, logger, cleanups)
	if err != nil {
		return nil, err
	}

	ctx := ctxutil.ContextWithLogger(context.Background(), logger)

	storageClient, err := initStorageClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	logger.Log(ctx, clog.LevelInfo, "Storage initialized", clog.String("bucket", cfg.ObjectStorageBucket), clog.String("template_prefix", "templates/"), clog.String("report_prefix", "reports/"))

	mongoConnection := buildMongoConnection(cfg, logger)

	var reportMongoDBRepository *reportData.ReportMongoDBRepository
	if cfg.MultiTenantEnabled {
		reportMongoDBRepository, err = reportData.NewReportMongoDBRepositoryLazy(mongoConnection)
	} else {
		reportMongoDBRepository, err = reportData.NewReportMongoDBRepository(mongoConnection)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize report mongodb repository: %w", err)
	}

	appendWorkerMongoCleanup(logger, mongoConnection, cleanups)

	if !cfg.MultiTenantEnabled {
		logger.Log(ctx, clog.LevelInfo, "Ensuring MongoDB indexes exist for reports...")

		if err = reportMongoDBRepository.EnsureIndexes(ctx); err != nil {
			return nil, fmt.Errorf("failed to ensure report indexes: %w", err)
		}
	}

	tracer, err := telemetry.Tracer(cfg.OtelLibraryName)
	if err != nil {
		return nil, fmt.Errorf("failed to create tracer: %w", err)
	}

	// Log datasource mode (Gap 9)
	logDatasourceMode(cfg, logger)

	circuitBreakerManager := pkg.NewCircuitBreakerManager(logger)

	// Build the readyz + datasource metric sets on the same meter used by
	// the rest of the worker. Both NewMetrics constructors tolerate a nil
	// meter (noop fallback) so partial-bootstrap paths don't crash.
	var meterForMetrics metric.Meter

	if telemetry != nil {
		if meter, mErr := telemetry.Meter(cfg.OtelLibraryName); mErr == nil {
			meterForMetrics = meter
		}
	}

	readyzMetrics, err := readyz.NewMetrics(meterForMetrics)
	if err != nil {
		return nil, fmt.Errorf("failed to register readyz metrics: %w", err)
	}

	dsMetrics, err := pkg.NewDatasourceMetrics(meterForMetrics)
	if err != nil {
		return nil, fmt.Errorf("failed to register datasource metrics: %w", err)
	}

	logger.Log(ctx, clog.LevelInfo, "Readyz + datasource metrics registered with OTel provider", clog.Bool("real_meter", meterForMetrics != nil))

	// Gap 3: Only connect to external datasources in direct mode.
	// In fetcher mode, the Fetcher service handles datasource connections.
	var externalDataSources *pkg.SafeDataSources

	var healthChecker *pkg.HealthChecker

	if !cfg.FetcherEnabled {
		externalDataSourcesMap := pkg.ExternalDatasourceConnections(logger)
		externalDataSources = pkg.NewSafeDataSources(externalDataSourcesMap)

		healthChecker, err = pkg.NewHealthCheckerWithMetrics(&externalDataSourcesMap, circuitBreakerManager, logger, dsMetrics)
		if err != nil {
			return nil, fmt.Errorf("failed to construct health checker: %w", err)
		}
	} else {
		logger.Log(ctx, clog.LevelInfo, "Fetcher mode enabled — skipping direct datasource connections")

		emptyMap := make(map[string]pkg.DataSource)
		externalDataSources = pkg.NewSafeDataSources(emptyMap)

		healthChecker, err = pkg.NewHealthCheckerWithMetrics(&emptyMap, circuitBreakerManager, logger, dsMetrics)
		if err != nil {
			return nil, fmt.Errorf("failed to construct health checker: %w", err)
		}
	}

	pdfPool := pdf.NewWorkerPool(cfg.PdfPoolWorkers, time.Duration(cfg.PdfPoolTimeoutSeconds)*time.Second, logger)
	logger.Log(ctx, clog.LevelInfo, "PDF pool initialized", clog.Int("workers", cfg.PdfPoolWorkers), clog.Int("timeout_seconds", cfg.PdfPoolTimeoutSeconds))
	appendWorkerPDFCleanup(logger, pdfPool, cleanups)

	service := &services.UseCase{
		Logger:                          logger,
		Tracer:                          tracer,
		TemplateSeaweedFS:               templateSeaweedFS.NewStorageRepository(storageClient),
		ReportSeaweedFS:                 reportSeaweedFS.NewStorageRepository(storageClient),
		ExternalDataSources:             externalDataSources,
		ReportDataRepo:                  reportMongoDBRepository,
		CircuitBreakerManager:           circuitBreakerManager,
		HealthChecker:                   healthChecker,
		ReportTTL:                       "",
		PdfPool:                         pdfPool,
		CryptoHashSecretKeyPluginCRM:    cfg.CryptoHashSecretKeyPluginCRM,
		CryptoEncryptSecretKeyPluginCRM: cfg.CryptoEncryptSecretKeyPluginCRM,
		AppEncKey:                       cfg.AppEncKey,
		// FetcherDataStorage is wired in config_fetcher.go when FETCHER_ENABLED=true
	}

	// Derive storage decryption and HMAC keys from APP_ENC_KEY using HKDF.
	// These derived keys are compatible with the Fetcher's encryption.
	if cfg.AppEncKey != "" {
		masterKey, mkErr := pkgCrypto.DecodeMasterKey(cfg.AppEncKey)
		if mkErr != nil {
			return nil, fmt.Errorf("failed to decode APP_ENC_KEY: %w", mkErr)
		}

		keyDeriver, kdErr := pkgCrypto.NewHKDFKeyDeriver(masterKey)
		if kdErr != nil {
			return nil, fmt.Errorf("failed to initialize key deriver: %w", kdErr)
		}

		service.StorageDecryptKey = keyDeriver.GetStorageEncryptKey()
		service.ExternalHMACKey = keyDeriver.GetExternalHMACKey()

		logger.Log(ctx, clog.LevelInfo, "HKDF key derivation initialized from APP_ENC_KEY")
	}

	logger.Log(ctx, clog.LevelInfo, "Reports will be stored permanently (no TTL - use S3 bucket lifecycle policies for expiration)")

	healthChecker.Start()
	appendWorkerHealthCleanup(logger, healthChecker, cleanups)

	return &workerDependencies{
		ctx:             ctx,
		telemetry:       telemetry,
		tracer:          tracer,
		mongoConnection: mongoConnection,
		reportRepo:      reportMongoDBRepository,
		healthChecker:   healthChecker,
		pdfPool:         pdfPool,
		service:         service,
		storageClient:   storageClient,
		readyzMetrics:   readyzMetrics,
	}, nil
}

// initWorkerTelemetry creates and configures the OpenTelemetry instance,
// applies global providers, and registers a shutdown cleanup.
func initWorkerTelemetry(cfg *Config, logger clog.Logger, cleanups *CleanupManager) (*libOtel.Telemetry, error) {
	telemetry, err := libOtel.NewTelemetry(libOtel.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		InsecureExporter:          cfg.OtelInsecureExporter,
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	if err = telemetry.ApplyGlobals(); err != nil {
		return nil, fmt.Errorf("failed to apply telemetry globals: %w", err)
	}

	cleanups.Register("shutting down telemetry", func() {
		telemetry.ShutdownTelemetry()
	})

	return telemetry, nil
}

// appendWorkerMongoCleanup registers a shutdown hook that disconnects the
// MongoDB connection when the worker is shutting down.
func appendWorkerMongoCleanup(logger clog.Logger, mongoConnection *mongoDB.MongoConnection, cleanups *CleanupManager) {
	cleanups.Register("disconnecting MongoDB", func() {
		if mongoConnection == nil {
			return
		}

		if disconnectErr := mongoConnection.Close(); disconnectErr != nil {
			logger.Log(context.Background(), clog.LevelError, "Failed to disconnect MongoDB", clog.Err(disconnectErr))
		}
	})
}

// appendWorkerPDFCleanup registers a shutdown hook that drains and closes
// the PDF rendering worker pool.
func appendWorkerPDFCleanup(_ clog.Logger, pdfPool *pdf.WorkerPool, cleanups *CleanupManager) {
	cleanups.Register("closing PDF worker pool", func() {
		pdfPool.Close()
	})
}

// appendWorkerHealthCleanup registers a shutdown hook that stops the
// periodic health checker goroutine.
func appendWorkerHealthCleanup(_ clog.Logger, healthChecker *pkg.HealthChecker, cleanups *CleanupManager) {
	cleanups.Register("stopping health checker", func() {
		healthChecker.Stop()
	})
}

// initMultiTenantWorkerService assembles the worker Service for multi-tenant mode.
// It initializes the multi-tenant RabbitMQ consumer, optionally wires fetcher mode
// with M2M credentials and reconciler, and configures the health server with
// tenant-manager and Redis readiness checks.
func initMultiTenantWorkerService(cfg *Config, logger clog.Logger, tmClient *tmclient.Client, tenantMongoManager *tmmongo.Manager, deps *workerDependencies, cleanups *CleanupManager) (*Service, error) {
	// Register multi-tenant OTel metrics with the telemetry provider.
	// Real instruments are used so metrics appear in dashboards; falls back to noop on error.
	var mtMetrics *multitenant.Metrics

	meter, mErr := deps.telemetry.Meter(cfg.OtelLibraryName)
	if mErr == nil {
		mtMetrics, _ = multitenant.NewMetrics(meter)
	}

	if mtMetrics == nil {
		mtMetrics = multitenant.NoopMetrics()
	}

	_ = mtMetrics // metrics registered with OTel provider; not yet passed to services

	logger.Log(deps.ctx, clog.LevelInfo, "Multi-tenant metrics registered with OTel provider")

	mtConsumer, rabbitMQManager, mtRedisConn, mtCleanup, mtErr := initMultiTenantConsumer(deps.ctx, cfg, logger, tenantMongoManager, tmClient)
	if mtErr != nil {
		return nil, mtErr
	}

	cleanups.Register("multi-tenant consumer cleanup", mtCleanup)

	var reconcilerCancel context.CancelFunc

	if cfg.FetcherEnabled {
		redisRepo, redisRepoErr := workerRedis.NewWorkerRedis(mtRedisConn)
		if redisRepoErr != nil {
			logger.Log(deps.ctx, clog.LevelWarn,
				"Redis unavailable for reconciler distributed lock — reconciler will run without locking",
				clog.Err(redisRepoErr))
		}

		var redisRepoInterface libRedis.RedisRepository
		if redisRepo != nil {
			redisRepoInterface = redisRepo
		}

		credFetcher, credErr := auth.BuildCredentialFetcher(deps.ctx, auth.SMCredentialFetcherConfig{
			AWSRegion:       cfg.AWSRegion,
			Environment:     cfg.MultiTenantEnvironment,
			ApplicationName: constant.ApplicationName,
		})
		if credErr != nil {
			return nil, fmt.Errorf("failed to build M2M credential fetcher: %w", credErr)
		}

		var fetcherErr error

		reconcilerCancel, fetcherErr = wireWorkerFetcherMode(deps.ctx, cfg, logger, deps.tracer, deps, redisRepoInterface, credFetcher, tmClient, tenantMongoManager)
		if fetcherErr != nil {
			return nil, fetcherErr
		}

		if regErr := registerNotificationConsumerMultiTenant(mtConsumer, deps.service, logger, tenantMongoManager); regErr != nil {
			return nil, regErr
		}
	}

	// drainState is created early because both the consumer and the health
	// server share it. The drainState is wired below after construction.
	multiQueueConsumer, mtConsumerErr := NewMultiQueueConsumerMultiTenant(mtConsumer, deps.service, cfg.RabbitMQGenerateReportQueue, logger, tenantMongoManager, nil)
	if mtConsumerErr != nil {
		return nil, mtConsumerErr
	}

	// Event-driven tenant discovery via Redis Pub/Sub.
	// Created AFTER both consumers are registered (generate-report + fetcher-notification)
	// so that EnsureConsumerStarted starts both queue consumers for a tenant.
	eventListenerCleanup, elErr := initEventListener(cfg, logger, tmClient, mtConsumer, tenantMongoManager, rabbitMQManager)
	if elErr != nil {
		return nil, elErr
	}

	// Discover existing tenants on startup so consumers start immediately.
	performInitialTenantSync(deps.ctx, logger, tmClient, mtConsumer)

	drainState := &readyz.DrainState{}

	// Gate 7: self-probe gates /health. Starts unhealthy; flipped below by
	// readyz.RunSelfProbe iff every dep reports up. Failure leaves it
	// false, /health returns 503, and K8s livenessProbe restarts the pod
	// cleanly (no os.Exit — log collection stays intact).
	selfProbeState := &readyz.SelfProbeState{}

	healthCfg := HealthServerConfig{
		Port:                cfg.HealthPort,
		MongoConnection:     nil, // multi-tenant: per-tenant probing deferred
		RabbitMQConnection:  nil, // multi-tenant: per-tenant probing deferred
		RedisConnection:     mtRedisConn,
		StorageClient:       deps.storageClient,
		StorageEndpoint:     cfg.ObjectStorageEndpoint,
		DataSourceProvider:  deps.dataSourceProvider,
		FetcherURL:          cfg.FetcherURL,
		TenantManagerClient: tmClient,
		MultiTenantEnabled:  true,
		FetcherEnabled:      cfg.FetcherEnabled,
		MongoURI:            cfg.MongoURI,
		RabbitURI:           cfg.RabbitURI,
		DrainState:          drainState,
		Version:             cfg.OtelServiceVersion,
		DeploymentMode:      cfg.DeploymentMode,
		Logger:              logger,
		Metrics:             deps.readyzMetrics,
		SelfProbeState:      selfProbeState,
	}
	healthServer := NewHealthServer(healthCfg)

	runWorkerSelfProbe(deps.ctx, healthCfg, deps.readyzMetrics, selfProbeState, logger)

	logger.Log(deps.ctx, clog.LevelInfo, "Health server configured", clog.String("port", cfg.HealthPort), clog.Bool("multi_tenant", true), clog.Any("endpoints", []string{"/health", "/readyz"}))

	multiQueueConsumer.drainState = drainState

	return &Service{
		MultiQueueConsumer:   multiQueueConsumer,
		Logger:               logger,
		healthChecker:        deps.healthChecker,
		healthServer:         healthServer,
		mongoConnection:      deps.mongoConnection,
		pdfPool:              deps.pdfPool,
		telemetry:            deps.telemetry,
		mtConsumer:           mtConsumer,
		mtCleanup:            mtCleanup,
		reconcilerCancel:     reconcilerCancel,
		eventListenerCleanup: eventListenerCleanup,
		drainState:           drainState,
		selfProbeState:       selfProbeState,
	}, nil
}

// initSingleTenantWorkerService assembles the worker Service for single-tenant mode.
// It connects directly to RabbitMQ, optionally wires fetcher mode with a local Redis
// lock for the reconciler, and configures the health server with a RabbitMQ readiness probe.
func initSingleTenantWorkerService(cfg *Config, logger clog.Logger, deps *workerDependencies, cleanups *CleanupManager) (*Service, error) {
	// Single-tenant mode: register noop metrics (no OTel overhead).
	mtMetrics := multitenant.NoopMetrics()
	_ = mtMetrics // metrics registered but using no-op in single-tenant mode

	rabbitSource := fmt.Sprintf("%s://%s:%s@%s:%s", cfg.RabbitURI, url.QueryEscape(cfg.RabbitMQUser), url.QueryEscape(cfg.RabbitMQPass), cfg.RabbitMQHost, cfg.RabbitMQPortAMQP)
	logger.Log(context.Background(), clog.LevelInfo, "RabbitMQ connecting", clog.String("dsn", pkg.RedactConnectionString(rabbitSource)))

	rabbitMQConnection := &libRabbitMQ.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortHost,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		Queue:                  cfg.RabbitMQGenerateReportQueue,
		AllowInsecureHealthCheck: strings.HasPrefix(strings.ToLower(cfg.RabbitMQHealthCheckURL), "http://") &&
			strings.ToLower(cfg.EnvName) != "production",
		Logger: logger,
	}

	routes, err := initConsumerRoutes(rabbitMQConnection, cfg.RabbitMQNumWorkers, logger, deps.telemetry, deps.reportRepo)
	if err != nil {
		return nil, err
	}

	cleanups.Register("closing RabbitMQ connection", closeRabbitMQ(rabbitMQConnection, logger))

	var (
		reconcilerCancel context.CancelFunc
		redisConn        *libRedis.RedisConnection
	)

	if cfg.FetcherEnabled {
		var redisRepoInterface libRedis.RedisRepository

		if cfg.RedisHost != "" {
			redisConn = buildWorkerRedisConnection(cfg, logger)

			redisRepo, redisRepoErr := workerRedis.NewWorkerRedis(redisConn)
			if redisRepoErr != nil {
				logger.Log(deps.ctx, clog.LevelWarn,
					"Redis unavailable for reconciler distributed lock — reconciler will run without locking",
					clog.Err(redisRepoErr))
			} else {
				redisRepoInterface = redisRepo

				cleanups.Register("closing worker Redis connection", func() {
					if closeErr := redisConn.Close(); closeErr != nil {
						logger.Log(context.Background(), clog.LevelError, "Failed to close worker Redis connection", clog.Err(closeErr))
					}
				})
			}
		}

		var fetcherErr error

		reconcilerCancel, fetcherErr = wireWorkerFetcherMode(deps.ctx, cfg, logger, deps.tracer, deps, redisRepoInterface, nil, nil, nil)
		if fetcherErr != nil {
			return nil, fetcherErr
		}

		registerNotificationConsumerSingleTenant(routes, deps.service, logger)
	}

	drainState := &readyz.DrainState{}

	// Gate 7: self-probe gates /health (see comment in multi-tenant path).
	selfProbeState := &readyz.SelfProbeState{}

	multiQueueConsumer := NewMultiQueueConsumer(routes, deps.service, cfg.RabbitMQGenerateReportQueue, logger, drainState)
	healthCfg := HealthServerConfig{
		Port:                cfg.HealthPort,
		MongoConnection:     deps.mongoConnection,
		RabbitMQConnection:  rabbitMQConnection,
		RedisConnection:     redisConn,
		StorageClient:       deps.storageClient,
		StorageEndpoint:     cfg.ObjectStorageEndpoint,
		DataSourceProvider:  deps.dataSourceProvider,
		FetcherURL:          cfg.FetcherURL,
		TenantManagerClient: nil,
		MultiTenantEnabled:  false,
		FetcherEnabled:      cfg.FetcherEnabled,
		MongoURI:            cfg.MongoURI,
		RabbitURI:           cfg.RabbitURI,
		DrainState:          drainState,
		Version:             cfg.OtelServiceVersion,
		DeploymentMode:      cfg.DeploymentMode,
		Logger:              logger,
		Metrics:             deps.readyzMetrics,
		SelfProbeState:      selfProbeState,
	}
	healthServer := NewHealthServer(healthCfg)

	runWorkerSelfProbe(deps.ctx, healthCfg, deps.readyzMetrics, selfProbeState, logger)

	logger.Log(context.Background(), clog.LevelInfo, "Health server configured", clog.String("port", cfg.HealthPort), clog.Bool("multi_tenant", false), clog.Any("endpoints", []string{"/health", "/readyz"}))

	return &Service{
		MultiQueueConsumer: multiQueueConsumer,
		Logger:             logger,
		healthChecker:      deps.healthChecker,
		healthServer:       healthServer,
		mongoConnection:    deps.mongoConnection,
		rabbitMQConnection: rabbitMQConnection,
		pdfPool:            deps.pdfPool,
		telemetry:          deps.telemetry,
		reconcilerCancel:   reconcilerCancel,
		drainState:         drainState,
		selfProbeState:     selfProbeState,
	}, nil
}

// runWorkerSelfProbe runs readyz.RunSelfProbe with the worker's checker set
// and flips selfProbeState.MarkHealthy() iff every dep reports up. Failure
// leaves the state unhealthy and the bootstrap returns normally — /health
// will return 503 and K8s livenessProbe restarts the pod cleanly. We
// deliberately DO NOT call os.Exit on probe failure (anti-pattern #7 in
// dev-readyz/SKILL.md): the process must stay alive long enough for
// telemetry to flush and CloudWatch / Loki to capture the failure logs.
func runWorkerSelfProbe(ctx context.Context, healthCfg HealthServerConfig, metrics *readyz.Metrics, state *readyz.SelfProbeState, logger clog.Logger) {
	checkers := BuildWorkerCheckers(healthCfg)

	if probeErr := readyz.RunSelfProbe(ctx, checkers, metrics, logger); probeErr != nil {
		logger.Log(ctx, clog.LevelError,
			"startup_self_probe_failed_letting_pod_stay_unhealthy",
			clog.Err(probeErr))

		return
	}

	state.MarkHealthy()
	logger.Log(ctx, clog.LevelInfo, "startup_self_probe_marked_healthy")
}
