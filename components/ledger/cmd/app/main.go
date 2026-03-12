// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
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

	logger, err := libZap.New(libZap.Config{
		Environment:     libZap.EnvironmentDevelopment,
		Level:           "info",
		OTelLibraryName: "midaz-ledger",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)

		os.Exit(1)
	}

	service, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger: logger,
	})
	if err != nil {
		logger.Log(context.Background(), libLog.LevelError, fmt.Sprintf("Failed to initialize ledger service: %v", err))
		_ = logger.Sync(context.Background())

		os.Exit(1)
	}

	service.Run()
}
