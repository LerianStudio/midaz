package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
)

// ConsumerService is the application glue where we put all top level components to be used.
type ConsumerService struct {
	*MultiQueueConsumer
	libLog.Logger
}

// Run starts the app service.
func (app *ConsumerService) Run() {
	libCommons.NewLauncher(
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("RabbitMQ Consumer", app.MultiQueueConsumer),
	).Run()
}
