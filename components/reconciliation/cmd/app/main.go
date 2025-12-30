// Package main is the entry point for the reconciliation service.
package main

import (
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/bootstrap"
)

func main() {
	libCommons.InitLocalEnvConfig()

	logger := libZap.InitializeLogger()

	service, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger: logger,
	})
	if err != nil {
		logger.Errorf("Failed to initialize reconciliation service: %v", err)
		_ = logger.Sync()

		os.Exit(1)
	}

	service.Run()
}
