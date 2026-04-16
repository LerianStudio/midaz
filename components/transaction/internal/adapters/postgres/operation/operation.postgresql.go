// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// Repository provides an interface for operations related to operation template entities.
// It defines methods for creating, retrieving, updating, and deleting operation templates.
//
//go:generate mockgen --destination=operation.postgresql_mock.go --package=operation . Repository
type Repository interface {
	Create(ctx context.Context, operation *Operation) (*Operation, error)
	CreateBatch(ctx context.Context, operations []*Operation) error
	FindAll(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
	FindAllByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, operationType *string, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
	Find(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID) (*Operation, error)
	FindByAccount(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*Operation, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Operation, error)
	Update(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID, operation *Operation) (*Operation, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	// Point-in-time balance queries
	FindLastOperationBeforeTimestamp(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, timestamp time.Time) (*Operation, error)
	FindLastOperationsForAccountBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
}

// OperationPostgreSQLRepository is a Postgresql-specific implementation of the OperationRepository.
type OperationPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

type operationBatchExecTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

const (
	// operationColumnsCount is the number of columns in the operations INSERT.
	operationColumnsCount = 26
	// maxOperationBatchSize is the maximum operations per batch chunk.
	maxOperationBatchSize = constant.MaxPGParams / operationColumnsCount
)

var operationColumnList = []string{
	"id",
	"transaction_id",
	"description",
	"type",
	"asset_code",
	"amount",
	"available_balance",
	"on_hold_balance",
	"available_balance_after",
	"on_hold_balance_after",
	"status",
	"status_description",
	"account_id",
	"account_alias",
	"balance_id",
	"chart_of_accounts",
	"organization_id",
	"ledger_id",
	"created_at",
	"updated_at",
	"deleted_at",
	"route",
	"balance_affected",
	"balance_key",
	"balance_version_before",
	"balance_version_after",
}

// operationPointInTimeColumns contains only the columns needed for point-in-time balance queries.
// This reduced column list enables PostgreSQL to use Index-Only Scan with the covering index
// idx_operation_point_in_time, avoiding expensive heap fetches.
// Note: 'id' is included for cursor pagination support in list queries.
var operationPointInTimeColumns = []string{
	"id",
	"balance_id",
	"account_id",
	"asset_code",
	"balance_key",
	"available_balance_after",
	"on_hold_balance_after",
	"balance_version_after",
	"created_at",
}

// NewOperationPostgreSQLRepository returns a new instance of OperationPostgreSQLRepository using the given Postgres connection.
//
// NOTE: tableName is schema-qualified ("public.operation") so every squirrel
// builder emits FROM/INSERT/UPDATE against public.operation regardless of the
// session's search_path. This matters after migration 000022
// (staged_cutover/000022_atomic_swap_operation.up.sql) renames
// operation → operation_legacy and operation_partitioned → operation: an
// unqualified identifier would silently resolve to whichever table matches
// first in search_path, and a stray `SET search_path` (e.g., from a tenant
// template) could flip reads to operation_legacy — a silent correctness
// regression with no observable error. Schema qualification makes the
// target explicit at the query layer; TestRepository_QueriesUseSchemaQualifiedTableNames
// enforces the invariant going forward.
func NewOperationPostgreSQLRepository(pc *libPostgres.PostgresConnection) (*OperationPostgreSQLRepository, error) {
	c := &OperationPostgreSQLRepository{
		connection: pc,
		tableName:  "public.operation",
	}

	if _, err := c.connection.GetDB(); err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	return c, nil
}

// Create a new Operation entity into Postgresql and returns it.
func (r *OperationPostgreSQLRepository) Create(ctx context.Context, operation *Operation) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	record := &OperationPostgreSQLModel{}
	record.FromEntity(operation)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	insert := squirrel.
		Insert(r.tableName).
		Columns(operationColumnList...).
		Values(
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
		).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := insert.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to build insert query", err)

		logger.Errorf("Failed to build insert query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			libOpentelemetry.HandleSpanEvent(&spanExec, "Operation already exists, skipping duplicate insert (idempotent retry)")

			logger.Infof("Operation already exists, skipping duplicate insert (idempotent retry)")

			return nil, fmt.Errorf("failed to execute query: %w", err)
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation. Rows affected is 0", err)

		logger.Warnf("Failed to create operation. Rows affected is 0: %v", err)

		return nil, err //nolint:wrapcheck
	}

	return record.ToEntity(), nil
}

// CreateBatch inserts multiple operations in a single multi-row INSERT statement.
// This replaces the per-operation INSERT loop, reducing N round-trips to PostgreSQL
// to a single statement. Duplicates are silently skipped via ON CONFLICT DO NOTHING.
//
// All chunks execute within a single database transaction so the entire batch is
// atomic — either all operations are inserted or none are.
func (r *OperationPostgreSQLRepository) CreateBatch(ctx context.Context, operations []*Operation) (err error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operations_batch")
	defer span.End()

	if len(operations) == 0 {
		return nil
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", err)

		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to rollback", rollbackErr)

				logger.Errorf("err on rollback: %v", rollbackErr)
			}
		} else {
			commitErr := tx.Commit()
			if commitErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to commit", commitErr)

				logger.Errorf("err on commit: %v", commitErr)
				err = commitErr
			}
		}
	}()

	totalRowsAffected, err := r.createBatchWithExecutor(ctx, tx, operations)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute batch insert", err)

		logger.Errorf("Failed to execute batch insert: %v", err)

		return fmt.Errorf("failed to execute query: %w", err)
	}

	if totalRowsAffected < int64(len(operations)) {
		logger.Warnf("Batch operation insert: %d/%d rows inserted (duplicates skipped)", totalRowsAffected, len(operations))
	} else {
		logger.Infof("Batch operation insert: %d/%d rows inserted", totalRowsAffected, len(operations))
	}

	return nil
}

