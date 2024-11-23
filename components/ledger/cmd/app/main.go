package main

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/ledger/internal/bootstrap"
)

func main() {
	common.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
