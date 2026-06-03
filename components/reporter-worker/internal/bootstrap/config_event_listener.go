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

	pkgConstant "github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmconsumer "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/consumer"
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	tmrabbitmq "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/rabbitmq"
	tmtc "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/tenantcache"
	clog "github.com/LerianStudio/lib-observability/log"
	goRedis "github.com/redis/go-redis/v9"
)

// initEventListener creates an EXTERNAL EventDispatcher + EventListener for tenant
// lifecycle events via Redis Pub/Sub. This follows the two-dispatcher pattern from
// the midaz ledger: the MultiTenantConsumer keeps its internal dispatcher (cache_A +
// loader_A), while this external dispatcher (cache_B + loader_B) receives Redis events
// and calls consumer PUBLIC methods (EnsureConsumerStarted, StopConsumer).
//
// MUST NOT use WithEventDispatcher() — see cache divergence bug in lib-commons where
// wireDispatcherCallbacks() swaps c.cache but c.loader keeps pointing to original cache.
//
// Returns nil cleanup when MULTI_TENANT_REDIS_HOST is empty (graceful degradation:
// worker runs with lazy-load only, no Pub/Sub events).
func initEventListener(
	cfg *Config,
	logger clog.Logger,
	tmClient *tmclient.Client,
	mtConsumer *tmconsumer.MultiTenantConsumer,
	tenantMongoManager *tmmongo.Manager,
	rabbitMQManager *tmrabbitmq.Manager,
) (func(), error) {
	redisHost := strings.TrimSpace(cfg.MultiTenantRedisHost)
	if redisHost == "" {
		logger.Log(context.Background(), clog.LevelWarn,
			"MULTI_TENANT_REDIS_HOST not configured; tenant event listener will NOT start "+
				"(tenants discovered via lazy-load only, no Pub/Sub events)")

		return nil, nil
	}

	// Create external TenantCache + TenantLoader (separate from consumer's internal ones).
	// This is cache_B + loader_B in the two-dispatcher pattern.
	tenantCache := tmtc.NewTenantCache()
	cacheTTL := time.Duration(cfg.MultiTenantCacheTTLSec) * time.Second

	tenantLoader := tmtc.NewTenantLoader(
		tmClient,
		tenantCache,
		pkgConstant.ApplicationName,
		cacheTTL,
		logger,
	)

	// Build dispatcher options with callbacks that call consumer PUBLIC methods.
	dispatcherOpts := buildDispatcherOptions(cfg, logger, tmClient, mtConsumer, tenantMongoManager, rabbitMQManager, cacheTTL)

	dispatcher := tmevent.NewEventDispatcher(
		tenantCache,
		tenantLoader,
		pkgConstant.ApplicationName,
		dispatcherOpts...,
	)

	// SetOnTenantLoaded ensures that when a tenant is lazy-loaded through the external
	// loader (e.g., triggered by an event for an unknown tenant), the consumer starts
	// processing messages for that tenant. This is the restart recovery path.
	tenantLoader.SetOnTenantLoaded(func(ctx context.Context, tenantID string) {
		mtConsumer.EnsureConsumerStarted(ctx, tenantID)
	})

	// Create Redis Pub/Sub client using MULTI_TENANT_REDIS_* env vars.
	// This is a SEPARATE Redis connection from the application Redis (REDIS_HOST).
	mtRedisClient, err := buildMultiTenantRedisClientForWorker(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build multi-tenant Redis client: %w", err)
	}

	eventListener, err := tmevent.NewTenantEventListener(
		mtRedisClient,
		dispatcher.HandleEvent,
		tmevent.WithListenerLogger(logger),
		tmevent.WithService(pkgConstant.ApplicationName),
	)
	if err != nil {
		_ = mtRedisClient.Close()
		return nil, fmt.Errorf("failed to create tenant event listener: %w", err)
	}

	if startErr := eventListener.Start(context.Background()); startErr != nil {
		_ = mtRedisClient.Close()
		return nil, fmt.Errorf("failed to start tenant event listener: %w", startErr)
	}

	logger.Log(context.Background(), clog.LevelInfo, "Tenant event listener started for worker",
		clog.String("redis_host", redisHost),
		clog.String("service", pkgConstant.ApplicationName),
	)

	// Cleanup stops the listener goroutine and closes the dedicated Redis client.
	cleanup := func() {
		logger.Log(context.Background(), clog.LevelInfo, "Cleanup: stopping worker tenant event listener")

		if stopErr := eventListener.Stop(); stopErr != nil {
			logger.Log(context.Background(), clog.LevelError, "Cleanup: failed to stop tenant event listener", clog.Err(stopErr))
		}

		if closeErr := mtRedisClient.Close(); closeErr != nil {
			logger.Log(context.Background(), clog.LevelError, "Cleanup: failed to close multi-tenant Redis client", clog.Err(closeErr))
		}
	}

	return cleanup, nil
}