// CreateBatchWithTx inserts operations using an existing SQL transaction.
func (r *OperationPostgreSQLRepository) CreateBatchWithTx(ctx context.Context, tx operationBatchExecTx, operations []*Operation) error {
	_, err := r.createBatchWithExecutor(ctx, tx, operations)

	return fmt.Errorf("failed to perform database operation: %w", err)
}

func (r *OperationPostgreSQLRepository) createBatchWithExecutor(ctx context.Context, executor operationBatchExecTx, operations []*Operation) (int64, error) {
	if executor == nil || len(operations) == 0 {
		return 0, nil
	}

	// Process operations in chunks to stay within PostgreSQL's 65535 parameter limit.
	// Each operation uses 26 params, so max batch is 65535/26 = 2520 operations per chunk.
	var totalRowsAffected int64

	for chunkStart := 0; chunkStart < len(operations); chunkStart += maxOperationBatchSize {
		chunkEnd := chunkStart + maxOperationBatchSize
		if chunkEnd > len(operations) {
			chunkEnd = len(operations)
		}

		chunk := operations[chunkStart:chunkEnd]

		insert := squirrel.
			Insert(r.tableName).
			Columns(operationColumnList...).
			PlaceholderFormat(squirrel.Dollar).
			Suffix("ON CONFLICT (id, ledger_id) DO NOTHING")

		hasRows := false

		for _, op := range chunk {
			if op == nil {
				continue
			}

			record := &OperationPostgreSQLModel{}
			record.FromEntity(op)

			insert = insert.Values(
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
			)

			hasRows = true
		}

		if !hasRows {
			continue
		}

		query, args, err := insert.ToSql()
		if err != nil {
			return 0, fmt.Errorf("failed to build batch insert query: %w", err)
		}

		result, err := executor.ExecContext(ctx, query, args...)
		if err != nil {
			return 0, fmt.Errorf("failed to execute batch insert: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}

		totalRowsAffected += rowsAffected
	}

	return totalRowsAffected, nil
}

// FindAll retrieves Operations entities from the database.
func (r *OperationPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to get database connection: %w", err)
	}

	operations := make([]*Operation, 0)

	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to decode cursor: %w", err)
		}
	}

	findAll := squirrel.Select(operationColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("transaction_id = ?", transactionID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to build query: %w", err)
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var operation OperationPostgreSQLModel
		if err := rows.Scan(
			&operation.ID,
			&operation.TransactionID,
			&operation.Description,
			&operation.Type,
			&operation.AssetCode,
			&operation.Amount,
			&operation.AvailableBalance,
			&operation.OnHoldBalance,
			&operation.AvailableBalanceAfter,
			&operation.OnHoldBalanceAfter,
			&operation.Status,
			&operation.StatusDescription,
			&operation.AccountID,
			&operation.AccountAlias,
			&operation.BalanceID,
			&operation.ChartOfAccounts,
			&operation.OrganizationID,
			&operation.LedgerID,
			&operation.CreatedAt,
			&operation.UpdatedAt,
			&operation.DeletedAt,
			&operation.Route,
			&operation.BalanceAffected,
			&operation.BalanceKey,
			&operation.VersionBalance,
			&operation.VersionBalanceAfter,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to scan row: %w", err)
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to perform database operation: %w", err)
	}

	hasPagination := len(operations) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	operations = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, operations, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(operations) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, operations[0].ID, operations[len(operations)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to calculate cursor: %w", err)
		}
	}

	return operations, cur, nil
}

