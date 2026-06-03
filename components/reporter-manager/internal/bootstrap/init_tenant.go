// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
	tmmiddleware "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	tmtc "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/gofiber/fiber/v2"
	goRedis "github.com/redis/go-redis/v9"
)

// tenantBypassPaths lists HTTP path prefixes that must bypass tenant resolution.
// Health, readiness, swagger, and version endpoints are infrastructure endpoints
// that do not carry a tenant JWT and must never be blocked by the tenant middleware.
// /readyz is the canonical readiness path (see ring:dev-readyz Gate 2).
var tenantBypassPaths = []string{
	"/health",
	"/readyz",
	"/swagger",
	"/version",
}

// initTenantMiddleware constructs the TenantMiddleware for the manager component.
//
// Returns nil (no-op) for the handler/cleanup return values in two cases:
//   - MultiTenantEnabled is false: single-tenant passthrough, zero performance impact.
//   - MultiTenantURL is empty: URL required to reach the Tenant Manager API.
//
// When enabled, it creates:
//  1. A Tenant Manager HTTP client pointing at cfg.MultiTenantURL with circuit breaker.
//  2. A MongoDB connection manager scoped to ApplicationName + ModuleManager.
//  3. A TenantCache + TenantLoader for event-driven tenant discovery.
//  4. An EventDispatcher + EventListener for Redis Pub/Sub tenant lifecycle events.
//  5. A TenantMiddleware with cache, loader, and MongoDB manager.
//
// The returned fiber.Handler wraps WithTenantDB with a bypass check so that
// /health, /readyz, /swagger, and /version skip tenant resolution entirely.
//
// The cleanup function MUST be called on shutdown to stop the event listener
// and close the dedicated Redis Pub/Sub client.
//
// The returned tmClient is the raw Tenant Manager client; callers thread it
// into the /readyz TenantManagerChecker for a nil-check (NOT an HTTP probe).
//
// The legacy readinessCheck closure is preserved in this signature for
// callers that still expect it (e.g. older tests). It returns nil when
// tmClient is configured.
func initTenantMiddleware(cfg *Config, logger log.Logger) (fiber.Handler, *tmclient.Client, func(context.Context) error, func(), error) {
	if !cfg.MultiTenantEnabled || cfg.MultiTenantURL == "" {
		return nil, nil, nil, nil, nil
	}

	tmClient, err := newTenantManagerClient(cfg, logger)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize tenant middleware client: %w", err)
	}

	mongoManager := tmmongo.NewManager(
		tmClient,
		constant.ApplicationName,
		tmmongo.WithModule(constant.ModuleManager),
		tmmongo.WithLogger(logger),
		tmmongo.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmmongo.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec)*time.Second),
	)

	// Event-driven tenant discovery: TenantCache + TenantLoader
	tenantCache := tmtc.NewTenantCache()
	cacheTTL := time.Duration(cfg.MultiTenantCacheTTLSec) * time.Second
	tenantLoader := tmtc.NewTenantLoader(tmClient, tenantCache, constant.ApplicationName, cacheTTL, logger)

	// EventDispatcher handles tenant lifecycle events (add, remove, update)
	dispatcher := tmevent.NewEventDispatcher(
		tenantCache, tenantLoader, constant.ApplicationName,
		tmevent.WithMongo(mongoManager),
		tmevent.WithDispatcherLogger(logger),
		tmevent.WithCacheTTL(cacheTTL),
	)

	// EventListener subscribes to Redis Pub/Sub for tenant events.
	// Uses MULTI_TENANT_REDIS_* with fallback to REDIS_HOST for backward compatibility.
	mtRedisClient, err := buildMultiTenantRedisClient(cfg)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to build multi-tenant Redis client: %w", err)
	}

	eventListener, err := tmevent.NewTenantEventListener(
		mtRedisClient, dispatcher.HandleEvent,
		tmevent.WithListenerLogger(logger),
		tmevent.WithService(constant.ApplicationName),
	)
	if err != nil {
		_ = mtRedisClient.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to create tenant event listener: %w", err)
	}

	if startErr := eventListener.Start(context.Background()); startErr != nil {
		_ = mtRedisClient.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to start tenant event listener: %w", startErr)
	}

	// TenantMiddleware with event-driven discovery (cache + loader + MongoDB manager)
	tenantMid := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithMB(mongoManager),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	// Readiness check verifies that the multi-tenant infrastructure is configured
	// (client + mongo manager exist). Per Ring multi-tenant standards, readiness
	// probes MUST NOT call Tenant Manager for tenant resolution — the previous
	// approach of calling GetDatabaseForTenant("__readiness_probe__") caused
	// WARN-level "tenant not found" log spam every 5s on every K8s probe.
	// If the Tenant Manager becomes unavailable, the circuit breaker on the client
	// will trip and fail-fast on real requests.
	readinessCheck := func(_ context.Context) error {
		// tmClient and mongoManager are captured by closure and were validated
		// during initialization — a nil check here guards against future refactors.
		if tmClient == nil {
			return fmt.Errorf("tenant manager client not initialized")
		}

		return nil
	}

	// Cleanup stops the event listener and closes the dedicated Redis client.
	cleanup := func() {
		logger.Log(context.Background(), log.LevelInfo, "Cleanup: stopping tenant event listener")

		if stopErr := eventListener.Stop(); stopErr != nil {
			logger.Log(context.Background(), log.LevelError, "Cleanup: failed to stop tenant event listener", log.Err(stopErr))
		}

		if closeErr := mtRedisClient.Close(); closeErr != nil {
			logger.Log(context.Background(), log.LevelError, "Cleanup: failed to close multi-tenant Redis client", log.Err(closeErr))
		}
	}

	handler := func(c *fiber.Ctx) error {
		// Bypass tenant resolution for infrastructure endpoints.
		// /swagger uses prefix match because it serves multiple sub-paths (e.g. /swagger/index.html).
		// All other bypass paths use exact match to prevent path-prefix bypass attacks.
		path := c.Path()

		for _, prefix := range tenantBypassPaths {
			if prefix == "/swagger" {
				if strings.HasPrefix(path, prefix) {
					return c.Next()
				}
			} else {
				if path == prefix {
					return c.Next()
				}
			}
		}

		return tenantMid.WithTenantDB(c)
	}

	return handler, tmClient, readinessCheck, cleanup, nil
}

