package bootstrap

import (
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mlog"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	*MultiQueueConsumer
	*CronConsumer
	mlog.Logger
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	pkg.NewLauncher(
		pkg.WithLogger(app.Logger),
		pkg.RunApp("Server Apis", app.Server),
		pkg.RunApp("RabbitMQ Consumer", app.MultiQueueConsumer),
		pkg.RunApp("Cron Consumer", app.CronConsumer),
	).Run()
}