// ListByIDs retrieves Operation entities from the database using the provided IDs.
func (r *OperationPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_operations_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	var operations []*Operation

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_ids.query")

	list := squirrel.
		Select(operationColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID}).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := list.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build list by IDs query", err)

		logger.Errorf("Failed to build list by IDs query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var operation OperationPostgreSQLModel
		if err := rows.Scan(
			&operation.ID,
			&operation.TransactionID,
			&operation.Description,
			&operation.Type,
			&operation.AssetCode,
			&operation.Amount,
			&operation.AvailableBalance,
			&operation.OnHoldBalance,
			&operation.AvailableBalanceAfter,
			&operation.OnHoldBalanceAfter,
			&operation.Status,
			&operation.StatusDescription,
			&operation.AccountID,
			&operation.AccountAlias,
			&operation.BalanceID,
			&operation.ChartOfAccounts,
			&operation.OrganizationID,
			&operation.LedgerID,
			&operation.CreatedAt,
			&operation.UpdatedAt,
			&operation.DeletedAt,
			&operation.Route,
			&operation.BalanceAffected,
			&operation.BalanceKey,
			&operation.VersionBalance,
			&operation.VersionBalanceAfter,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, fmt.Errorf("failed to perform database operation: %w", err)
	}

	return operations, nil
}