// buildMultiTenantRedisClient creates a go-redis client for multi-tenant Pub/Sub.
// Uses MULTI_TENANT_REDIS_* env vars with fallback to REDIS_HOST for backward compatibility.
//
// When MULTI_TENANT_REDIS_HOST is set, it combines it with MULTI_TENANT_REDIS_PORT
// (defaulting to "6379"). When empty, falls back to REDIS_HOST which already contains
// host:port (e.g. "redis:6379").
func buildMultiTenantRedisClient(cfg *Config) (goRedis.UniversalClient, error) {
	var addr string

	if cfg.MultiTenantRedisHost != "" {
		port := cfg.MultiTenantRedisPort
		if port == "" {
			port = "6379"
		}

		addr = cfg.MultiTenantRedisHost + ":" + port
	} else {
		addr = cfg.RedisHost
	}

	password := cfg.MultiTenantRedisPassword
	if password == "" {
		password = cfg.RedisPassword
	}

	opts := &goRedis.Options{
		Addr:     addr,
		Password: password,
	}

	if cfg.MultiTenantRedisTLS {
		tlsCfg := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		if cfg.MultiTenantRedisCACert != "" {
			caCert, err := base64.StdEncoding.DecodeString(cfg.MultiTenantRedisCACert)
			if err != nil {
				return nil, fmt.Errorf("failed to base64-decode MULTI_TENANT_REDIS_CA_CERT: %w", err)
			}

			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("MULTI_TENANT_REDIS_CA_CERT contains no valid PEM certificates")
			}

			tlsCfg.RootCAs = pool
		}

		opts.TLSConfig = tlsCfg
	}

	return goRedis.NewClient(opts), nil
}

func newTenantManagerClient(cfg *Config, logger log.Logger) (*tmclient.Client, error) {
	var clientOpts []tmclient.ClientOption

	if cfg.MultiTenantCircuitBreakerThreshold > 0 {
		cbTimeout := time.Duration(cfg.MultiTenantCircuitBreakerTimeoutSec) * time.Second
		clientOpts = append(clientOpts,
			tmclient.WithCircuitBreaker(
				cfg.MultiTenantCircuitBreakerThreshold,
				cbTimeout,
			),
		)
	}

	clientOpts = append(clientOpts, tmclient.WithServiceAPIKey(cfg.MultiTenantServiceAPIKey))

	if cfg.MultiTenantTimeout > 0 {
		clientOpts = append(clientOpts, tmclient.WithTimeout(time.Duration(cfg.MultiTenantTimeout)*time.Second))
	}

	if cfg.MultiTenantCacheTTLSec > 0 {
		clientOpts = append(clientOpts, tmclient.WithCacheTTL(time.Duration(cfg.MultiTenantCacheTTLSec)*time.Second))
	}

	if cfg.MultiTenantAllowInsecureHTTP {
		clientOpts = append(clientOpts, tmclient.WithAllowInsecureHTTP())
	}

	return tmclient.NewClient(cfg.MultiTenantURL, logger, clientOpts...)
}
