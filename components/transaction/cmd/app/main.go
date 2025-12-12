package main

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"
)

//	@title						Midaz Transaction API
//	@version					v1.48.0
//	@description				This is a swagger documentation for the Midaz Transaction API
//	@termsOfService				http://swagger.io/terms/
//	@contact.name				Discord community
//	@contact.url				https://discord.gg/DnhqKwkGv3
//	@license.name				Apache 2.0
//	@license.url				http://www.apache.org/licenses/LICENSE-2.0.html
//	@host						localhost:3001
//	@BasePath					/
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Bearer token authentication. Format: 'Bearer {access_token}'. Only required when auth plugin is enabled.
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
