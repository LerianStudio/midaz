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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/bootstrap"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg"

	// automaxprocs side-effects GOMAXPROCS to honour Linux cgroup CPU quotas.
	// Without this, on cgroup-restricted hosts (Kubernetes, Docker with
	// --cpus flag) Go would default GOMAXPROCS to runtime.NumCPU() — the
	// host CPU count — which causes context-switch storms when the cgroup
	// quota is much smaller. The library logs the chosen value at startup.
	_ "go.uber.org/automaxprocs"
)

// @title						Tracer API
// @version						4.0.0
// @description					Transaction validation service with rules and limits
// @termsOfService				http://swagger.io/terms/
// @host						localhost:4020
// @schemes						http https
// @BasePath					/
// @securityDefinitions.apikey	ApiKeyAuth
// @in							header
// @name						X-API-Key
// @description					API Key for authentication
func main() {
	if err := run(); err != nil {
		// Print to stderr — logger may not be wired yet at this point.
		fmt.Fprintf(os.Stderr, "tracer: fatal: %v\n", err)
		os.Exit(1)
	}
}

// run is the actual entry point. Extracted from main so we can return an
// error and exit cleanly via os.Exit instead of panicking. Panics in main
// skip deferred functions in libraries that wired teardown via init() —
// returning is the safer pattern.
func run() error {
	pkg.InitLocalEnvConfig()

	// Root ctx is signal-aware so SIGINT/SIGTERM during bootstrap (migration,
	// DB connect, cache warmup) cancel startup immediately instead of hanging
	// until internal timeouts.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	service, err := bootstrap.InitServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize servers: %w", err)
	}

	service.Run()

	return nil
}
