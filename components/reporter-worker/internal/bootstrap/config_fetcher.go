// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	notificationAdapter "github.com/LerianStudio/midaz/v4/components/reporter-worker/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/components/reporter-worker/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/auth"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/fetcher"
	extractionRepo "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/extraction"
	libRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/storage"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	tmconsumer "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/consumer"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	clog "github.com/LerianStudio/lib-observability/log"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// logDatasourceMode logs which datasource mode is active at startup.
func logDatasourceMode(cfg *Config, logger clog.Logger) {
	if cfg.FetcherEnabled {
		logger.Log(context.Background(), clog.LevelInfo,
			"Datasource mode: FETCHER (delegated extraction)",
			clog.String("fetcher_url", cfg.FetcherURL))
	} else {
		logger.Log(context.Background(), clog.LevelInfo,
			"Datasource mode: DIRECT (local datasource connections)")
	}
}

// buildFetcherProvider creates a FetcherProvider using the datasource factory,
// wiring M2M auth when multi-tenant mode is active. The meter (may be nil)
// is used to build *fetcher.Metrics; when nil or instrument creation fails
// the client falls back to NoopMetrics so callers never see a nil emitter.
func buildFetcherProvider(
	cfg *Config,
	logger clog.Logger,
	tracer trace.Tracer,
	credFetcher auth.CredentialFetcher,
	meter metric.Meter,
) (datasource.DataSourceProvider, *fetcher.FetcherClient, error) {
	// Build Fetcher OTel metrics (F3: auth-retry counter). Single instance
	// shared between the provider's internal client and the standalone
	// extraction client constructed below.
	fetcherMetrics := fetcher.NoopMetrics()

	if meter != nil {
		fm, fmErr := fetcher.NewMetrics(meter)
		if fmErr != nil {
			return nil, nil, fmt.Errorf("failed to build fetcher metrics: %w", fmErr)
		}

		fetcherMetrics = fm
	}

	providerCfg := datasource.ProviderConfig{
		FetcherEnabled:     true,
		FetcherURL:         cfg.FetcherURL,
		MultiTenantEnabled: cfg.MultiTenantEnabled,
		FetcherClientOptions: []fetcher.FetcherClientOption{
			fetcher.WithMetrics(fetcherMetrics),
		},
	}

	// Wire M2M token provider when multi-tenant + auth is configured
	if cfg.MultiTenantEnabled && credFetcher != nil && cfg.AuthAddress != "" {
		m2mCfg := auth.M2MProviderConfig{
			AuthAddress:      cfg.AuthAddress,
			TargetService:    cfg.M2MTargetService,
			CredentialTTL:    time.Duration(cfg.M2MCredentialCacheTTLSec) * time.Second,
			TokenCacheMargin: time.Duration(cfg.M2MTokenCacheMarginSec) * time.Second,
		}

		m2mMetrics := auth.NoopM2MMetrics()
		m2mProvider := auth.NewM2MCredentialProvider(m2mCfg, credFetcher, logger, tracer, m2mMetrics)
		providerCfg.M2MTokenProvider = m2mProvider

		logger.Log(context.Background(), clog.LevelInfo,
			"M2M token provider configured for Fetcher client",
			clog.String("auth_address", cfg.AuthAddress),
			clog.String("target_service", cfg.M2MTargetService))
	}

	// Single-tenant + auth-enabled: use a static CLIENT_ID/CLIENT_SECRET to
	// exchange for an application token (mirrors how plugin-fees talks to
	// Midaz). The tenant-aware provider above cannot be used because no
	// tenantId is present on the worker's RabbitMQ-driven context.
	if providerCfg.M2MTokenProvider == nil && cfg.AuthEnabled && cfg.AuthAddress != "" && !cfg.MultiTenantEnabled {
		authClient := middleware.NewAuthClient(cfg.AuthAddress, cfg.AuthEnabled, &logger)

		staticProvider, staticErr := auth.NewStaticAppTokenProvider(authClient, cfg.ClientID, cfg.ClientSecret)
		if staticErr != nil {
			return nil, nil, fmt.Errorf("failed to build single-tenant fetcher token provider: %w", staticErr)
		}

		providerCfg.M2MTokenProvider = staticProvider

		logger.Log(context.Background(), clog.LevelInfo,
			"Static application token provider configured for Worker Fetcher client (single-tenant)",
			clog.String("auth_address", cfg.AuthAddress))
	}

	provider, err := datasource.NewProvider(providerCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create datasource provider: %w", err)
	}

	// Also create a standalone FetcherClient for extraction job operations
	// (the provider uses its own internal client for datasource listing).
	// Reuse the same metrics instance built above so both clients emit to
	// the same OTel counter.
	clientOpts := []fetcher.FetcherClientOption{
		fetcher.WithMetrics(fetcherMetrics),
	}
	if providerCfg.M2MTokenProvider != nil {
		clientOpts = append(clientOpts, fetcher.WithM2MTokenProvider(providerCfg.M2MTokenProvider))
	}

	fetcherClient := fetcher.NewFetcherClient(cfg.FetcherURL, clientOpts...)

	return provider, fetcherClient, nil
}

