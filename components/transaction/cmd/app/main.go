// Package main is the entry point for the Midaz transaction service.
//
// The transaction service is responsible for processing financial transactions,
// managing account balances, tracking operations, and enforcing double-entry
// accounting principles. It provides RESTful APIs and async processing for
// high-throughput transaction handling.
//
// The service follows hexagonal architecture with CQRS pattern, using PostgreSQL
// for transaction/balance storage, MongoDB for metadata, RabbitMQ for async
// processing, and Redis for caching and idempotency.
package main

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"
)

// @title			Midaz Transaction API
// @version		v1.48.0
// @description	This is a swagger documentation for the Midaz Transaction API
// @termsOfService	http://swagger.io/terms/
// @contact.name	Discord community
// @contact.url	https://discord.gg/DnhqKwkGv3
// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
// @host			localhost:3001
// @BasePath		/

// main is the entry point for the transaction service.
//
// This function:
// 1. Initializes local environment configuration
// 2. Calls InitServers to initialize all dependencies
// 3. Starts multiple concurrent servers (HTTP, RabbitMQ, Redis consumers)
// 4. Blocks until shutdown signal is received
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
