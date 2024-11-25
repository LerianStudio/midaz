package bootstrap

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
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
	common.NewLauncher(
		common.WithLogger(app.Logger),
		common.RunApp("HTTP server", app.Server),
		common.RunApp("gRPC server", app.ServerGRPC),
	).Run()
}
