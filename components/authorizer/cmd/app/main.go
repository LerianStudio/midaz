// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/bootstrap"
)

func main() {
	libCommons.InitLocalEnvConfig()

	logger, err := libZap.InitializeLoggerWithError()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		logger.Errorf("Failed to load authorizer config: %v", err)
		_ = logger.Sync()
		os.Exit(1)
	}

	telemetry, err := bootstrap.InitTelemetry(cfg, logger)
	if err != nil {
		logger.Errorf("Failed to initialize authorizer telemetry: %v", err)
		_ = logger.Sync()
		os.Exit(1)
	}
	defer telemetry.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Run(ctx, cfg, logger, telemetry); err != nil {
		logger.Errorf("Authorizer exited with error: %v", err)
		_ = logger.Sync()
		os.Exit(1)
	}
}
