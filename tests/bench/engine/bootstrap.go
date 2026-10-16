//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	_ "embed" // schema_bench.sql
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/backoff"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	disposableMarker   = "midaz-bench-engine:disposable"
	primaryClusterName = "bench-pg-primary"
	standbyClusterName = "bench-pg-standby"
	standbyName        = "standby1"
	replicationWait    = 120 * time.Second
)

// verifiedStack holds sessions on nodes the guard proved to be the disposable compose stack.
type verifiedStack struct {
	primary *pgx.Conn
	standby *pgx.Conn
}

//go:embed schema_bench.sql
var schemaBenchSQL string

// bootstrapSchema proves the target is the disposable stack, then rebuilds public from the real ledger
// migrations, the lib-commons outbox schema and schema_bench.sql. Nothing is written before the guard passes.
func bootstrapSchema(ctx context.Context, env *benchEnv) error {
	repo, err := moduleDir(ctx, "github.com/LerianStudio/midaz/v4")
	if err != nil {
		return err
	}

	libCommons, err := moduleDir(ctx, "github.com/LerianStudio/lib-commons/v7")
	if err != nil {
		return err
	}

	outboxSQL, err := os.ReadFile(filepath.Join(libCommons, "commons/outbox/postgres/migrations/000001_outbox_events_schema.up.sql"))
	if err != nil {
		return fmt.Errorf("read lib-commons outbox schema: %w", err)
	}

	stack, err := openVerifiedStack(ctx, env)
	if err != nil {
		return err
	}
	defer stack.close(ctx)

	return stack.rebuild(ctx, filepath.Join(repo, "components/ledger/migrations/transaction"), string(outboxSQL))
}

// rebuild resets public on the verified primary session and replays every schema source in order.
// Every commit waits for standby1 (the guard checked synchronous_commit=on); any failure aborts the run.
func (s *verifiedStack) rebuild(ctx context.Context, ledgerMigrations, outboxSQL string) error {
	if _, err := s.primary.Exec(ctx, "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public"); err != nil {
		return fmt.Errorf("reset public schema: %w", err)
	}

	if err := migrateLedger(ctx, s.primary.Config(), ledgerMigrations); err != nil {
		return err
	}

	// Each source is one simple-protocol Exec: an implicit transaction, all or nothing.
	for _, source := range []struct{ name, sql string }{{"lib-commons outbox schema", outboxSQL}, {"schema_bench.sql", schemaBenchSQL}} {
		if _, err := s.primary.Exec(ctx, source.sql); err != nil {
			return fmt.Errorf("apply %s: %w", source.name, err)
		}
	}

	return nil
}

// migrateLedger runs the ledger migrations through the lib-commons Migrator, one file per Exec, so
// migration 46's CREATE INDEX CONCURRENTLY runs outside any transaction.
func migrateLedger(ctx context.Context, cfg *pgx.ConnConfig, dir string) error {
	// The registered name hands the migrator this verified config instead of a DSN it would re-parse.
	name := stdlib.RegisterConnConfig(cfg)
	defer stdlib.UnregisterConnConfig(name)

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:     name,
		DatabaseName:   benchDatabase,
		MigrationsPath: dir,
	})
	if err != nil {
		return fmt.Errorf("ledger migrator: %w", err)
	}

	// shortcut: Up stops watching ctx once golang-migrate starts; go test -timeout bounds the rest.
	if err := migrator.Up(ctx); err != nil {
		return fmt.Errorf("ledger migrations: %w", err)
	}

	return nil
}

// moduleDir resolves the directory of a module pinned by go.mod.
func moduleDir(ctx context.Context, module string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	out, err := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}", module).Output()
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
		return "", fmt.Errorf("go list -m %s: %w: %s", module, err, bytes.TrimSpace(exitErr.Stderr))
	}

	if err != nil {
		return "", fmt.Errorf("go list -m %s: %w", module, err)
	}

	return string(bytes.TrimSpace(out)), nil
}