// initExtractionRepo creates the ExtractionMapping MongoDB repository.
// Uses lazy initialization for multi-tenant mode, eager for single-tenant.
func initExtractionRepo(
	cfg *Config,
	deps *workerDependencies,
	logger clog.Logger,
) (*extractionRepo.ExtractionMappingMongoDBRepository, error) {
	var (
		extRepo *extractionRepo.ExtractionMappingMongoDBRepository
		err     error
	)

	if cfg.MultiTenantEnabled {
		extRepo, err = extractionRepo.NewExtractionMappingMongoDBRepositoryLazy(deps.mongoConnection)
	} else {
		extRepo, err = extractionRepo.NewExtractionMappingMongoDBRepository(deps.mongoConnection)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize extraction mapping repository: %w", err)
	}

	logger.Log(context.Background(), clog.LevelInfo,
		"ExtractionMapping repository initialized",
		clog.Bool("lazy", cfg.MultiTenantEnabled))

	return extRepo, nil
}

// wireWorkerFetcherMode populates the worker UseCase fields for fetcher mode
// and starts the reconciler goroutine. This is called from both single-tenant
// and multi-tenant service initialization.
// tenantLister is non-nil in multi-tenant mode (enables per-tenant reconciliation).
// The returned CancelFunc stops the reconciler goroutine during graceful shutdown.
func wireWorkerFetcherMode(
	ctx context.Context,
	cfg *Config,
	logger clog.Logger,
	tracer trace.Tracer,
	deps *workerDependencies,
	redisRepo libRedis.RedisRepository,
	credFetcher auth.CredentialFetcher,
	tenantLister services.TenantLister,
	mongoManager *tmmongo.Manager,
) (context.CancelFunc, error) {
	// Resolve the OTel meter from deps.telemetry so the Fetcher client can
	// emit real instruments. May be nil when telemetry is disabled or when
	// Meter() returns an error — buildFetcherProvider falls back to noop.
	var meter metric.Meter

	if deps.telemetry != nil {
		if m, mErr := deps.telemetry.Meter(cfg.OtelLibraryName); mErr == nil {
			meter = m
		}
	}

	// Build provider and FetcherClient
	provider, fetcherClient, err := buildFetcherProvider(cfg, logger, tracer, credFetcher, meter)
	if err != nil {
		return nil, err
	}

	// Surface the provider to workerDependencies so the /readyz health
	// server's FetcherChecker can probe it via type-assertion. When fetcher
	// mode is disabled this stays nil and the checker reports skipped.
	deps.dataSourceProvider = provider

	// Create ExtractionMapping repository
	extRepo, err := initExtractionRepo(cfg, deps, logger)
	if err != nil {
		return nil, err
	}

	// Ensure indexes for single-tenant mode
	if !cfg.MultiTenantEnabled {
		if err = extRepo.EnsureIndexes(ctx); err != nil {
			return nil, fmt.Errorf("failed to ensure extraction mapping indexes: %w", err)
		}
	}

	// Wire UseCase fields
	deps.service.FetcherClient = fetcherClient
	deps.service.ExtractionMappingRepo = extRepo

	// Wire Fetcher data storage for downloading extracted results via S3.
	// Uses the same OBJECT_STORAGE_* credentials with a different bucket.
	// FETCHER_STORAGE_ENDPOINT overrides the endpoint (empty = same as OBJECT_STORAGE_ENDPOINT).
	endpoint := cfg.FetcherStorageEndpoint
	if endpoint == "" {
		endpoint = cfg.ObjectStorageEndpoint
	}

	fetcherStorageClient, err := storage.NewStorageClient(ctx, storage.Config{
		Bucket:            cfg.FetcherStorageBucket,
		S3Endpoint:        endpoint,
		S3Region:          cfg.ObjectStorageRegion,
		S3AccessKeyID:     cfg.ObjectStorageAccessKeyID,
		S3SecretAccessKey: cfg.ObjectStorageSecretKey,
		S3UsePathStyle:    cfg.ObjectStorageUsePathStyle,
		S3DisableSSL:      cfg.ObjectStorageDisableSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing fetcher data storage: %w", err)
	}

	deps.service.FetcherDataStorage = storage.NewFetcherStorageAdapter(fetcherStorageClient)

	logger.Log(ctx, clog.LevelInfo, "Fetcher data storage configured (S3)",
		clog.String("bucket", cfg.FetcherStorageBucket),
		clog.String("endpoint", endpoint))

	// Build and start Reconciler
	var reconcilerOpts []services.ReconcilerOption
	if cfg.ReconciliationIntervalMin > 0 {
		reconcilerOpts = append(reconcilerOpts,
			services.WithInterval(time.Duration(cfg.ReconciliationIntervalMin)*time.Minute))
	}

	if tenantLister != nil {
		reconcilerOpts = append(reconcilerOpts,
			services.WithMultiTenant(tenantLister, mongoManager, constant.ApplicationName))
	}

	reconciler := services.NewReconciler(
		deps.service,
		extRepo,
		fetcherClient,
		redisRepo,
		logger,
		tracer,
		reconcilerOpts...,
	)

	reconcilerCtx, reconcilerCancel := context.WithCancel(ctx) // #nosec G118 -- cancel is returned to the caller and invoked during graceful shutdown

	go reconciler.Start(reconcilerCtx)

	logger.Log(ctx, clog.LevelInfo, "Reconciler started in background goroutine")

	return reconcilerCancel, nil
}

// registerNotificationConsumerSingleTenant registers the Consumer 2 notification
// handler with ConsumerRoutes for single-tenant mode.
func registerNotificationConsumerSingleTenant(
	routes *notificationAdapter.ConsumerRoutes,
	service *services.UseCase,
	logger clog.Logger,
) {
	handler := notificationAdapter.NewNotificationConsumerHandler(service, logger)
	routes.Register(constant.FetcherNotificationQueue, handler.Handle)

	logger.Log(context.Background(), clog.LevelInfo,
		"Consumer 2 registered: Fetcher notification handler",
		clog.String("queue", constant.FetcherNotificationQueue))
}

// registerNotificationConsumerMultiTenant registers the Consumer 2 notification
// handler with the MultiTenantConsumer for multi-tenant mode.
// mongoManager is used to resolve per-tenant MongoDB before handling the message.
func registerNotificationConsumerMultiTenant(
	mtConsumer MultiTenantConsumerInterface,
	service *services.UseCase,
	logger clog.Logger,
	mongoManager *tmmongo.Manager,
) error {
	handler := notificationAdapter.NewNotificationConsumerHandler(service, logger)

	// Build a lightweight resolver to reuse the shared resolveMultiTenantMongo logic.
	// to make dependency surface explicit. Current partial struct is safe because the method
	// only reads logger and mongoManager, but fragile if extended. (reported by code-reviewer,
	// nil-safety-reviewer, security-reviewer on 2026-03-23, severity: Medium)
	resolver := &MultiQueueConsumer{logger: logger, mongoManager: mongoManager}

	// Wrap to match tmconsumer.HandlerFunc signature: func(ctx, amqp.Delivery) error
	// DEFENSIVE RETRY GUARD: lib-commons multi-tenant consumer calls msg.Nack(false, true)
	// for any non-nil handler error, which causes infinite redelivery for permanent errors.
	// This handler returns nil for non-retryable errors (after logging) so that lib-commons
	// Acks the message instead of requeuing it indefinitely.
	wrappedHandler := func(ctx context.Context, delivery amqp.Delivery) error {
		ctx, err := resolver.resolveMultiTenantMongo(ctx)
		if err != nil {
			if isNonRetryableHandlerError(err) {
				logger.Log(ctx, clog.LevelError, "Consumer 2: permanent tenant resolution failure (message will be dropped)",
					clog.Int("body_length", len(delivery.Body)),
					clog.Err(err))

				return nil
			}

			return err
		}

		err = handler.Handle(ctx, delivery.Body)
		if err != nil {
			if isNonRetryableHandlerError(err) {
				logger.Log(ctx, clog.LevelError, "Consumer 2: non-retryable notification handler error (message will be dropped)",
					clog.Int("body_length", len(delivery.Body)),
					clog.Err(err))

				return nil
			}

			return err
		}

		return nil
	}

	if err := mtConsumer.Register(constant.FetcherNotificationQueue, tmconsumer.HandlerFunc(wrappedHandler)); err != nil {
		return fmt.Errorf("failed to register notification consumer: %w", err)
	}

	logger.Log(context.Background(), clog.LevelInfo,
		"Consumer 2 registered (multi-tenant): Fetcher notification handler",
		clog.String("queue", constant.FetcherNotificationQueue))

	return nil
}

// buildWorkerRedisConnection creates a Redis connection from the worker config.
// Used by the reconciler for distributed locking in fetcher mode.
func buildWorkerRedisConnection(cfg *Config, logger clog.Logger) *libRedis.RedisConnection {
	return &libRedis.RedisConnection{
		Address:                      strings.Split(cfg.RedisHost, ","),
		Password:                     cfg.RedisPassword,
		DB:                           cfg.RedisDB,
		Protocol:                     cfg.RedisProtocol,
		MasterName:                   cfg.RedisMasterName,
		UseTLS:                       cfg.RedisTLS,
		CACert:                       cfg.RedisCACert,
		UseGCPIAMAuth:                cfg.RedisUseGCPIAM,
		ServiceAccount:               cfg.RedisServiceAccount,
		GoogleApplicationCredentials: cfg.GoogleApplicationCredentials,
		TokenLifeTime:                time.Duration(cfg.RedisTokenLifeTime) * time.Minute,
		RefreshDuration:              time.Duration(cfg.RedisTokenRefreshDuration) * time.Minute,
		Logger:                       logger,
	}
}
