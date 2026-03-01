// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"

	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	httpin "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// Ports groups all external interface dependencies for the onboarding service.
// These are the "ports" in hexagonal architecture that connect to external systems
// or are exposed to other modules (like unified ledger mode).
type Ports struct {
	// MetadataPort is the MongoDB metadata repository for direct access in unified ledger mode.
	MetadataPort mbootstrap.MetadataIndexRepository
}

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	Logger libLog.Logger

	// Ports groups all external interface dependencies.
	Ports Ports

	// Route registration dependencies (for unified ledger mode)
	auth                *middleware.AuthClient
	accountHandler      *httpin.AccountHandler
	portfolioHandler    *httpin.PortfolioHandler
	ledgerHandler       *httpin.LedgerHandler
	assetHandler        *httpin.AssetHandler
	organizationHandler *httpin.OrganizationHandler
	segmentHandler      *httpin.SegmentHandler
	accountTypeHandler  *httpin.AccountTypeHandler

	telemetry          *libOpentelemetry.Telemetry
	postgresConnection *libPostgres.PostgresConnection
	mongoConnection    *libMongo.MongoConnection
	redisConnection    *libRedis.RedisConnection
	closeOnce          sync.Once
	closeErr           error
}

// Run starts the application.
// This is the only necessary code to run an app in main.go.
func (app *Service) Run() {
	libCommons.NewLauncher(
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Server", app.Server),
	).Run()

	if err := app.Close(); err != nil {
		app.Logger.Warnf("Onboarding service shutdown encountered errors: %v", err)
	}
}

// Close releases external resources created during service initialization.
func (app *Service) Close() error {
	if app == nil {
		return nil
	}

	app.closeOnce.Do(func() {
		var closeErrs []error

		if app.redisConnection != nil {
			if err := app.redisConnection.Close(); err != nil {
				closeErrs = append(closeErrs, err)
			}
		}

		if app.postgresConnection != nil && app.postgresConnection.ConnectionDB != nil {
			if err := (*app.postgresConnection.ConnectionDB).Close(); err != nil {
				closeErrs = append(closeErrs, err)
			}
		}

		if app.mongoConnection != nil && app.mongoConnection.DB != nil {
			if err := app.mongoConnection.DB.Disconnect(context.Background()); err != nil {
				closeErrs = append(closeErrs, err)
			}
		}

		if app.telemetry != nil {
			app.telemetry.ShutdownTelemetry()
		}

		app.closeErr = errors.Join(closeErrs...)
	})

	return app.closeErr
}

// GetRunnables returns all runnable components for composition in unified deployment.
// Implements mbootstrap.Service interface.
func (app *Service) GetRunnables() []mbootstrap.RunnableConfig {
	return []mbootstrap.RunnableConfig{
		{Name: "Onboarding Server", Runnable: app.Server},
	}
}

// GetRouteRegistrar returns a function that registers onboarding routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func (app *Service) GetRouteRegistrar() func(*fiber.App) {
	return func(fiberApp *fiber.App) {
		httpin.RegisterRoutesToApp(
			fiberApp,
			app.auth,
			app.accountHandler,
			app.portfolioHandler,
			app.ledgerHandler,
			app.assetHandler,
			app.organizationHandler,
			app.segmentHandler,
			app.accountTypeHandler,
		)
	}
}

// GetMetadataIndexPort returns the metadata index port for use by ledger in unified mode.
// This allows direct in-process calls for metadata index operations.
func (app *Service) GetMetadataIndexPort() mbootstrap.MetadataIndexRepository {
	return app.Ports.MetadataPort
}

// Ensure Service implements mbootstrap.Service interface at compile time.
var _ mbootstrap.Service = (*Service)(nil)
