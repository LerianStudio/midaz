package bootstrap

import (
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mlog"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	*ServerGRPC
	mlog.Logger
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	pkg.NewLauncher(
		pkg.WithLogger(app.Logger),
		pkg.RunApp("HTTP server", app.Server),
		pkg.RunApp("gRPC server", app.ServerGRPC),
	).Run()
}
