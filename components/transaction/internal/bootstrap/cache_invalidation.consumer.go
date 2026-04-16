// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/twmb/franz-go/pkg/kgo"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
)

// CacheInvalidationConsumer wraps a redpanda.CacheInvalidationRunner so it
// plugs into the libCommons.Launcher lifecycle (Run(*Launcher) error). The
// wrapper owns signal plumbing so a SIGINT/SIGTERM forces a clean shutdown
// that waits for in-flight record processing and commits the offset when
// safe.
type CacheInvalidationConsumer struct {
	runner *redpanda.CacheInvalidationRunner
	logger libLog.Logger
}

// NewCacheInvalidationConsumer builds the runner from service-scoped
// dependencies (config, logger, Redpanda seed brokers, Redis connection,
// telemetry). Returns (nil, nil) when the feature is disabled via config so
// callers can append to the runnable list with an unconditional nil check.
func NewCacheInvalidationConsumer(
	cfg *Config,
	logger libLog.Logger,
	seedBrokers []string,
	redisConn *libRedis.RedisConnection,
	telemetry *libOpentelemetry.Telemetry,
) (*CacheInvalidationConsumer, error) {
	if cfg == nil || logger == nil {
		return nil, nil
	}

	if !cfg.CacheInvalidationConsumerEnabled {
		logger.Info("CacheInvalidationConsumer disabled via CACHE_INVALIDATION_CONSUMER_ENABLED=false")
		return nil, nil
	}

	if len(seedBrokers) == 0 || redisConn == nil {
		logger.Warn("CacheInvalidationConsumer: missing seed brokers or Redis connection; consumer disabled")
		return nil, nil
	}

	securityCfg := redpanda.ClientSecurityConfig{
		TLSEnabled:            cfg.RedpandaTLSEnabled,
		TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
		TLSCAFile:             cfg.RedpandaTLSCAFile,
		SASLEnabled:           cfg.RedpandaSASLEnabled,
		SASLMechanism:         cfg.RedpandaSASLMechanism,
		SASLUsername:          cfg.RedpandaSASLUsername,
		SASLPassword:          cfg.RedpandaSASLPassword,
		Environment:           cfg.EnvName,
	}

	securityOptions, err := redpanda.BuildSecurityOptions(securityCfg)
	if err != nil {
		return nil, fmt.Errorf("cache-invalidation consumer: build security options: %w", err)
	}

	runnerCfg := redpanda.CacheInvalidationRunnerConfig{
		SeedBrokers:         seedBrokers,
		SecurityOptions:     securityOptions,
		ConsumerGroup:       strings.TrimSpace(cfg.CacheInvalidationConsumerGroup),
		SourceTopic:         strings.TrimSpace(cfg.CacheInvalidationTopic),
		RedisProvider:       redisConn,
		Logger:              logger,
		ShardCount:          cfg.RedisShardCount,
		MaxTransientRetries: cfg.CacheInvalidationMaxTransientRetries,
	}

	if telemetry != nil && telemetry.MetricProvider != nil {
		runnerCfg.Meter = telemetry.MetricProvider.Meter("midaz.transaction.cache_invalidation")
	}

	runner, err := redpanda.NewCacheInvalidationRunner(runnerCfg)
	if err != nil {
		return nil, fmt.Errorf("cache-invalidation consumer: %w", err)
	}

	return &CacheInvalidationConsumer{runner: runner, logger: logger}, nil
}

// Run blocks until the launcher returns or SIGINT/SIGTERM is received. On
// shutdown it invokes Close() on the runner so in-flight record processing
// can observe cancellation and commit offsets cleanly.
func (c *CacheInvalidationConsumer) Run(_ *libCommons.Launcher) error {
	if c == nil || c.runner == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c.logger.Info("CacheInvalidationConsumer starting")

	done := make(chan struct{})

	go func() {
		defer close(done)

		c.runner.Run(ctx)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}

	c.runner.Close()
	<-done

	c.logger.Info("CacheInvalidationConsumer stopped")

	return nil
}

// Close releases the underlying kgo client. Safe when called from
// resource_closer alongside other services.
func (c *CacheInvalidationConsumer) Close() error {
	if c == nil || c.runner == nil {
		return nil
	}

	c.runner.Close()

	return nil
}

// guard: ensure the kgo import is used when the SecurityOptions list is
// forwarded to the runner. Without this reference the import would be
// reported by goimports as unused in the narrow case where securityCfg
// produces a nil slice.
var _ = kgo.NewClient