// openVerifiedStack proves the three database endpoints are this stack's primary, client proxy and standby,
// and that standby1 is its only synchronous, streaming replica.
func openVerifiedStack(ctx context.Context, env *benchEnv) (*verifiedStack, error) {
	primary, systemID, err := connectNode(ctx, env.primary, primaryClusterName, false)
	if err != nil {
		return nil, fmt.Errorf("bench primary: %w", err)
	}

	standby, err := verifyPeers(ctx, env, systemID)
	if err != nil {
		primary.Close(ctx)

		return nil, err
	}

	stack := &verifiedStack{primary: primary, standby: standby}
	if err := stack.awaitStandby(ctx); err != nil {
		stack.close(ctx)

		return nil, err
	}

	return stack, nil
}

func (s *verifiedStack) close(ctx context.Context) {
	s.primary.Close(ctx)
	s.standby.Close(ctx)
}

// verifyPeers checks the client proxy reaches the primary and the standby replicates the same cluster.
func verifyPeers(ctx context.Context, env *benchEnv, systemID int64) (*pgx.Conn, error) {
	client, clientID, err := connectNode(ctx, env.clientProxy, primaryClusterName, false)
	if err != nil {
		return nil, fmt.Errorf("bench client proxy: %w", err)
	}

	client.Close(ctx)

	standby, standbyID, err := connectNode(ctx, env.standby, standbyClusterName, true)
	if err != nil {
		return nil, fmt.Errorf("bench standby: %w", err)
	}

	if clientID != systemID || standbyID != systemID {
		standby.Close(ctx)

		return nil, fmt.Errorf("system_identifier differs: primary %d, client proxy %d, standby %d", systemID, clientID, standbyID)
	}

	return standby, nil
}

// connectNode connects and checks the server identity; a node that fails the check is closed.
func connectNode(ctx context.Context, cfg *pgx.ConnConfig, wantCluster string, wantRecovery bool) (*pgx.Conn, int64, error) {
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, 0, err
	}

	var (
		cluster, marker, syncCommit string
		inRecovery                  bool
		systemID                    int64
	)

	err = conn.QueryRow(ctx, `SELECT current_setting('cluster_name'), pg_is_in_recovery(),
		coalesce(shobj_description(d.oid, 'pg_database'), ''),
		(SELECT system_identifier FROM pg_control_system()), current_setting('synchronous_commit')
		FROM pg_database d WHERE d.datname = current_database()`).
		Scan(&cluster, &inRecovery, &marker, &systemID, &syncCommit)

	switch {
	case err != nil:
		err = fmt.Errorf("read identity: %w", err)
	case cluster != wantCluster:
		err = fmt.Errorf("cluster_name %q, want %q", cluster, wantCluster)
	case inRecovery != wantRecovery:
		err = fmt.Errorf("pg_is_in_recovery %t, want %t", inRecovery, wantRecovery)
	case marker != disposableMarker:
		err = fmt.Errorf("database marker %q, want %q", marker, disposableMarker)
	case !wantRecovery && syncCommit != "on":
		err = fmt.Errorf("synchronous_commit %q, want on", syncCommit)
	}

	if err != nil {
		conn.Close(ctx)

		return nil, 0, fmt.Errorf("%s:%d: %w", cfg.Host, cfg.Port, err)
	}

	return conn, systemID, nil
}

// awaitStandby waits until standby1 is the only replica, streaming and synchronous, and the standby's
// WAL receiver streams.
func (s *verifiedStack) awaitStandby(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, replicationWait)
	defer cancel()

	for {
		var synced, senders int
		if err := s.primary.QueryRow(ctx, `SELECT count(*) FILTER (WHERE application_name = $1 AND state = 'streaming'
			AND sync_state = 'sync'), count(*) FROM pg_stat_replication`, standbyName).Scan(&synced, &senders); err != nil {
			return fmt.Errorf("read pg_stat_replication: %w", err)
		}

		var receiving bool
		if err := s.standby.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_wal_receiver WHERE status = 'streaming')`).
			Scan(&receiving); err != nil {
			return fmt.Errorf("read pg_stat_wal_receiver: %w", err)
		}

		if synced == 1 && senders == 1 && receiving {
			return nil
		}

		if err := backoff.WaitContext(ctx, time.Second); err != nil {
			return fmt.Errorf("%s not the only streaming synchronous replica within %s (synced %d, senders %d, receiving %t): %w",
				standbyName, replicationWait, synced, senders, receiving, err)
		}
	}
}