// Find retrieves a Operation entity from the database using the provided ID.
func (r *OperationPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	operation := &OperationPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	find := squirrel.
		Select(operationColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID, "transaction_id": transactionID, "id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := find.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build find query", err)

		logger.Errorf("Failed to build find query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&operation.ID,
		&operation.TransactionID,
		&operation.Description,
		&operation.Type,
		&operation.AssetCode,
		&operation.Amount,
		&operation.AvailableBalance,
		&operation.OnHoldBalance,
		&operation.AvailableBalanceAfter,
		&operation.OnHoldBalanceAfter,
		&operation.Status,
		&operation.StatusDescription,
		&operation.AccountID,
		&operation.AccountAlias,
		&operation.BalanceID,
		&operation.ChartOfAccounts,
		&operation.OrganizationID,
		&operation.LedgerID,
		&operation.CreatedAt,
		&operation.UpdatedAt,
		&operation.DeletedAt,
		&operation.Route,
		&operation.BalanceAffected,
		&operation.BalanceKey,
		&operation.VersionBalance,
		&operation.VersionBalanceAfter,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation not found", err)

			logger.Warnf("Operation not found: %v", err)

			return nil, err //nolint:wrapcheck
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	return operation.ToEntity(), nil
}

// FindByAccount retrieves a Operation entity from the database using the provided account ID.
func (r *OperationPostgreSQLRepository) FindByAccount(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations_by_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	operation := &OperationPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all_by_account.query")

	findAcc := squirrel.
		Select(operationColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID, "account_id": accountID, "id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAcc.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build find by account query", err)

		logger.Errorf("Failed to build find by account query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&operation.ID,
		&operation.TransactionID,
		&operation.Description,
		&operation.Type,
		&operation.AssetCode,
		&operation.Amount,
		&operation.AvailableBalance,
		&operation.OnHoldBalance,
		&operation.AvailableBalanceAfter,
		&operation.OnHoldBalanceAfter,
		&operation.Status,
		&operation.StatusDescription,
		&operation.AccountID,
		&operation.AccountAlias,
		&operation.BalanceID,
		&operation.ChartOfAccounts,
		&operation.OrganizationID,
		&operation.LedgerID,
		&operation.CreatedAt,
		&operation.UpdatedAt,
		&operation.DeletedAt,
		&operation.Route,
		&operation.BalanceAffected,
		&operation.BalanceKey,
		&operation.VersionBalance,
		&operation.VersionBalanceAfter,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation not found", err)

			logger.Warnf("Operation not found: %v", err)

			return nil, err //nolint:wrapcheck
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	return operation.ToEntity(), nil
}

// Update an Operation entity into Postgresql and returns the Operation updated.
func (r *OperationPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID, operation *Operation) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	record := &OperationPostgreSQLModel{}
	record.FromEntity(operation)

	qb := squirrel.Update(r.tableName).
		PlaceholderFormat(squirrel.Dollar)

	if operation.Description != "" {
		qb = qb.Set("description", record.Description)
	}

	record.UpdatedAt = time.Now().UTC()

	qb = qb.Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID, "transaction_id": transactionID, "id": id}).
		Where(squirrel.Eq{"deleted_at": nil})

	query, args, err := qb.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build update query", err)

		logger.Errorf("Failed to build update query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation. Rows affected is 0", err)

		logger.Warnf("Failed to update operation. Rows affected is 0: %v", err)

		return nil, err //nolint:wrapcheck
	}

	return record.ToEntity(), nil
}

// Delete removes a Operation entity from the database using the provided IDs.
func (r *OperationPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	qb := squirrel.Update(r.tableName).
		Set("deleted_at", squirrel.Expr("now()"))
	qb = qb.Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID, "id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := qb.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build delete query", err)

		logger.Errorf("Failed to build delete query: %v", err)

		return fmt.Errorf("failed to build query: %w", err)
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute database query", err)

		logger.Errorf("Failed to execute database query: %v", err)

		return fmt.Errorf("failed to execute query: %w", err)
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete operation. Rows affected is 0", err)

		logger.Warnf("Failed to delete operation. Rows affected is 0: %v", err)

		return err //nolint:wrapcheck
	}

	return nil
}

// FindAllByAccount retrieves Operations entities from the database using the provided account ID.
func (r *OperationPostgreSQLRepository) FindAllByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, operationType *string, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations_by_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to get database connection: %w", err)
	}

	operations := make([]*Operation, 0)

	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to decode cursor: %w", err)
		}
	}

	findAll := squirrel.Select(operationColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("account_id = ?", accountID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	if !libCommons.IsNilOrEmpty(operationType) {
		findAll = findAll.Where(squirrel.Expr("type = ?", *operationType))
	}

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to build query: %w", err)
	}

	logger.Debugf("FindAllByAccount query: %s with args: %v", query, args)

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all_by_account.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		logger.Errorf("Failed to query database: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var operation OperationPostgreSQLModel
		if err := rows.Scan(
			&operation.ID,
			&operation.TransactionID,
			&operation.Description,
			&operation.Type,
			&operation.AssetCode,
			&operation.Amount,
			&operation.AvailableBalance,
			&operation.OnHoldBalance,
			&operation.AvailableBalanceAfter,
			&operation.OnHoldBalanceAfter,
			&operation.Status,
			&operation.StatusDescription,
			&operation.AccountID,
			&operation.AccountAlias,
			&operation.BalanceID,
			&operation.ChartOfAccounts,
			&operation.OrganizationID,
			&operation.LedgerID,
			&operation.CreatedAt,
			&operation.UpdatedAt,
			&operation.DeletedAt,
			&operation.Route,
			&operation.BalanceAffected,
			&operation.BalanceKey,
			&operation.VersionBalance,
			&operation.VersionBalanceAfter,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to scan row: %w", err)
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to perform database operation: %w", err)
	}

	hasPagination := len(operations) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	operations = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, operations, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(operations) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, operations[0].ID, operations[len(operations)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to calculate cursor: %w", err)
		}
	}

	return operations, cur, nil
}

