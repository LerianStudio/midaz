// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Partition-cutover repository wiring. This file is intentionally isolated
// from config.go / consumer_service.go so the cutover surface area can be
// reviewed, tested, and (eventually) removed in a single commit once the
// RENAME swap is permanent and legacy tables are dropped.
//
// Phase semantics (mirrors migrations 000021..000024):
//
//   - legacy_only: only the legacy (non-partitioned) tables exist as the
//     write target. Install the plain repositories. This is the default
//     on fresh deployments and before the cutover begins.
//   - dual_write: both the legacy `balance` / `operation` and the
//     `balance_partitioned` / `operation_partitioned` shells exist. Install
//     the DualWriteRepository wrapper so every INSERT lands atomically in
//     both tables. Reads stay on the legacy (renamed post-swap) table.
//   - partitioned: the atomic RENAME swap has happened; `balance` and
//     `operation` ARE the partitioned tables now. Install the plain
//     repositories — no wrapping needed, the name targets the post-swap
//     table transparently.
//
// Phase changes require a process restart (or a TTL-bounded cache delay of
// up to partitionstate.DefaultTTL, currently 30s, during which the reader
// picks up the new phase on the next refresh). In practice, operators
// invoke the RENAME-swap migration and immediately trigger a rolling
// restart so reads and writes converge at the same instant.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/partitionstate"
)

// partitionReaderDB adapts *libPostgres.PostgresConnection to the
// partitionstate.DB surface (only QueryRowContext is needed). lib-commons
// caches the dbresolver.DB after the first successful Connect(), so the
// GetDB call inside QueryRowContext is effectively a map lookup after
// startup.
type partitionReaderDB struct {
	conn *libPostgres.PostgresConnection
}

// QueryRowContext routes the read through the dbresolver returned by
// GetDB(). Bootstrap calls newPartitionReader after the Postgres connection
// is Connect()-ed, so GetDB is a cached lookup by the time queries flow.
// lib-commons surfaces driver/connection errors on the returned Row's Scan,
// matching the contract partitionstate.Reader expects for graceful
// fallback.
func (p partitionReaderDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db, _ := p.conn.GetDB()

	return db.QueryRowContext(ctx, query, args...)
}

// newPartitionReader constructs a phase Reader over the given Postgres
// connection. The reader caches successful lookups for DefaultTTL and
// gracefully degrades to PhaseLegacyOnly on DB error, so a misconfigured
// control table never blocks startup.
//
// Precondition: pc.GetDB() has returned successfully at least once before
// this call. Bootstrap satisfies this by initialising the connection before
// wiring repositories.
func newPartitionReader(pc *libPostgres.PostgresConnection, logger libLog.Logger) *partitionstate.Reader {
	return partitionstate.NewReader(partitionReaderDB{conn: pc}, 0, logger)
}

// wrapBalanceForPhase returns the balance repository honoring the current
// partition phase:
//
//   - PhaseDualWrite  → *DualWriteRepository wrapping the primary repo
//   - all other phases → the primary repository (unchanged semantics)
//
// The primary repository is constructed separately by the caller so that
// bootstrap-only helpers (e.g. pre-warm) can keep using the concrete struct
// type for methods outside the Repository interface. A later phase
// transition to dual_write will be picked up automatically within the
// reader's TTL, so phase changes usually require only a rolling restart.
func wrapBalanceForPhase(
	primary *balance.BalancePostgreSQLRepository,
	phaseReader balance.PartitionPhaseReader,
	phase partitionstate.Phase,
	logger libLog.Logger,
) balance.Repository {
	switch phase {
	case partitionstate.PhaseDualWrite:
		logger.Infof("partition: wiring balance DualWriteRepository (phase=%s)", phase)

		return balance.NewDualWriteRepository(primary, phaseReader)
	case partitionstate.PhaseLegacyOnly, partitionstate.PhasePartitioned:
		logger.Infof("partition: wiring plain balance repository (phase=%s)", phase)

		return primary
	default:
		logger.Warnf("partition: unknown phase %q, falling back to plain balance repository", phase)

		return primary
	}
}

// wrapOperationForPhase returns the operation repository honoring the
// current partition phase. Semantics mirror wrapBalanceForPhase — see its
// doc for details.
func wrapOperationForPhase(
	primary *operation.OperationPostgreSQLRepository,
	phaseReader operation.PartitionPhaseReader,
	phase partitionstate.Phase,
	logger libLog.Logger,
) operation.Repository {
	switch phase {
	case partitionstate.PhaseDualWrite:
		logger.Infof("partition: wiring operation DualWriteRepository (phase=%s)", phase)

		return operation.NewDualWriteRepository(primary, phaseReader)
	case partitionstate.PhaseLegacyOnly, partitionstate.PhasePartitioned:
		logger.Infof("partition: wiring plain operation repository (phase=%s)", phase)

		return primary
	default:
		logger.Warnf("partition: unknown phase %q, falling back to plain operation repository", phase)

		return primary
	}
}

// resolveInitialPartitionPhase queries the phase once at startup so bootstrap
// can pick the right repository wrapper. On failure it logs and falls back
// to PhaseLegacyOnly; this matches partitionstate.Reader semantics and
// prevents a transient control-table read from blocking service start.
func resolveInitialPartitionPhase(ctx context.Context, reader *partitionstate.Reader, logger libLog.Logger) partitionstate.Phase {
	phase, err := reader.Phase(ctx)
	if err != nil {
		logger.Warnf("partition: initial phase lookup failed, defaulting to %s: %v", partitionstate.PhaseLegacyOnly, err)

		return partitionstate.PhaseLegacyOnly
	}

	return phase
}

// partitionWiredRepos bundles the primary (concrete) and phase-wrapped
// (interface) forms of the repositories that participate in the partition
// cutover. Callers that need the concrete struct for methods outside the
// Repository interface (e.g. pre-warm helpers on BalancePostgreSQLRepository)
// use the *Primary fields; the UseCase receives the wrapped interface form.
type partitionWiredRepos struct {
	balancePrimary   *balance.BalancePostgreSQLRepository
	operationPrimary *operation.OperationPostgreSQLRepository

	balanceRepo   balance.Repository
	operationRepo operation.Repository
}

// wirePartitionAwareRepos constructs both primary repositories and their
// phase-aware wrappers in one call, keeping the bootstrap code paths
// compact enough to satisfy gocognit without duplicating logic between
// the main service and the dedicated consumer service.
func wirePartitionAwareRepos(pc *libPostgres.PostgresConnection, logger libLog.Logger) (*partitionWiredRepos, error) {
	reader := newPartitionReader(pc, logger)
	phase := resolveInitialPartitionPhase(context.Background(), reader, logger)

	balancePrimary, err := balance.NewBalancePostgreSQLRepository(pc)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize balance repository: %w", err)
	}

	operationPrimary, err := operation.NewOperationPostgreSQLRepository(pc)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize operation repository: %w", err)
	}

	return &partitionWiredRepos{
		balancePrimary:   balancePrimary,
		operationPrimary: operationPrimary,
		balanceRepo:      wrapBalanceForPhase(balancePrimary, reader, phase, logger),
		operationRepo:    wrapOperationForPhase(operationPrimary, reader, phase, logger),
	}, nil
}
