package main

import (
	"plugin-crm/v2/internal/bootstrap"
	"plugin-crm/v2/pkg"
)

// @title			Plugin CRM
// @version		1.0.0
// @description	The CRM API provides a set of endpoints for managing holder data, including information related to their ledger accounts.
// @host			localhost:4003
// @BasePath		/
func main() {
	pkg.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
