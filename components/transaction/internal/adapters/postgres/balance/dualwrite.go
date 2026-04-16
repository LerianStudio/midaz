// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Dual-write support for the partition cutover. This file is intentionally
// isolated from balance.postgresql.go so the partition-cutover surface area
// can be reviewed, tested, and (eventually) removed in a single commit.
//
// The dual-write repository wraps a vanilla *BalancePostgreSQLRepository
// and, when partition phase is PhaseDualWrite, mirrors INSERTs into the
// balance_partitioned shell table inside the same database transaction as
// the primary legacy write.
//
// Bootstrap wires the dual-write wrapper only while phase is dual_write —
// before or after the cutover it uses the plain repository. The wrapper
// implements the same Repository interface so callers are oblivious.
//
// Reads, Updates and Deletes go only to the primary (legacy) table while
// phase is dual_write. The backfill tool is responsible for seeding the
// partitioned table with pre-existing rows; after phase transitions to
// PhasePartitioned the primary repository's tableName is "balance" (the
// RENAME swap has happened) so the same Repository implementation now
// transparently targets the partitioned table.

package balance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/partitionstate"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// BalancePartitionedTableName is the name of the partitioned shell target
// during the dual-write window.
const BalancePartitionedTableName = "balance_partitioned"

// PartitionPhaseReader is the minimal surface the dual-write layer needs.
// partitionstate.Reader satisfies it.
type PartitionPhaseReader interface {
	Phase(ctx context.Context) (partitionstate.Phase, error)
}

// DualWriteRepository wraps Repository with partition-cutover dual-write.
//
// When PartitionPhaseReader reports PhaseDualWrite, write-path methods (Create,
// CreateIfNotExists) mirror the insert into BalancePartitionedTableName inside
// the same transaction as the primary legacy insert; either both writes land
// or both roll back. All other Repository methods delegate unchanged.
//
// A nil PhaseReader or a read error degrades to single-write (primary only),
// so dual-write infrastructure issues never break the hot write path. This
// matches the partitionstate.Reader fallback-to-PhaseLegacyOnly semantics.
type DualWriteRepository struct {
	Repository

	inner       *BalancePostgreSQLRepository
	phaseReader PartitionPhaseReader
}

// NewDualWriteRepository returns a Repository that honors the current
// partition phase when executing write-path methods. All other methods
// delegate to the wrapped Repository.
func NewDualWriteRepository(inner *BalancePostgreSQLRepository, phaseReader PartitionPhaseReader) *DualWriteRepository {
	return &DualWriteRepository{
		Repository:  inner,
		inner:       inner,
		phaseReader: phaseReader,
	}
}

// shouldDualWrite reports whether the current phase mandates mirroring to
// the partitioned shell. Any read error degrades to false so the primary
// write path is never blocked by control-table unavailability.
func (d *DualWriteRepository) shouldDualWrite(ctx context.Context) bool {
	if d.phaseReader == nil {
		return false
	}

	phase, err := d.phaseReader.Phase(ctx)
	if err != nil {
		return false
	}

	return phase == partitionstate.PhaseDualWrite
}

// balanceInsertValues materializes the 16 positional values for a balance
// INSERT in the exact order of balanceColumnList. Extracted so Create and
// CreateIfNotExists use an identical value vector and cannot drift.
func balanceInsertValues(record *BalancePostgreSQLModel) []any {
	return []any{
		record.ID,
		record.OrganizationID,
		record.LedgerID,
		record.AccountID,
		record.Alias,
		record.AssetCode,
		record.Available,
		record.OnHold,
		record.Version,
		record.AccountType,
		record.AllowSending,
		record.AllowReceiving,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
		record.Key,
	}
}

