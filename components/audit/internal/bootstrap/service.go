package bootstrap

import (
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mlog"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	*Consumer
	mlog.Logger
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	pkg.NewLauncher(
		pkg.WithLogger(app.Logger),
		pkg.RunApp("HTTP Service", app.Server),
		pkg.RunApp("Rabbit MQ Consumer", app.Consumer),
	).Run()
}
