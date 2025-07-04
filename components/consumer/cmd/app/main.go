package main

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/consumer/internal/bootstrap"
)

// @title			Midaz Consumer
// @version		v1.48.0
// @description	This is the Midaz Consumer service for processing RabbitMQ messages
// @termsOfService	http://swagger.io/terms/
// @contact.name	Discord community
// @contact.url	https://discord.gg/DnhqKwkGv3
// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitConsumer().Run()
}