// Create inserts the balance into the legacy table and, when dual-write is
// active, also into BalancePartitionedTableName inside the same transaction.
// When dual-write is inactive the call delegates to the inner repository.
func (d *DualWriteRepository) Create(ctx context.Context, balance *mmodel.Balance) error {
	if !d.shouldDualWrite(ctx) {
		return d.inner.Create(ctx, balance)
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balance_dualwrite")
	defer span.End()

	db, err := d.inner.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	record := &BalancePostgreSQLModel{}
	record.FromEntity(balance)

	values := balanceInsertValues(record)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin dual-write tx", err)

		return fmt.Errorf("failed to begin dual-write tx: %w", err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Primary legacy insert. Unique-violation is surfaced the same way the
	// non-dual-write Create does because application code relies on it.
	legacySQL, legacyArgs, err := squirrel.
		Insert(d.inner.tableName).
		Columns(balanceColumnList...).
		Values(values...).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build legacy insert", err)

		return fmt.Errorf("failed to build legacy insert: %w", err)
	}

	result, err := tx.ExecContext(ctx, legacySQL, legacyArgs...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			libOpentelemetry.HandleSpanEvent(&span, "Balance already exists, skipping duplicate insert (idempotent retry)")

			logger.Infof("Balance already exists, skipping duplicate insert (idempotent retry)")

			return fmt.Errorf("failed to insert balance: %w", err)
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to execute legacy insert", err)

		return fmt.Errorf("failed to insert balance: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to read rows affected", err)

		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		bizErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create balance (rows affected is 0)", bizErr)

		return bizErr //nolint:wrapcheck
	}

	// Partitioned mirror. ON CONFLICT DO NOTHING so that a row already placed
	// by the background backfill tool does not fail the transaction. The
	// legacy path's unique-violation is surfaced (signals a true idempotency
	// hit); the partitioned mirror's conflicts are expected during cutover.
	if err = d.execPartitionedInsert(ctx, tx, values); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute partitioned insert", err)

		return err
	}

	if err = tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit dual-write", err)

		return fmt.Errorf("failed to commit dual-write: %w", err)
	}

	committed = true

	return nil
}

// CreateIfNotExists mirrors the ON CONFLICT DO NOTHING insert to both tables
// inside a single transaction when dual-write is active. Both sides use
// ON CONFLICT DO NOTHING because a concurrent materialization race is
// expected by callers (hot-path pre-split balance creation).
func (d *DualWriteRepository) CreateIfNotExists(ctx context.Context, balance *mmodel.Balance) error {
	if !d.shouldDualWrite(ctx) {
		return d.inner.CreateIfNotExists(ctx, balance)
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balance_if_not_exists_dualwrite")
	defer span.End()

	db, err := d.inner.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	record := &BalancePostgreSQLModel{}
	record.FromEntity(balance)

	values := balanceInsertValues(record)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin dual-write tx", err)

		return fmt.Errorf("failed to begin dual-write tx: %w", err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Primary legacy upsert: ON CONFLICT DO NOTHING matches the single-write
	// CreateIfNotExists semantics — duplicate alias/key is not an error.
	legacySQL, legacyArgs, err := squirrel.
		Insert(d.inner.tableName).
		Columns(balanceColumnList...).
		Values(values...).
		Suffix("ON CONFLICT (organization_id, ledger_id, alias, key) WHERE deleted_at IS NULL DO NOTHING").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build legacy upsert", err)

		return fmt.Errorf("failed to build legacy upsert: %w", err)
	}

	result, err := tx.ExecContext(ctx, legacySQL, legacyArgs...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute legacy upsert", err)

		logger.Errorf("Failed to execute legacy upsert: %v", err)

		return fmt.Errorf("failed to execute query: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to read rows affected", err)

		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		logger.Infof("Balance already exists for alias=%s key=%s, skipped legacy insert", balance.Alias, balance.Key)
	}

	if err = d.execPartitionedInsert(ctx, tx, values); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute partitioned insert", err)

		return err
	}

	if err = tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit dual-write", err)

		return fmt.Errorf("failed to commit dual-write: %w", err)
	}

	committed = true

	return nil
}

// dualWriteTx is the minimal transactional surface the partitioned mirror
// needs. Both *sql.Tx and the dbresolver.Tx returned by lib-commons satisfy
// it, which keeps this file decoupled from the concrete transaction type.
type dualWriteTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// execPartitionedInsert writes the balance row into BalancePartitionedTableName
// using ON CONFLICT DO NOTHING. Kept package-private so Create and
// CreateIfNotExists share exactly one partitioned-side implementation.
func (d *DualWriteRepository) execPartitionedInsert(ctx context.Context, tx dualWriteTx, values []any) error {
	partSQL, partArgs, err := squirrel.
		Insert(BalancePartitionedTableName).
		Columns(balanceColumnList...).
		Values(values...).
		Suffix("ON CONFLICT DO NOTHING").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build partitioned insert: %w", err)
	}

	if _, err = tx.ExecContext(ctx, partSQL, partArgs...); err != nil {
		return fmt.Errorf("failed to execute partitioned insert: %w", err)
	}

	return nil
}

// Compile-time verification that DualWriteRepository still satisfies the
// public Repository interface so bootstrap can swap implementations freely.
var _ Repository = (*DualWriteRepository)(nil)
