// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/reporter/internal/app"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-observability/log"
	libZap "github.com/LerianStudio/lib-observability/zap"
)

// @title						Reporter API
// @version					4.0.0
// @description				This is a swagger documentation for Reporter. The unified reporter binary serves the REST API (RUN_MODE=api) and/or the RabbitMQ report-generation worker (RUN_MODE=worker); RUN_MODE=all runs both in one process for local development.
// @termsOfService				http://swagger.io/terms/
// @host						localhost:4005
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				The authorization token in the 'Bearer access_token' format. Only required when auth plugin is enabled.
func main() {
	libCommons.InitLocalEnvConfig()

	mode, err := app.ParseRunMode(os.Getenv("RUN_MODE"))
	if err != nil {
		// fmt.Fprintf is used here because the structured logger is not yet
		// available — RUN_MODE is validated before any surface bootstrap runs.
		fmt.Fprintf(os.Stderr, "Failed to start reporter: %v\n", err)
		os.Exit(1)
	}

	logger, err := newOrchestratorLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	svc, err := app.InitService(mode, logger)
	if err != nil {
		logger.Log(context.Background(), libLog.LevelError, "Failed to initialize reporter service", libLog.Err(err))
		_ = logger.Sync(context.Background())

		os.Exit(1)
	}

	svc.Run()

	_ = logger.Sync(context.Background())
}

// newOrchestratorLogger builds the top-level logger the app orchestrator and
// shared launcher use for their own lifecycle lines. Each surface bootstrap
// still builds its own service logger internally; this one exists so the
// launcher and RUN_MODE wiring are observable before/around those.
func newOrchestratorLogger() (libLog.Logger, error) {
	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if logLevel == "" {
		logLevel = "info"
	}

	envName := strings.ToLower(strings.TrimSpace(os.Getenv("ENV_NAME")))
	if envName == "" {
		envName = "development"
	}

	otelServiceName := os.Getenv("OTEL_RESOURCE_SERVICE_NAME")
	if otelServiceName == "" {
		otelServiceName = "reporter"
	}

	return libZap.New(libZap.Config{
		Environment:     libZap.Environment(envName),
		Level:           logLevel,
		OTelLibraryName: otelServiceName,
	})
}
