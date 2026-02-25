// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"
)

// @title						Midaz Transaction API
// @version					v1.48.0
// @description				This is a swagger documentation for the Midaz Transaction API
// @termsOfService				http://swagger.io/terms/
// @contact.name				Discord community
// @contact.url				https://discord.gg/DnhqKwkGv3
// @license.name				Apache 2.0
// @license.url				http://www.apache.org/licenses/LICENSE-2.0.html
// @host						localhost:3001
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token authentication. Format: 'Bearer {access_token}'. Only required when auth plugin is enabled.
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
		logger.Errorf("Failed to initialize transaction service: %v", err)
		_ = logger.Sync()

		os.Exit(1)
	}

	service.Run()
}
