// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	libRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmconsumer "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/consumer"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	tmrabbitmq "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/rabbitmq"
	clog "github.com/LerianStudio/lib-observability/log"
)

// initMultiTenantManagers creates the Tenant Manager client and MongoDB manager when multi-tenant mode is enabled.
func initMultiTenantManagers(cfg *Config, logger clog.Logger) (*tmclient.Client, *tmmongo.Manager, error) {
	if !cfg.MultiTenantEnabled {
		return nil, nil, nil
	}

	if cfg.MultiTenantURL == "" {
		return nil, nil, fmt.Errorf("MULTI_TENANT_URL is required when MULTI_TENANT_ENABLED=true")
	}

	tmClient, err := newTenantManagerClient(cfg, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize tenant manager client: %w", err)
	}

	tenantMongoManager := tmmongo.NewManager(
		tmClient,
		pkgConstant.ApplicationName,
		tmmongo.WithModule(pkgConstant.ModuleWorker),
		tmmongo.WithLogger(logger),
		tmmongo.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmmongo.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec)*time.Second),
	)
	logger.Log(context.Background(), clog.LevelInfo, "Worker: tenant MongoDB manager initialized")

	return tmClient, tenantMongoManager, nil
}

// initTenantManagerClient creates a Tenant Manager HTTP client with circuit breaker.
// This is shared across MongoDB manager and MultiTenantConsumer to avoid duplicate instances.
func newTenantManagerClient(cfg *Config, logger clog.Logger) (*tmclient.Client, error) {
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

	client, err := tmclient.NewClient(cfg.MultiTenantURL, logger, clientOpts...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// initMultiTenantConsumer creates the tmconsumer.MultiTenantConsumer for per-tenant
// vhost isolation with lazy initialization.
func initMultiTenantConsumer(
	ctx context.Context,
	cfg *Config,
	logger clog.Logger,
	tenantMongoManager *tmmongo.Manager,
	tmClient *tmclient.Client,
) (*tmconsumer.MultiTenantConsumer, *tmrabbitmq.Manager, *libRedis.RedisConnection, func(), error) {
	if tmClient == nil {
		return nil, nil, nil, nil, fmt.Errorf("tenant manager client is required for multi-tenant consumer")
	}

	return initMultiTenantConsumerWithRedis(ctx, cfg, logger, tenantMongoManager, tmClient)
}

func initMultiTenantConsumerWithRedis(
	ctx context.Context,
	cfg *Config,
	logger clog.Logger,
	tenantMongoManager *tmmongo.Manager,
	tmClient *tmclient.Client,
) (*tmconsumer.MultiTenantConsumer, *tmrabbitmq.Manager, *libRedis.RedisConnection, func(), error) {
	// tmClient nil check is performed by the caller (initMultiTenantConsumer).

	// Application Redis for internal worker operations (Reconciler distributed locking,
	// tenant-aware key prefixing via GetKeyContext). Uses REDIS_HOST — NOT MULTI_TENANT_REDIS_*.
	// The MULTI_TENANT_REDIS_* env vars are reserved for the EventListener Pub/Sub connection
	// to tenant-manager (see TASK-019).
	redisConn := &libRedis.RedisConnection{
		Address:                      strings.Split(cfg.RedisHost, ","),
		MasterName:                   cfg.RedisMasterName,
		Password:                     cfg.RedisPassword,
		DB:                           cfg.RedisDB,
		Protocol:                     cfg.RedisProtocol,
		UseTLS:                       cfg.RedisTLS,
		CACert:                       cfg.RedisCACert,
		UseGCPIAMAuth:                cfg.RedisUseGCPIAM,
		ServiceAccount:               cfg.RedisServiceAccount,
		GoogleApplicationCredentials: cfg.GoogleApplicationCredentials,
		TokenLifeTime:                time.Duration(cfg.RedisTokenLifeTime) * time.Minute,
		RefreshDuration:              time.Duration(cfg.RedisTokenRefreshDuration) * time.Minute,
		Logger:                       logger,
	}

	rmqOpts := []tmrabbitmq.Option{
		tmrabbitmq.WithModule(pkgConstant.ModuleWorker),
		tmrabbitmq.WithLogger(logger),
		tmrabbitmq.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmrabbitmq.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec) * time.Second),
	}

	if cfg.RabbitMQTLS {
		rmqOpts = append(rmqOpts, tmrabbitmq.WithTLS())
	}

	rabbitMQManager := tmrabbitmq.NewManager(
		tmClient,
		pkgConstant.ApplicationName,
		rmqOpts...,
	)

	mtConfig := tmconsumer.DefaultMultiTenantConfig()
	mtConfig.Service = pkgConstant.ApplicationName
	mtConfig.Environment = cfg.MultiTenantEnvironment
	mtConfig.MultiTenantURL = cfg.MultiTenantURL
	mtConfig.ServiceAPIKey = cfg.MultiTenantServiceAPIKey
	mtConfig.AllowInsecureHTTP = cfg.MultiTenantAllowInsecureHTTP
	mtConfig.PrefetchCount = pkgConstant.DefaultPrefetchCount

	consumerOpts := []tmconsumer.Option{
		tmconsumer.WithRabbitMQ(rabbitMQManager),
	}
	if tenantMongoManager != nil {
		consumerOpts = append(consumerOpts, tmconsumer.WithMongoManager(tenantMongoManager))
	}

	mtConsumer, err := tmconsumer.NewMultiTenantConsumerWithError(
		mtConfig,
		logger,
		consumerOpts...,
	)
	if err != nil {
		closeMultiTenantBootstrapResources(logger, nil, rabbitMQManager, redisConn)
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize multi-tenant consumer: %w", err)
	}

	logger.Log(ctx, clog.LevelInfo, "MultiTenantConsumer initialized with per-tenant vhost isolation")

	cleanup := func() {
		closeMultiTenantBootstrapResources(logger, nil, rabbitMQManager, redisConn)
	}

	return mtConsumer, rabbitMQManager, redisConn, cleanup, nil
}

func closeMultiTenantBootstrapResources(
	logger clog.Logger,
	mtConsumer *tmconsumer.MultiTenantConsumer,
	rabbitMQManager *tmrabbitmq.Manager,
	redisConn *libRedis.RedisConnection,
) {
	if mtConsumer != nil {
		logger.Log(context.Background(), clog.LevelInfo, "Cleanup: closing multi-tenant consumer")

		if closeErr := mtConsumer.Close(); closeErr != nil {
			logger.Log(context.Background(), clog.LevelError, "Cleanup: failed to close multi-tenant consumer", clog.Err(closeErr))
		}
	}

	if rabbitMQManager != nil {
		logger.Log(context.Background(), clog.LevelInfo, "Cleanup: closing multi-tenant RabbitMQ manager")

		if closeErr := rabbitMQManager.Close(context.Background()); closeErr != nil {
			logger.Log(context.Background(), clog.LevelError, "Cleanup: failed to close multi-tenant RabbitMQ manager", clog.Err(closeErr))
		}
	}

	if redisConn != nil {
		logger.Log(context.Background(), clog.LevelInfo, "Cleanup: closing Redis connection")

		if closeErr := redisConn.Close(); closeErr != nil {
			logger.Log(context.Background(), clog.LevelError, "Cleanup: failed to close Redis connection", clog.Err(closeErr))
		}
	}
}
