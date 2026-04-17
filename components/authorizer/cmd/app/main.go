// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package main is the entry point for the authorizer service.
// The real bootstrap wiring lives in components/authorizer/internal/bootstrap;
// this file is intentionally thin so it can be exercised with hermetic unit
// tests via injected dependencies. Follows the same pattern used by the
// consumer binary's main.go.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/bootstrap"
)

// telemetryHandle is the minimal contract main() needs from the telemetry
// object returned by bootstrap.InitTelemetry. Exists so tests can substitute
// a fake that records ShutdownTelemetry calls without booting a real OTEL
// exporter.
type telemetryHandle interface {
	ShutdownTelemetry()
}

// deps bundles every side-effectful constructor main() depends on. realDeps()
// wires the production implementations; tests inject fakes to drive every
// success and failure branch without touching the network, filesystem, or OTEL
// collectors.
type deps struct {
	initEnvConfig  func()
	initLogger     func() (libLog.Logger, error)
	loadConfig     func() (*bootstrap.Config, error)
	initTelemetry  func(cfg *bootstrap.Config, logger libLog.Logger) (telemetryHandle, error)
	notifyContext  func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc)
	runBootstrap   func(ctx context.Context, cfg *bootstrap.Config, logger libLog.Logger, telemetry telemetryHandle) error
}

// realDeps wires the production constructors. InitLocalEnvConfig is void in
// lib-commons, so we preserve its fire-and-forget contract rather than invent
// error paths. The telemetry closure unboxes the concrete *Telemetry into our
// telemetryHandle interface, and the run closure casts the handle back when
// forwarding to bootstrap.Run (which takes the concrete type).
func realDeps() deps {
	return deps{
		initEnvConfig: func() { _ = libCommons.InitLocalEnvConfig() },
		initLogger:    libZap.InitializeLoggerWithError,
		loadConfig:    bootstrap.LoadConfig,
		initTelemetry: func(cfg *bootstrap.Config, logger libLog.Logger) (telemetryHandle, error) {
			t, err := bootstrap.InitTelemetry(cfg, logger)
			if err != nil {
				return nil, err
			}

			return t, nil
		},
		notifyContext: signal.NotifyContext,
		runBootstrap: func(ctx context.Context, cfg *bootstrap.Config, logger libLog.Logger, telemetry telemetryHandle) error {
			// telemetry is always the concrete *libOpentelemetry.Telemetry in
			// production; assert it so we can call the existing Run signature
			// unchanged. Tests inject fakes and never reach this path.
			concrete, _ := telemetry.(*libOpentelemetry.Telemetry)
			return bootstrap.Run(ctx, cfg, logger, concrete)
		},
	}
}

// run performs the authorizer bootstrap sequence and returns a process exit
// code. Behavior must remain identical to the pre-refactor main():
//   - env config init, then logger init, then config load, then telemetry init
//   - each failure logs to the logger (or stderr before the logger exists) and
//     exits with code 1
//   - telemetry shutdown is deferred immediately after successful init
//   - SIGINT/SIGTERM are registered before calling into bootstrap.Run
//   - logger.Sync is best-effort flushed on every error path after the logger
//     exists (preserves the original flow)
func run(stderr io.Writer, d deps) int {
	d.initEnvConfig()

	logger, err := d.initLogger()
	if err != nil {
		fmt.Fprintf(stderr, "failed to initialize logger: %v\n", err)
		return 1
	}

	cfg, err := d.loadConfig()
	if err != nil {
		logger.Errorf("Failed to load authorizer config: %v", err)
		_ = logger.Sync()

		return 1
	}

	telemetry, err := d.initTelemetry(cfg, logger)
	if err != nil {
		logger.Errorf("Failed to initialize authorizer telemetry: %v", err)
		_ = logger.Sync()

		return 1
	}

	defer telemetry.ShutdownTelemetry()

	ctx, stop := d.notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := d.runBootstrap(ctx, cfg, logger, telemetry); err != nil {
		logger.Errorf("Authorizer exited with error: %v", err)
		_ = logger.Sync()

		return 1
	}

	return 0
}

func main() {
	os.Exit(run(os.Stderr, realDeps()))
}
