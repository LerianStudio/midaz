// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/LerianStudio/lib-commons/v7/commons/buildinfo"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libZap "github.com/LerianStudio/lib-observability/v4/zap"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/bootstrap"
)

// Filled at build time with -ldflags "-X main.version=... -X main.revision=...
// -X main.buildTime=...". Empty in a local build, where buildinfo falls back to
// the Go VCS stamp and then to dev/unknown.
var version, revision, buildTime string

func main() {
	buildinfo.Set(buildinfo.Build{Version: version, Revision: revision, BuildTime: buildTime})
	buildinfo.HandleFlag() // "--version" prints the identity as JSON and exits 0

	libCommons.InitLocalEnvConfig()

	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if logLevel == "" {
		logLevel = "info"
	}

	envName := strings.ToLower(strings.TrimSpace(os.Getenv("ENV_NAME")))
	if envName == "" {
		envName = "development"
	}

	otelServiceName := os.Getenv("OTEL_RESOURCE_SERVICE_NAME")
	if otelServiceName == "" {
		otelServiceName = "ledger"
	}

	logger, err := libZap.New(libZap.Config{
		Environment:     libZap.Environment(envName),
		Level:           logLevel,
		OTelLibraryName: otelServiceName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)

		os.Exit(1)
	}

	service, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
		Logger: logger,
	})
	if err != nil {
		logger.Log(context.Background(), libLog.LevelError, "Failed to initialize ledger service", libLog.Err(err))
		_ = logger.Sync(context.Background())

		os.Exit(1)
	}

	service.Run()

	_ = logger.Sync(context.Background())
}
