// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Command migrate-cel-rules is the one-shot "000023" release-step job that
// rewrites the GLOBAL currency reference to asset in every stored CEL rule
// expression.
//
// It lives OUTSIDE the numbered golang-migrate .sql sequence (the last .sql
// stays 000022) because the rewrite operates on rule TEXT, not schema, and
// needs the CEL rewriter and the new (asset) environment.
//
// SINGLE-TENANT ONLY: the job resolves rules through the static connection and
// has no per-tenant fan-out, so it refuses to run under MULTI_TENANT_ENABLED=true
// (exiting non-zero) rather than half-migrate a multi-tenant deployment.
// Per-tenant execution is a documented follow-up.
//
// ROLLOUT (stop-the-world / drained): there is NO currency->asset alias, so the
// CEL env variable and the stored rules must BOTH be asset before any instance
// serves traffic. Run this job in the same coordinated, drained release as the
// schema rename, after traffic is stopped and before instances come back up.
//
// UP (default, or --up): atomic. One transaction write-locks the rules table,
// rewrites and RECOMPILES every non-deleted rule against the new environment,
// and commits only if all recompile; any failure rolls the whole transaction
// back.
//
// DOWN (--down): IRREVERSIBLE. The rules table stores no provenance of the
// rewrite, so a migrated asset rule is indistinguishable from an authored one.
// The job refuses to run down and exits non-zero.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/bootstrap"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg"

	// automaxprocs honours Linux cgroup CPU quotas; see cmd/app/main.go.
	_ "go.uber.org/automaxprocs"
)

func main() {
	down := flag.Bool("down", false, "refuse and fail loud: the currency->asset rewrite is irreversible (no stored provenance)")
	up := flag.Bool("up", true, "apply the currency->asset rewrite to all stored CEL rules (atomic; rolls back on any recompile failure)")

	flag.Usage = usage

	flag.Parse()

	if err := run(*up, *down); err != nil {
		fmt.Fprintf(os.Stderr, "migrate-cel-rules: fatal: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(flag.CommandLine.Output(),
		`migrate-cel-rules — one-shot "000023" stored CEL rule migration (currency -> asset).

Usage:
  migrate-cel-rules [--up]     apply the rewrite (default)
  migrate-cel-rules --down     refuse (irreversible) and exit non-zero

SINGLE-TENANT ONLY: this job has no per-tenant fan-out. It refuses to run under
MULTI_TENANT_ENABLED=true and exits non-zero. Per-tenant execution is a follow-up.

Rollout is STOP-THE-WORLD / DRAINED: there is no currency->asset alias, so the
CEL env variable and the stored rules must both be asset before any instance
serves traffic. Configuration and the database connection are read from the
same environment variables as the tracer service.

Flags:
`)
	flag.PrintDefaults()
}

// run wires the migration from the shared bootstrap and dispatches to up/down.
// Extracted from main so it can return an error and exit cleanly via os.Exit
// instead of skipping deferred cleanup.
func run(up, down bool) error {
	pkg.InitLocalEnvConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	migration, err := bootstrap.InitCELRuleMigration(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize migration: %w", err)
	}

	defer func() {
		if closeErr := migration.Close(ctx); closeErr != nil {
			migration.Logger.Log(ctx, libLog.LevelError, "Failed to close migration resources", libLog.Err(closeErr))
		}
	}()

	// --down is explicit intent to fail loud; it takes precedence over the
	// default --up so `--down` alone (which leaves --up at its true default)
	// still runs the irreversible guard.
	if down {
		return migration.Migrator.Down(ctx)
	}

	if !up {
		return fmt.Errorf("nothing to do: pass --up to apply, or --down to run the irreversible guard")
	}

	res, err := migration.Migrator.Up(ctx)
	if err != nil {
		return fmt.Errorf("migration failed (rolled back, no rules changed): %w", err)
	}

	migration.Logger.Log(
		ctx, libLog.LevelInfo, "CEL stored-rule migration complete",
		libLog.Int("rules_scanned", res.Scanned),
		libLog.Int("rules_rewritten", res.Rewritten),
		libLog.Int("rules_unchanged", res.Unchanged),
	)

	return nil
}
