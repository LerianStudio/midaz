package main

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/bootstrap"
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
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
