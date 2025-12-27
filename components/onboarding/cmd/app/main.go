// Package main is the entry point for the Midaz Onboarding service, which handles
// organization, ledger, account, asset, portfolio, and segment management operations.
package main

import (
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/bootstrap"
)

// @title						Midaz Onboarding API
// @version					v1.48.0
// @description				This is a swagger documentation for the Midaz Onboarding API
// @termsOfService				http://swagger.io/terms/
// @contact.name				Discord community
// @contact.url				https://discord.gg/DnhqKwkGv3
// @license.name				Apache 2.0
// @license.url				http://www.apache.org/licenses/LICENSE-2.0.html
// @host						localhost:3000
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token authentication. Format: 'Bearer {access_token}'. Only required when auth plugin is enabled.
func main() {
	libCommons.InitLocalEnvConfig()

	logger := libZap.InitializeLogger()

	service, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger: logger,
	})
	if err != nil {
		logger.Errorf("Failed to initialize onboarding service: %v", err)
		_ = logger.Sync()

		os.Exit(1)
	}

	service.Run()
}
