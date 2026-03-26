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

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/v4/commons/opentelemetry/metrics"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmevent "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/event"
	tmmiddleware "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

const moduleName = "crm"

// initTenantMiddleware creates the tenant middleware for multi-tenant mode.
// Returns (nil, nil, nil) when multi-tenant is disabled or the URL is not configured.
// The middleware extracts tenantId from JWT, resolves the tenant-specific
// MongoDB connection via Tenant Manager, and injects it into the request context.
// When telemetry is non-nil, emits tenant_connection_errors_total on handler errors.
//
// In addition to the middleware handler, it returns a TenantEventListener that
// subscribes to Redis Pub/Sub for tenant lifecycle events (suspend, delete, etc.).
// The listener is nil when MULTI_TENANT_REDIS_HOST is not configured.
func initTenantMiddleware(
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
) (fiber.Handler, *tmevent.TenantEventListener, error) {
	if !cfg.MultiTenantEnabled {
		return nil, nil, nil
	}

	mtURL := strings.TrimSpace(cfg.MultiTenantURL)
	if mtURL == "" {
		return nil, nil, fmt.Errorf("MULTI_TENANT_URL must not be blank when MULTI_TENANT_ENABLED=true")
	}

	tenantServiceName := strings.TrimSpace(cfg.ApplicationName)
	if tenantServiceName == "" {
		return nil, nil, fmt.Errorf("APPLICATION_NAME must not be blank when MULTI_TENANT_ENABLED=true")
	}

	clientOpts, err := buildTenantClientOptions(cfg, mtURL)
	if err != nil {
		return nil, nil, err
	}

	tmClient, err := tmclient.NewClient(mtURL, logger, clientOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize tenant manager client: %w", err)
	}

	mongoOpts := buildMongoManagerOptions(cfg, logger)

	mongoManager := tmmongo.NewManager(tmClient, tenantServiceName, mongoOpts...)

	// Tenant cache and loader for event-driven discovery
	tenantCache := tenantcache.NewTenantCache()

	cacheTTL := time.Duration(cfg.MultiTenantCacheTTLSec) * time.Second
	tenantLoader := tenantcache.NewTenantLoader(tmClient, tenantCache, tenantServiceName, cacheTTL, logger)

	tenantMid := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithMongoManager(mongoManager),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	logger.Log(context.Background(), libLog.LevelInfo, fmt.Sprintf("Multi-tenant middleware initialized: target=%s service=%s",
		redactedTenantManagerURL(mtURL), tenantServiceName))

	handler := wrapTenantMiddlewareWithMetrics(tenantMid.WithTenantDB, telemetry, logger)

	// Event dispatcher and listener for tenant lifecycle events
	eventListener, err := initTenantEventListener(cfg, logger, tmClient, tenantServiceName, tenantCache, tenantLoader, mongoManager, cacheTTL)
	if err != nil {
		return nil, nil, err
	}

	return handler, eventListener, nil
}

