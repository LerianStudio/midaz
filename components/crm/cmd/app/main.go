// Package main is the entry point for the CRM API server.
package main

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
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
	bootstrap.InitServers().Run()
}
