package main

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/bootstrap"
)

// @title			Midaz Ledger API
// @version		v1.48.0
// @description	This is a swagger documentation for the Midaz Ledger API (unified onboarding + transaction)
// @termsOfService	http://swagger.io/terms/
// @contact.name	Discord community
// @contact.url	https://discord.gg/DnhqKwkGv3
// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
// @host			localhost:3000
// @BasePath		/
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
