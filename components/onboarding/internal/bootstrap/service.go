package bootstrap

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	httpin "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/gofiber/fiber/v2"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	Logger libLog.Logger

	// Route registration dependencies (for unified ledger mode)
	auth               *middleware.AuthClient
	accountHandler     *httpin.AccountHandler
	portfolioHandler   *httpin.PortfolioHandler
	ledgerHandler      *httpin.LedgerHandler
	assetHandler       *httpin.AssetHandler
	organizationHandler *httpin.OrganizationHandler
	segmentHandler     *httpin.SegmentHandler
	accountTypeHandler *httpin.AccountTypeHandler
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	libCommons.NewLauncher(
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Server", app.Server),
	).Run()
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

// Ensure Service implements mbootstrap.Service interface at compile time
var _ mbootstrap.Service = (*Service)(nil)
