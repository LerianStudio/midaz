// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v5/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/bootstrap"
)

// @title						CRM API
// @version					v3.7.0
// @description				The CRM API provides a set of endpoints for managing holder data, including information related to their ledger accounts.
// @host						localhost:4003
// @BasePath					/
// @schemes					http
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token authentication. Format: 'Bearer {access_token}'. Only required when auth plugin is enabled.
func main() {
	libCommons.InitLocalEnvConfig()

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
		otelServiceName = "crm"
	}

	logger, err := libZap.New(libZap.Config{
		Environment:     libZap.Environment(envName),
		Level:           logLevel,
		OTelLibraryName: otelServiceName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)

		os.Exit(1)
	}

	service, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger: logger,
	})
	if err != nil {
		logger.Log(context.Background(), libLog.LevelError, fmt.Sprintf("Failed to initialize CRM service: %v", err))
		_ = logger.Sync(context.Background())

		os.Exit(1)
	}

	service.Run()

	_ = logger.Sync(context.Background())
}