// initTenantEventListener creates the EventDispatcher and EventListener for
// tenant lifecycle events (suspend, delete, disassociate). Returns nil when
// MULTI_TENANT_REDIS_HOST is not configured.
func initTenantEventListener(
	cfg *Config,
	logger libLog.Logger,
	tmClient *tmclient.Client,
	tenantServiceName string,
	tenantCache *tenantcache.TenantCache,
	tenantLoader *tenantcache.TenantLoader,
	mongoManager *tmmongo.Manager,
	cacheTTL time.Duration,
) (*tmevent.TenantEventListener, error) {
	dispatcher := tmevent.NewEventDispatcher(
		tenantCache,
		tenantLoader,
		tenantServiceName,
		tmevent.WithDispatcherLogger(logger),
		tmevent.WithCacheTTL(cacheTTL),
		tmevent.WithMongo(mongoManager),
		tmevent.WithOnTenantRemoved(func(ctx context.Context, tenantID string) {
			// Close MongoDB manager connection for the evicted tenant
			if mongoManager != nil {
				if err := mongoManager.CloseConnection(ctx, tenantID); err != nil {
					logger.Log(ctx, libLog.LevelWarn, "failed to close Mongo connection",
						libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
				}
			}

			// Invalidate pmClient internal cache so lazy-load fetches fresh state
			if tmClient != nil {
				if err := tmClient.InvalidateConfig(ctx, tenantID, tenantServiceName); err != nil {
					logger.Log(ctx, libLog.LevelWarn, "failed to invalidate tenant config cache",
						libLog.String("tenant_id", tenantID), libLog.String("error", err.Error()))
				}
			}

			logger.Log(ctx, libLog.LevelInfo, "tenant evicted: all connections and caches invalidated",
				libLog.String("tenant_id", tenantID))
		}),
	)

	redisHost := strings.TrimSpace(cfg.MultiTenantRedisHost)
	if redisHost == "" {
		logger.Log(context.Background(), libLog.LevelWarn,
			"MULTI_TENANT_REDIS_HOST not configured; tenant event listener will NOT start (no Pub/Sub)")

		return nil, nil
	}

	redisPort := strings.TrimSpace(cfg.MultiTenantRedisPort)
	if redisPort == "" {
		redisPort = "6379"
	}

	tmRedisConfig := libRedis.Config{
		Topology: libRedis.Topology{
			Standalone: &libRedis.StandaloneTopology{
				Address: redisHost + ":" + redisPort,
			},
		},
		Logger: logger,
	}

	if cfg.MultiTenantRedisPassword != "" {
		tmRedisConfig.Auth = libRedis.Auth{
			StaticPassword: &libRedis.StaticPasswordAuth{Password: cfg.MultiTenantRedisPassword},
		}
	}

	tmRedisConn, err := libRedis.New(context.Background(), tmRedisConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tenant-manager Redis for Pub/Sub: %w", err)
	}

	tmRedisClient, err := tmRedisConn.GetClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant-manager Redis client: %w", err)
	}

	eventListener, err := tmevent.NewTenantEventListener(
		tmRedisClient,
		dispatcher.HandleEvent,
		tmevent.WithListenerLogger(logger),
		tmevent.WithService(tenantServiceName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant event listener: %w", err)
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Tenant event listener configured",
		libLog.String("redis_host", redisHost),
		libLog.String("service", tenantServiceName),
	)

	return eventListener, nil
}

func buildTenantClientOptions(cfg *Config, mtURL string) ([]tmclient.ClientOption, error) {
	clientOpts := make([]tmclient.ClientOption, 0)

	clientOpts = append(clientOpts, tmclient.WithServiceAPIKey(cfg.MultiTenantServiceAPIKey))

	if cfg.MultiTenantCacheTTLSec >= 0 {
		clientOpts = append(clientOpts, tmclient.WithCacheTTL(time.Duration(cfg.MultiTenantCacheTTLSec)*time.Second))
	}

	if cfg.MultiTenantTimeout > 0 {
		clientOpts = append(clientOpts, tmclient.WithTimeout(time.Duration(cfg.MultiTenantTimeout)*time.Second))
	}

	if cfg.MultiTenantCircuitBreakerThreshold > 0 {
		clientOpts = append(clientOpts,
			tmclient.WithCircuitBreaker(
				cfg.MultiTenantCircuitBreakerThreshold,
				time.Duration(cfg.MultiTenantCircuitBreakerTimeoutSec)*time.Second,
			),
		)
	}

	parsedURL, err := url.Parse(mtURL)
	if err != nil {
		return nil, fmt.Errorf("invalid MULTI_TENANT_URL: %w", err)
	}

	scheme := strings.ToLower(strings.TrimSpace(parsedURL.Scheme))
	if scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("MULTI_TENANT_URL must be an absolute URL with scheme and host")
	}

	switch scheme {
	case "https":
		return clientOpts, nil
	case "http":
		if !allowInsecureTenantManagerHTTP(cfg.EnvName) {
			return nil, fmt.Errorf("MULTI_TENANT_URL must use https outside local/development/test environments")
		}

		clientOpts = append(clientOpts, tmclient.WithAllowInsecureHTTP())
	default:
		return nil, fmt.Errorf("MULTI_TENANT_URL scheme must be http or https")
	}

	return clientOpts, nil
}

func allowInsecureTenantManagerHTTP(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "local", "development", "dev", "test", "testing":
		return true
	default:
		return false
	}
}

func redactedTenantManagerURL(raw string) string {
	parsedURL, err := url.Parse(raw)
	if err != nil {
		return "invalid-url"
	}

	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""

	return parsedURL.Redacted()
}

func buildMongoManagerOptions(cfg *Config, logger libLog.Logger) []tmmongo.Option {
	mongoOpts := []tmmongo.Option{
		tmmongo.WithModule(moduleName),
		tmmongo.WithLogger(logger),
	}

	if cfg.MultiTenantMaxTenantPools > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools))
	}

	if cfg.MultiTenantIdleTimeoutSec > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec)*time.Second))
	}

	if cfg.MultiTenantSettingsCheckIntervalSec > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithSettingsCheckInterval(
			time.Duration(cfg.MultiTenantSettingsCheckIntervalSec)*time.Second,
		))
	}

	return mongoOpts
}

func emitTenantMetric(ctx context.Context, telemetry *libOpentelemetry.Telemetry, logger libLog.Logger, metric metrics.Metric) {
	if telemetry == nil || telemetry.MetricsFactory == nil {
		return
	}

	counter, err := telemetry.MetricsFactory.Counter(metric)
	if err != nil {
		return
	}

	if err = counter.WithAttributes(
		attribute.String("service", in.ApplicationName),
		attribute.String("db", "mongodb"),
	).AddOne(ctx); err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("failed to increment metric %v: %v", metric, err))
	}
}

func wrapTenantMiddlewareWithMetrics(baseHandler fiber.Handler, telemetry *libOpentelemetry.Telemetry, logger libLog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := baseHandler(c)
		if err != nil {
			emitTenantMetric(c.UserContext(), telemetry, logger, utils.TenantConnectionErrorsTotal)
		}

		return err
	}
}
