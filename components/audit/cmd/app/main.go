package main

import (
	"github.com/LerianStudio/midaz/components/audit/internal/bootstrap"
	"github.com/LerianStudio/midaz/pkg"
)

func main() {
	pkg.InitLocalEnvConfig()
	service, rabbitmq := bootstrap.InitServers()
	go rabbitmq.ConsumerAudit()
	service.Run()
}
