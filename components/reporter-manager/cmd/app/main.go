// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"

	"github.com/LerianStudio/reporter/components/manager/internal/bootstrap"
)

// @title						Reporter
// @version					1.2.0
// @description				This is a swagger documentation for Reporter
// @termsOfService				http://swagger.io/terms/
// @host						localhost:4005
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				The authorization token in the 'Bearer access_token' format. Only required when auth plugin is enabled.
func main() {
	libCommons.InitLocalEnvConfig()

	svc, err := bootstrap.InitServers()
	if err != nil {
		// fmt.Fprintf is used here because the structured logger (zap) is not yet
		// available — it is initialized inside InitServers. This is the only place
		// where fmt output is acceptable per Ring standards.
		fmt.Fprintf(os.Stderr, "Failed to initialize manager: %v\n", err)
		os.Exit(1)
	}

	svc.Run()
}
