package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	Logger libLog.Logger
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

// Ensure Service implements mbootstrap.Service interface at compile time
var _ mbootstrap.Service = (*Service)(nil)
