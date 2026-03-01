// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	tmmiddleware "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/mongo"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// initTenantMiddleware creates the tenant middleware for multi-tenant mode.
// Returns nil when multi-tenant is disabled or the URL is not configured.
// The middleware extracts tenantId from JWT, resolves the tenant-specific
// MongoDB connection via Tenant Manager, and injects it into the request context.
// When telemetry is non-nil, emits tenant_connections_total (on manager creation)
// and tenant_connection_errors_total (on handler errors).
func initTenantMiddleware(cfg *Config, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) (fiber.Handler, error) {
	if !cfg.MultiTenantEnabled {
		return nil, nil
	}

	mtURL := strings.TrimSpace(cfg.MultiTenantURL)
	if mtURL == "" {
		return nil, fmt.Errorf("MULTI_TENANT_URL must not be blank when MULTI_TENANT_ENABLED=true")
	}

	// Build client options
	var clientOpts []tmclient.ClientOption

	if cfg.MultiTenantTimeout > 0 {
		clientOpts = append(clientOpts,
			tmclient.WithTimeout(time.Duration(cfg.MultiTenantTimeout)*time.Second))
	}

	if cfg.MultiTenantCircuitBreakerThreshold > 0 {
		clientOpts = append(clientOpts,
			tmclient.WithCircuitBreaker(cfg.MultiTenantCircuitBreakerThreshold,
				time.Duration(cfg.MultiTenantCircuitBreakerTimeoutSec)*time.Second))
	}

	tmClient := tmclient.NewClient(mtURL, logger, clientOpts...)

	// Build mongo manager options
	var mongoOpts []tmmongo.Option

	mongoOpts = append(mongoOpts,
		tmmongo.WithModule(in.ApplicationName),
		tmmongo.WithLogger(logger),
	)

	if cfg.MultiTenantMaxTenantPools > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools))
	}

	if cfg.MultiTenantIdleTimeoutSec > 0 {
		mongoOpts = append(mongoOpts,
			tmmongo.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec)*time.Second))
	}

	mongoManager := tmmongo.NewManager(tmClient, in.ApplicationName, mongoOpts...)

	// Emit tenant_connections_total metric for MongoDB manager creation (connection pool setup).
	if telemetry != nil && telemetry.MetricsFactory != nil {
		telemetry.MetricsFactory.Counter(utils.TenantConnectionsTotal).
			WithAttributes(
				attribute.String("service", in.ApplicationName),
				attribute.String("db", "mongodb"),
			).
			AddOne(context.Background())
	}

	tenantMid := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithMongoManager(mongoManager),
	)

	logger.Infof("Multi-tenant middleware initialized: url=%s service=%s",
		mtURL, in.ApplicationName)

	baseHandler := tenantMid.WithTenantDB

	// Wrap handler to emit tenant_connection_errors_total on middleware errors.
	wrappedHandler := func(c *fiber.Ctx) error {
		err := baseHandler(c)
		if err != nil && telemetry != nil && telemetry.MetricsFactory != nil {
			telemetry.MetricsFactory.Counter(utils.TenantConnectionErrorsTotal).
				WithAttributes(
					attribute.String("service", in.ApplicationName),
					attribute.String("db", "mongodb"),
				).
				AddOne(c.UserContext())
		}

		return err
	}

	return wrappedHandler, nil
}
