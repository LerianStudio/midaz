package main

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/gen"
)

func main() {
	common.InitLocalEnvConfig()
	gen.InitializeService().Run()
}
