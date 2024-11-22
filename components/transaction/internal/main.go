package main

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/gen"
)

// @title Midaz Transaction API
// @version 1.0
// @description This is a swagger documentation for the Midaz Transaction API
// @termsOfService http://swagger.io/terms/
// @contact.name Discord community
// @contact.url https://discord.gg/DnhqKwkGv3
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:3002
// @BasePath /
func main() {
	common.InitLocalEnvConfig()
	gen.InitializeService().Run()
}
