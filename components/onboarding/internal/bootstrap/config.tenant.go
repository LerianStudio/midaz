// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	tmmiddleware "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/postgres"
	"github.com/gofiber/fiber/v2"
)

// initTenantMiddleware creates the tenant middleware for standalone multi-tenant mode.
// Returns nil when multi-tenant is disabled, running in unified mode, or the URL is not configured.
// The middleware extracts tenantId from JWT, resolves the tenant-specific PostgreSQL and MongoDB
// connections via Tenant Manager, and injects them into the request context.
func initTenantMiddleware(opts *Options, cfg *Config, logger libLog.Logger) (fiber.Handler, error) {
	// In unified mode, the ledger's MultiPoolMiddleware handles tenant resolution.
	if opts != nil && opts.MultiTenantEnabled {
		return nil, nil
	}

	if !cfg.MultiTenantEnabled || cfg.MultiTenantURL == "" {
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

	if cfg.MultiTenantRetryMax > 0 {
		clientOpts = append(clientOpts,
			tmclient.WithCircuitBreaker(cfg.MultiTenantRetryMax,
				time.Duration(cfg.MultiTenantRetryDelay)*time.Second))
	}

	tmClient := tmclient.NewClient(mtURL, logger, clientOpts...)

	// Build PostgreSQL manager options
	pgManager := tmpostgres.NewManager(tmClient, ApplicationName,
		tmpostgres.WithModule(ApplicationName),
		tmpostgres.WithLogger(logger),
	)

	// Build MongoDB manager options
	var mongoOpts []tmmongo.Option

	mongoOpts = append(mongoOpts,
		tmmongo.WithModule(ApplicationName),
		tmmongo.WithLogger(logger),
	)

	if cfg.MultiTenantCacheSize > 0 {
		mongoOpts = append(mongoOpts, tmmongo.WithMaxTenantPools(cfg.MultiTenantCacheSize))
	}

	if cfg.MultiTenantCacheTTL > 0 {
		mongoOpts = append(mongoOpts,
			tmmongo.WithIdleTimeout(time.Duration(cfg.MultiTenantCacheTTL)*time.Second))
	}

	mongoManager := tmmongo.NewManager(tmClient, ApplicationName, mongoOpts...)

	tenantMid := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithPostgresManager(pgManager),
		tmmiddleware.WithMongoManager(mongoManager),
	)

	logger.Infof("Multi-tenant middleware initialized (standalone): url=%s service=%s",
		mtURL, ApplicationName)

	return tenantMid.WithTenantDB, nil
}