// performInitialTenantSync fetches all active tenants from the Tenant Manager API
// and calls EnsureConsumerStarted for each one. This ensures the worker starts
// consuming messages for all known tenants immediately on startup, rather than
// waiting for a Redis Pub/Sub event or lazy-load trigger.
//
// Non-blocking: logs a warning and returns on failure. The worker will still
// discover tenants via events or lazy-load.
func performInitialTenantSync(
	ctx context.Context,
	logger clog.Logger,
	tmClient *tmclient.Client,
	mtConsumer *tmconsumer.MultiTenantConsumer,
) {
	if tmClient == nil {
		logger.Log(ctx, clog.LevelWarn,
			"Initial tenant sync skipped: tenant manager client is nil")

		return
	}

	tenants, err := tmClient.GetActiveTenantsByService(ctx, pkgConstant.ApplicationName)
	if err != nil {
		logger.Log(ctx, clog.LevelWarn,
			"Initial tenant sync failed; tenants will be discovered via events or lazy-load",
			clog.Err(err))

		return
	}

	for _, t := range tenants {
		logger.Log(ctx, clog.LevelDebug, "Initial tenant sync: starting consumer",
			clog.String("tenant_id", t.ID),
			clog.String("tenant_name", t.Name),
			clog.String("tenant_status", t.Status))

		mtConsumer.EnsureConsumerStarted(ctx, t.ID)
	}

	logger.Log(ctx, clog.LevelInfo, "Initial tenant sync completed",
		clog.Int("tenant_count", len(tenants)))
}

// buildDispatcherOptions constructs the EventDispatcher options with callbacks
// following the midaz ledger two-dispatcher pattern:
//
// OnTenantAdded: InvalidateConfig -> EnsureConsumerStarted
// OnTenantRemoved: StopConsumer -> CloseConnection (Mongo, RabbitMQ) -> InvalidateConfig
func buildDispatcherOptions(
	_ *Config,
	logger clog.Logger,
	tmClient *tmclient.Client,
	mtConsumer *tmconsumer.MultiTenantConsumer,
	tenantMongoManager *tmmongo.Manager,
	rabbitMQManager *tmrabbitmq.Manager,
	cacheTTL time.Duration,
) []tmevent.DispatcherOption {
	tenantServiceName := pkgConstant.ApplicationName

	opts := []tmevent.DispatcherOption{
		tmevent.WithDispatcherLogger(logger),
		tmevent.WithCacheTTL(cacheTTL),
		tmevent.WithOnTenantAdded(func(ctx context.Context, tenantID string) {
			// Invalidate tmClient cache so next lazy-load fetches fresh config.
			if tmClient != nil {
				_ = tmClient.InvalidateConfig(ctx, tenantID, tenantServiceName)
			}

			// Start consumer for the new/reactivated tenant.
			mtConsumer.EnsureConsumerStarted(ctx, tenantID)

			logger.Log(ctx, clog.LevelInfo, "tenant added: consumer started",
				clog.String("tenant_id", tenantID))
		}),
		tmevent.WithOnTenantRemoved(func(ctx context.Context, tenantID string) {
			// Stop consumer FIRST — prevents message processing during teardown.
			mtConsumer.StopConsumer(tenantID)

			// Close Mongo connection for this tenant (if manager provided).
			if tenantMongoManager != nil {
				if err := tenantMongoManager.CloseConnection(ctx, tenantID); err != nil {
					logger.Log(ctx, clog.LevelWarn, "failed to close Mongo connection for removed tenant",
						clog.String("tenant_id", tenantID), clog.Err(err))
				}
			}

			// Close RabbitMQ connection for this tenant (if manager provided).
			if rabbitMQManager != nil {
				if err := rabbitMQManager.CloseConnection(ctx, tenantID); err != nil {
					logger.Log(ctx, clog.LevelWarn, "failed to close RabbitMQ connection for removed tenant",
						clog.String("tenant_id", tenantID), clog.Err(err))
				}
			}

			// Invalidate tmClient cache LAST so lazy-load fetches fresh state.
			if tmClient != nil {
				if err := tmClient.InvalidateConfig(ctx, tenantID, tenantServiceName); err != nil {
					logger.Log(ctx, clog.LevelWarn, "failed to invalidate tenant config cache",
						clog.String("tenant_id", tenantID), clog.Err(err))
				}
			}

			logger.Log(ctx, clog.LevelInfo, "tenant evicted: consumer stopped, connections closed, cache invalidated",
				clog.String("tenant_id", tenantID))
		}),
	}

	// Wire infrastructure managers for the dispatcher's built-in connection tracking.
	if tenantMongoManager != nil {
		opts = append(opts, tmevent.WithMongo(tenantMongoManager))
	}

	if rabbitMQManager != nil {
		opts = append(opts, tmevent.WithRabbitMQ(rabbitMQManager))
	}

	return opts
}

// buildMultiTenantRedisClientForWorker creates a go-redis client for multi-tenant Pub/Sub
// in the worker component. Uses MULTI_TENANT_REDIS_* env vars.
//
// This is the same pattern as the manager's buildMultiTenantRedisClient (init_tenant.go)
// but without the fallback to REDIS_HOST — the worker explicitly requires
// MULTI_TENANT_REDIS_HOST for Pub/Sub. When empty, initEventListener returns nil
// (graceful degradation) before reaching this function.
func buildMultiTenantRedisClientForWorker(cfg *Config) (goRedis.UniversalClient, error) {
	port := cfg.MultiTenantRedisPort
	if port == "" {
		port = "6379"
	}

	addr := cfg.MultiTenantRedisHost + ":" + port

	opts := &goRedis.Options{
		Addr:     addr,
		Password: cfg.MultiTenantRedisPassword,
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
