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

// @title						Midaz Tracer API
// @version						4.0.0
// @description					Midaz Tracer API — pre-flight transaction validation. Provides CEL-based rule evaluation, spending limits, two-phase reservations (hold / confirm / release), validation decisions, and a hash-chained audit trail.
// @tag.name					Rules
// @tag.description				CEL-based validation rules.
// @tag.name					Limits
// @tag.description				Spending limits and usage windows.
// @tag.name					Validations
// @tag.description				Pre-flight transaction validation decisions.
// @tag.name					Reservations
// @tag.description				Two-phase balance reservations: hold, then confirm or release.
// @tag.name					Audit
// @tag.description				Hash-chained audit event trail.
// @tag.name					Health
// @tag.description				Liveness and readiness probes.
// @tag.name					Info
// @tag.description				Service build and version metadata.
// @termsOfService				https://www.elastic.co/licensing/elastic-license
// @contact.name				Discord community
// @contact.url					https://discord.gg/DnhqKwkGv3
// @license.name				Elastic License 2.0
// @license.url					https://www.elastic.co/licensing/elastic-license
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
