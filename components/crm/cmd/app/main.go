// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/bootstrap"
)

// @title						CRM API
// @version					1.0.0
// @description				The CRM API provides a set of endpoints for managing holder data, including information related to their ledger accounts.
// @host						localhost:4003
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token authentication. Format: 'Bearer {access_token}'. Only required when auth plugin is enabled.
// @Security					BearerAuth
func main() {
	libCommons.InitLocalEnvConfig()

	logger, err := libZap.InitializeLoggerWithError()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)

		os.Exit(1)
	}

	service, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger: logger,
	})
	if err != nil {
		logger.Errorf("Failed to initialize CRM service: %v", err)
		_ = logger.Sync()

		os.Exit(1)
	}

	service.Run()
}