// FindLastOperationBeforeTimestamp finds the last operation for a specific balance before a given timestamp.
// This is used for point-in-time balance queries to determine the balance state at a specific moment.
func (r *OperationPostgreSQLRepository) FindLastOperationBeforeTimestamp(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, timestamp time.Time) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_last_operation_before_timestamp")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Build query to find the last operation for this balance before the timestamp
	// Uses optimized column list (9 columns vs 26) to enable Index-Only Scan with covering index
	findQuery := squirrel.Select(operationPointInTimeColumns...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"balance_id": balanceID}).
		Where(squirrel.LtOrEq{"created_at": timestamp}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC", "balance_version_after DESC", "id DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)
		logger.Errorf("Failed to build query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.Debugf("FindLastOperationBeforeTimestamp query: %s with args: %v", query, args)

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_last_operation_before_timestamp.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	var operation OperationPointInTimeModel
	if err := row.Scan(
		&operation.ID,
		&operation.BalanceID,
		&operation.AccountID,
		&operation.AssetCode,
		&operation.BalanceKey,
		&operation.AvailableBalanceAfter,
		&operation.OnHoldBalanceAfter,
		&operation.VersionBalanceAfter,
		&operation.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "No operation found before timestamp", err)
			logger.Debugf("No operation found for balance %s before timestamp %s", balanceID, timestamp)

			return nil, nil
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)
		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	return operation.ToEntity(), nil
}

// FindLastOperationsForAccountBeforeTimestamp finds the last operation for each balance of an account before a given timestamp.
// This is used for point-in-time account balance queries to get all balance states at a specific moment.
func (r *OperationPostgreSQLRepository) FindLastOperationsForAccountBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_last_operations_for_account_before_timestamp")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to get database connection: %w", err)
	}

	operations := make([]*Operation, 0)

	// Cursor pagination setup
	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)
			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to decode cursor: %w", err)
		}
	}

	// Build query using DISTINCT ON to get the last operation per balance_id
	// PostgreSQL DISTINCT ON returns the first row for each distinct value based on ORDER BY
	// Uses optimized column list (9 columns vs 26) to enable Index-Only Scan with covering index
	findQuery := squirrel.Select("DISTINCT ON (balance_id) "+strings.Join(operationPointInTimeColumns, ", ")).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"account_id": accountID}).
		Where(squirrel.LtOrEq{"created_at": timestamp}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("balance_id", "created_at DESC", "balance_version_after DESC", "id DESC").
		PlaceholderFormat(squirrel.Dollar)

	// Apply pagination on the outer query
	outerQuery := squirrel.Select(operationPointInTimeColumns...).
		FromSelect(findQuery, "sub").
		PlaceholderFormat(squirrel.Dollar)

	outerQuery, orderDirection = libHTTP.ApplyCursorPagination(outerQuery, decodedCursor, orderDirection, filter.Limit)

	query, args, err := outerQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build outer query", err)
		logger.Errorf("Failed to build outer query: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to build query: %w", err)
	}

	logger.Debugf("FindLastOperationsForAccountBeforeTimestamp query: %s with args: %v", query, args)

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_last_operations_for_account_before_timestamp.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)
		logger.Errorf("Failed to query database: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var operation OperationPointInTimeModel
		if err := rows.Scan(
			&operation.ID,
			&operation.BalanceID,
			&operation.AccountID,
			&operation.AssetCode,
			&operation.BalanceKey,
			&operation.AvailableBalanceAfter,
			&operation.OnHoldBalanceAfter,
			&operation.VersionBalanceAfter,
			&operation.CreatedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)
			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to scan row: %w", err)
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)
		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to perform database operation: %w", err)
	}

	hasPagination := len(operations) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	operations = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, operations, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(operations) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, operations[0].ID, operations[len(operations)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)
			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to calculate cursor: %w", err)
		}
	}

	return operations, cur, nil
}
