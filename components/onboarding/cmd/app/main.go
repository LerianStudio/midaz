// Package main is the entry point for the Midaz onboarding service.
//
// The onboarding service is responsible for managing the lifecycle of organizations,
// ledgers, accounts, assets, portfolios, segments, and account types. It provides
// RESTful APIs for creating, reading, updating, and deleting these entities.
//
// The service follows hexagonal architecture with CQRS pattern, using PostgreSQL
// for entity storage, MongoDB for metadata, and RabbitMQ for async messaging.
package main

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/bootstrap"
)

// @title			Midaz Onboarding API
// @version		v1.48.0
// @description	This is a swagger documentation for the Midaz Ledger API
// @termsOfService	http://swagger.io/terms/
// @contact.name	Discord community
// @contact.url	https://discord.gg/DnhqKwkGv3
// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
// @host			localhost:3000
// @BasePath		/

// main is the entry point for the onboarding service.
//
// This function:
// 1. Initializes local environment configuration
// 2. Calls InitServers to initialize all dependencies
// 3. Starts the HTTP server with graceful shutdown
// 4. Blocks until shutdown signal is received
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
