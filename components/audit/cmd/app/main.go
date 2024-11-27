package main

import (
	"context"
	"github.com/LerianStudio/midaz/components/audit/internal/bootstrap"
	"github.com/LerianStudio/midaz/pkg"
)

func main() {
	ctx := context.Background()

	pkg.InitLocalEnvConfig()
	uc, logger, telemetry := bootstrap.InitServers()

	telemetry.InitializeTelemetry(logger)
	defer telemetry.ShutdownTelemetry()

	defer func() {
		if err := logger.Sync(); err != nil {
			logger.Infof("Launcher: App (%s) error:", bootstrap.ApplicationName)
			logger.Infof("Failed to sync logger: %s", err)
			logger.Fatalf("\u001b[31m%s\u001b[0m", err)

		}
	}()

	logger.Infof("Launcher: App (%s) finished\n", bootstrap.ApplicationName)

	var obj string

	message := make(chan string)

	uc.RabbitMQRepo.ConsumerDefault(message)

	obj = <-message

	logger.Info(obj)

	treeID, err := uc.CreateAuditTree(ctx, pkg.GenerateUUIDv7().String(), pkg.GenerateUUIDv7().String())
	if err != nil {
		logger.Fatalf("Failed to run the server: %s", err)
	}

	logger.Infof("Tree ID: %v", treeID)
}
