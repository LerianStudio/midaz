package main

import (
	"github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/bootstrap"
)

func main() {
	commons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
