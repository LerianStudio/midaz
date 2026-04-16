// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Dual-write support for the partition cutover. This file is intentionally
// isolated from operation.postgresql.go so the partition-cutover surface area
// can be reviewed, tested, and (eventually) removed in a single commit.
//
// The dual-write repository wraps a vanilla *OperationPostgreSQLRepository
// and, when partition phase is PhaseDualWrite, mirrors INSERTs into the
// operation_partitioned shell table inside the same database transaction as
// the primary legacy write.
//
// Bootstrap wires the dual-write wrapper only while phase is dual_write —
// before or after the cutover it uses the plain repository. The wrapper
// implements the same Repository interface so callers are oblivious.

package operation

import (
	"context"
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
)

// OperationPartitionedTableName is the name of the partitioned shell target
// during the dual-write window.
const OperationPartitionedTableName = "operation_partitioned"

// PartitionPhaseReader is the minimal surface the dual-write layer needs.
// partitionstate.Reader satisfies it.
type PartitionPhaseReader interface {
	Phase(ctx context.Context) (partitionstate.Phase, error)
}

// DualWriteRepository wraps Repository with partition-cutover dual-write.
//
// When PartitionPhaseReader reports PhaseDualWrite, Create mirrors the insert
// into OperationPartitionedTableName inside the same transaction as the
// primary legacy insert; either both writes land or both roll back. All other
// Repository methods delegate unchanged.
//
// A nil PhaseReader or a read error degrades to single-write (primary only),
// so dual-write infrastructure issues never break the hot write path.
type DualWriteRepository struct {
	Repository

	inner       *OperationPostgreSQLRepository
	phaseReader PartitionPhaseReader
}

// NewDualWriteRepository returns a Repository that honors the current
// partition phase when executing Create. All other methods delegate.
func NewDualWriteRepository(inner *OperationPostgreSQLRepository, phaseReader PartitionPhaseReader) *DualWriteRepository {
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

// Create inserts the operation into the legacy table and, when dual-write is
// active, also into OperationPartitionedTableName inside the same transaction.
// When dual-write is inactive the call delegates to the inner repository.
func (d *DualWriteRepository) Create(ctx context.Context, op *Operation) (*Operation, error) {
	if !d.shouldDualWrite(ctx) {
		return d.inner.Create(ctx, op)
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operation_dualwrite")
	defer span.End()

	db, err := d.inner.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	record := &OperationPostgreSQLModel{}
	record.FromEntity(op)

	values := []any{
		record.ID,
		record.TransactionID,
		record.Description,
		record.Type,
		record.AssetCode,
		record.Amount,
		record.AvailableBalance,
		record.OnHoldBalance,
		record.AvailableBalanceAfter,
		record.OnHoldBalanceAfter,
		record.Status,
		record.StatusDescription,
		record.AccountID,
		record.AccountAlias,
		record.BalanceID,
		record.ChartOfAccounts,
		record.OrganizationID,
		record.LedgerID,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
		record.Route,
		record.BalanceAffected,
		record.BalanceKey,
		record.VersionBalance,
		record.VersionBalanceAfter,
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin dual-write tx", err)

		return nil, fmt.Errorf("failed to begin dual-write tx: %w", err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Primary legacy insert: DO propagate unique-violation semantics the same
	// way the non-dual-write Create does, because application code relies on
	// it for idempotent retries.
	legacySQL, legacyArgs, err := squirrel.
		Insert(d.inner.tableName).
		Columns(operationColumnList...).
		Values(values...).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build legacy insert", err)

		return nil, fmt.Errorf("failed to build legacy insert: %w", err)
	}

	result, err := tx.ExecContext(ctx, legacySQL, legacyArgs...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			libOpentelemetry.HandleSpanEvent(&span, "Operation already exists, skipping duplicate insert (idempotent retry)")

			logger.Infof("Operation already exists, skipping duplicate insert (idempotent retry)")

			return nil, fmt.Errorf("failed to execute query: %w", err)
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to execute legacy insert", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to read rows affected", err)

		return nil, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		bizErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation (rows affected is 0)", bizErr)

		return nil, bizErr //nolint:wrapcheck
	}

	// Partitioned mirror. ON CONFLICT DO NOTHING so that a row already placed
	// by the background backfill tool does not fail the transaction. This is
	// different from the legacy path — the legacy table's unique-violation is
	// surfaced because it signals a true idempotency hit; the partitioned
	// mirror's conflicts are expected during the cutover window.
	partSQL, partArgs, err := squirrel.
		Insert(OperationPartitionedTableName).
		Columns(operationColumnList...).
		Values(values...).
		Suffix("ON CONFLICT DO NOTHING").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build partitioned insert", err)

		return nil, fmt.Errorf("failed to build partitioned insert: %w", err)
	}

	if _, err = tx.ExecContext(ctx, partSQL, partArgs...); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute partitioned insert", err)

		return nil, fmt.Errorf("failed to execute partitioned insert: %w", err)
	}

	if err = tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit dual-write", err)

		return nil, fmt.Errorf("failed to commit dual-write: %w", err)
	}

	committed = true

	return record.ToEntity(), nil
}

// Compile-time verification that DualWriteRepository still satisfies the
// public Repository interface so bootstrap can swap implementations freely.
var _ Repository = (*DualWriteRepository)(nil)
