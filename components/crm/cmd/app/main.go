package main

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/bootstrap"
)

// @title			CRM API
// @version		1.0.0
// @description	The CRM API provides a set of endpoints for managing holder data, including information related to their ledger accounts.
// @host			localhost:4003
// @BasePath		/
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
