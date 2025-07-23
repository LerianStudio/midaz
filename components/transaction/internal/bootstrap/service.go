package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	*MultiQueueConsumer
	*RedisQueueConsumer
	*StreamQueueConsumer
	libLog.Logger
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	libCommons.NewLauncher(
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Service", app.Server),
		libCommons.RunApp("Redis Queue Consumer", app.RedisQueueConsumer),
		//libCommons.RunApp("RabbitMQ Queue Consumer", app.MultiQueueConsumer),
		libCommons.RunApp("RabbitMQ Stream Consumer", app.StreamQueueConsumer),
	).Run()
}
