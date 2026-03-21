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
	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmmiddleware "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// initTenantMiddleware creates the tenant middleware for multi-tenant mode.
// Returns nil when multi-tenant is disabled or the URL is not configured.
// The middleware extracts tenantId from JWT, resolves the tenant-specific
// MongoDB connection via Tenant Manager, and injects it into the request context.
// When telemetry is non-nil, emits tenant_connection_errors_total on handler errors.
func initTenantMiddleware(cfg *Config, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) (fiber.Handler, error) {
	if !cfg.MultiTenantEnabled {
		return nil, nil
	}

	mtURL := strings.TrimSpace(cfg.MultiTenantURL)
	if mtURL == "" {
		return nil, fmt.Errorf("MULTI_TENANT_URL must not be blank when MULTI_TENANT_ENABLED=true")
	}

	clientOpts, err := buildTenantClientOptions(cfg, mtURL)
	if err != nil {
		return nil, err
	}

	tmClient, err := tmclient.NewClient(mtURL, logger, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tenant manager client: %w", err)
	}

	mongoOpts := buildMongoManagerOptions(cfg, logger)

	mongoManager := tmmongo.NewManager(tmClient, cfg.ApplicationName, mongoOpts...)

	tenantMid := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithMongoManager(mongoManager),
	)

	logger.Log(context.Background(), libLog.LevelInfo, fmt.Sprintf("Multi-tenant middleware initialized: target=%s service=%s",
		redactedTenantManagerURL(mtURL), cfg.ApplicationName))

	return wrapTenantMiddlewareWithMetrics(tenantMid.WithTenantDB, telemetry, logger), nil
}

func buildTenantClientOptions(cfg *Config, mtURL string) ([]tmclient.ClientOption, error) {
	clientOpts := make([]tmclient.ClientOption, 0)

	clientOpts = append(clientOpts, tmclient.WithServiceAPIKey(cfg.MultiTenantServiceAPIKey))

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
		tmmongo.WithModule(in.ModuleName),
		tmmongo.WithLogger(logger),
	}

	if cfg.MultiTenantMaxTenantPools > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools))
	}

	if cfg.MultiTenantIdleTimeoutSec > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec)*time.Second))
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
