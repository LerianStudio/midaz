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
	"sort"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v4/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/repository"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"

	// Repository provides an interface for operations related to operation template entities.
	// It defines methods for creating, retrieving, updating, and deleting operation templates.
	//
	//go:generate mockgen --destination=operation.postgresql_mock.go --package=operation . Repository
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

// OperationFilter holds optional filters for listing operations.
type OperationFilter struct {
	OperationType *string
	Direction     *string
	RouteID       *string
	RouteCode     *string
}

type Repository interface {
	Create(ctx context.Context, operation *Operation) (*Operation, error)
	CreateBulk(ctx context.Context, operations []*Operation) (*repository.BulkInsertResult, error)
	CreateBulkTx(ctx context.Context, tx repository.DBExecutor, operations []*Operation) (*repository.BulkInsertResult, error)
	FindAll(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
	FindAllByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, opFilter OperationFilter, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
	Find(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID) (*Operation, error)
	FindByAccount(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*Operation, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Operation, error)
	Update(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID, operation *Operation) (*Operation, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	// Point-in-time balance queries
	FindLastOperationBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID, balanceID uuid.UUID, timestamp time.Time) (*Operation, error)
	FindLastOperationsForAccountBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
}

// OperationPostgreSQLRepository is a Postgresql-specific implementation of the OperationRepository.
type OperationPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

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
	"direction",
	"route_id",
	"route_code",
	"route_description",
}

// operationColumns is derived from operationColumnList for use with squirrel.Select.
var operationColumns = strings.Join(operationColumnList, ", ")

// operationPointInTimeColumns contains only the columns needed for point-in-time balance queries.
// These columns are served by idx_operation_account_balance_pit via heap fetches (the index
// is a lean key-only index without INCLUDE columns for optimal storage).
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
func NewOperationPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *OperationPostgreSQLRepository {
	c := &OperationPostgreSQLRepository{
		connection: pc,
		tableName:  "operation",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *OperationPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	// Module-specific connection (from middleware WithModule)
	if db := tmcore.GetPGContext(ctx, constant.ModuleTransaction); db != nil {
		return db, nil
	}

	// Generic connection fallback (single-module services)
	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant {
		return nil, fmt.Errorf("tenant postgres connection missing from context")
	}

	if r.connection == nil {
		return nil, fmt.Errorf("postgres connection not available")
	}

	return r.connection.Resolver(ctx)
}

// Create a new Operation entity into Postgresql and returns it.
func (r *OperationPostgreSQLRepository) Create(ctx context.Context, operation *Operation) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operation")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
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
			record.Direction,
			record.RouteID,
			record.RouteCode,
			record.RouteDescription,
		).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := insert.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to build insert query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build insert query: %v", err))

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			libOpentelemetry.HandleSpanEvent(spanExec, "Operation already exists, skipping duplicate insert (idempotent retry)")

			logger.Log(ctx, libLog.LevelInfo, "Operation already exists, skipping duplicate insert (idempotent retry)")

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create operation. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to create operation. Rows affected is 0: %v", err))

		return nil, err
	}

	return record.ToEntity(), nil
}

// CreateBulk inserts multiple operations in bulk using multi-row INSERT with ON CONFLICT DO NOTHING.
// Returns BulkInsertResult with counts of attempted, inserted, and ignored (duplicate) rows.
// Operations are sorted by ID before insert to prevent deadlocks in concurrent scenarios.
// Large bulks are automatically chunked to stay within PostgreSQL's parameter limits.
//
// NOTE: Chunks are committed independently. If chunk N fails, chunks 1 to N-1 remain committed.
// On error, partial results are returned along with the error. Retry is safe due to idempotency.
// On error, only Inserted is reliable; Ignored remains 0 since unprocessed chunks are not duplicates.
//
// NOTE: The input slice is sorted in-place by ID. Callers should not rely on original order after this call.
func (r *OperationPostgreSQLRepository) CreateBulk(ctx context.Context, operations []*Operation) (*repository.BulkInsertResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_bulk_operations")
	defer span.End()

	// Early return for empty input before acquiring DB connection
	if len(operations) == 0 {
		return &repository.BulkInsertResult{}, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	return r.createBulkInternal(ctx, db, operations, "postgres.create_bulk_operations_internal", "")
}

// CreateBulkTx inserts multiple operations in bulk using a caller-provided transaction.
// This allows the caller to control transaction boundaries for atomic multi-table operations.
// Returns BulkInsertResult with counts of attempted, inserted, and ignored (duplicate) rows.
// Operations are sorted by ID before insert to prevent deadlocks in concurrent scenarios.
// Large bulks are automatically chunked to stay within PostgreSQL's parameter limits.
//
// NOTE: The caller is responsible for calling Commit() or Rollback() on the transaction.
// On error, partial results are returned along with the error. The caller should rollback.
// On error, only Inserted is reliable; Ignored remains 0 since unprocessed chunks are not duplicates.
//
// NOTE: The input slice is sorted in-place by ID. Callers should not rely on original order after this call.
func (r *OperationPostgreSQLRepository) CreateBulkTx(ctx context.Context, tx repository.DBExecutor, operations []*Operation) (*repository.BulkInsertResult, error) {
	if tx == nil {
		return nil, repository.ErrNilDBExecutor
	}

	return r.createBulkInternal(ctx, tx, operations, "postgres.create_bulk_operations_tx", " (tx)")
}

// createBulkInternal contains the shared logic for CreateBulk and CreateBulkTx.
// It validates input, sorts operations by ID to prevent deadlocks, and inserts in chunks.
// Returns partial results on error with Attempted/Inserted counts.
func (r *OperationPostgreSQLRepository) createBulkInternal(
	ctx context.Context,
	db repository.DBExecutor,
	operations []*Operation,
	spanName string,
	logSuffix string,
) (*repository.BulkInsertResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	if len(operations) == 0 {
		return &repository.BulkInsertResult{}, nil
	}

	// Validate no nil elements to prevent panic during sort or insert
	for i, op := range operations {
		if op == nil {
			err := fmt.Errorf("nil operation at index %d", i)
			libOpentelemetry.HandleSpanError(span, "Invalid input: nil operation", err)

			return nil, err
		}
	}

	// Sort by ID (string UUID) to prevent deadlocks in concurrent bulk operations
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].ID < operations[j].ID
	})

	result := &repository.BulkInsertResult{
		Attempted:   int64(len(operations)),
		InsertedIDs: make([]string, 0, len(operations)),
	}

	// Chunk into bulks of ~1,000 rows to stay within PostgreSQL's parameter limit
	// Operation has 30 columns, so 1000 rows = 30,000 parameters (under 65,535 limit)
	const chunkSize = 1000

	for i := 0; i < len(operations); i += chunkSize {
		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			libOpentelemetry.HandleSpanError(span, "Context cancelled during bulk insert", ctx.Err())
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Context cancelled during bulk insert: %v", ctx.Err()))

			// Return partial result; Ignored stays 0 since remaining items were not processed
			return result, ctx.Err()
		default:
		}

		end := min(i+chunkSize, len(operations))

		chunkResult, err := r.insertOperationChunk(ctx, db, operations[i:end])
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to insert operation chunk", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to insert operation chunk: %v", err))

			// Return partial result; Ignored stays 0 since remaining items were not processed (not duplicates)
			return result, err
		}

		result.Inserted += chunkResult.inserted
		result.InsertedIDs = append(result.InsertedIDs, chunkResult.insertedIDs...)
	}

	result.Ignored = result.Attempted - result.Inserted

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Bulk insert operations%s: attempted=%d, inserted=%d, ignored=%d",
		logSuffix, result.Attempted, result.Inserted, result.Ignored))

	return result, nil
}

// operationChunkInsertResult holds the result of inserting a chunk of operations.
type operationChunkInsertResult struct {
	inserted    int64
	insertedIDs []string
}

// insertOperationChunk inserts a chunk of operations using multi-row INSERT.
// Uses repository.DBExecutor to work with both dbresolver.DB and dbresolver.Tx.
// Returns the count of inserted rows and their IDs for downstream filtering.
func (r *OperationPostgreSQLRepository) insertOperationChunk(ctx context.Context, db repository.DBExecutor, operations []*Operation) (*operationChunkInsertResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.insert_operation_chunk")
	defer span.End()

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Inserting chunk of %d operations", len(operations)))

	builder := squirrel.Insert(r.tableName).
		Columns(operationColumnList...).
		PlaceholderFormat(squirrel.Dollar)

	for _, op := range operations {
		record := &OperationPostgreSQLModel{}
		record.FromEntity(op)

		builder = builder.Values(
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
			record.Direction,
			record.RouteID,
			record.RouteCode,
			record.RouteDescription,
		)
	}

	builder = builder.Suffix("ON CONFLICT (id) DO NOTHING RETURNING id")

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build bulk insert query", err)

		return nil, err
	}

	// Use QueryContext to retrieve the RETURNING clause results
	querier, ok := db.(repository.DBQuerier)
	if !ok {
		libOpentelemetry.HandleSpanError(span, "DBExecutor does not support QueryContext", repository.ErrQueryContextNotSupported)

		return nil, repository.ErrQueryContextNotSupported
	}

	rows, err := querier.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute bulk insert", err)

		return nil, err
	}
	defer rows.Close()

	result := &operationChunkInsertResult{
		insertedIDs: make([]string, 0, len(operations)),
	}

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan inserted ID", err)

			return nil, err
		}

		result.insertedIDs = append(result.insertedIDs, id)
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Error iterating inserted IDs", err)

		return nil, err
	}

	result.inserted = int64(len(result.insertedIDs))

	return result, nil
}

// FindAll retrieves Operations entities from the database.
func (r *OperationPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

	decodedCursor := libHTTP.Cursor{Direction: libHTTP.CursorDirectionNext}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode cursor", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to decode cursor: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select(operationColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("transaction_id = ?", transactionID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, err = applyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to apply cursor pagination", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to get operations on repo", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get operations on repo: %v", err))

		return nil, libHTTP.CursorPagination{}, err
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
			&operation.Direction,
			&operation.RouteID,
			&operation.RouteCode,
			&operation.RouteDescription,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(operations) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)

	operations = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.Direction, operations, filter.Limit)

	cur := libHTTP.CursorPagination{}
	if len(operations) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.Direction, operations[0].ID, operations[len(operations)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to calculate cursor: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}

// ListByIDs retrieves Operation entities from the database using the provided IDs.
func (r *OperationPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_operations_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	var operations []*Operation

	findAll := squirrel.Select(operationColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_ids.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to get operations on repo", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get operations on repo: %v", err))

		return nil, err
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
			&operation.Direction,
			&operation.RouteID,
			&operation.RouteCode,
			&operation.RouteDescription,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows: %v", err))

		return nil, err
	}

	return operations, nil
}

// Find retrieves a Operation entity from the database using the provided ID.
func (r *OperationPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	findOne := squirrel.Select(operationColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("transaction_id = ?", transactionID)).
		Where(squirrel.Expr("id = ?", id)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findOne.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	operation := &OperationPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

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
		&operation.Direction,
		&operation.RouteID,
		&operation.RouteCode,
		&operation.RouteDescription,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation not found", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Operation not found: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return nil, err
	}

	return operation.ToEntity(), nil
}

// FindByAccount retrieves a Operation entity from the database using the provided account ID.
func (r *OperationPostgreSQLRepository) FindByAccount(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations_by_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	findOne := squirrel.Select(operationColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("account_id = ?", accountID)).
		Where(squirrel.Expr("id = ?", id)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findOne.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	operation := &OperationPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all_by_account.query")

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
		&operation.Direction,
		&operation.RouteID,
		&operation.RouteCode,
		&operation.RouteDescription,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation not found", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Operation not found: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return nil, err
	}

	return operation.ToEntity(), nil
}

// Update an Operation entity into Postgresql and returns the Operation updated.
func (r *OperationPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID, operation *Operation) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_operation")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	record := &OperationPostgreSQLModel{}
	record.FromEntity(operation)

	qb := squirrel.Update(r.tableName).
		PlaceholderFormat(squirrel.Dollar)

	if operation.Description != "" {
		qb = qb.Set("description", record.Description)
	}

	record.UpdatedAt = time.Now()

	qb = qb.Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID, "transaction_id": transactionID, "id": id}).
		Where(squirrel.Eq{"deleted_at": nil})

	query, args, err := qb.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build update query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build update query: %v", err))

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows affected: %v", err))

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update operation. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to update operation. Rows affected is 0: %v", err))

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Operation entity from the database using the provided IDs.
func (r *OperationPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_operation")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return err
	}

	qb := squirrel.Update(r.tableName).
		Set("deleted_at", squirrel.Expr("now()"))
	qb = qb.Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID, "id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := qb.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build delete query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build delete query: %v", err))

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute database query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute database query: %v", err))

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows affected: %v", err))

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete operation. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to delete operation. Rows affected is 0: %v", err))

		return err
	}

	return nil
}

// FindAllByAccount retrieves Operations entities from the database using the provided account ID.
func (r *OperationPostgreSQLRepository) FindAllByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, opFilter OperationFilter, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations_by_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

	decodedCursor := libHTTP.Cursor{Direction: libHTTP.CursorDirectionNext}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode cursor", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to decode cursor: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select(operationColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("account_id = ?", accountID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	if !libCommons.IsNilOrEmpty(opFilter.OperationType) {
		findAll = findAll.Where(squirrel.Expr("type = ?", *opFilter.OperationType))
	}

	if !libCommons.IsNilOrEmpty(opFilter.Direction) {
		findAll = findAll.Where(squirrel.Expr("direction = ?", *opFilter.Direction))
	}

	if !libCommons.IsNilOrEmpty(opFilter.RouteID) {
		findAll = findAll.Where(squirrel.Expr("route_id = ?", *opFilter.RouteID))
	}

	if !libCommons.IsNilOrEmpty(opFilter.RouteCode) {
		findAll = findAll.Where(squirrel.Expr("route_code = ?", *opFilter.RouteCode))
	}

	findAll, err = applyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to apply cursor pagination", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("FindAllByAccount query: %s with args: %v", query, args))

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all_by_account.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to query database", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to query database: %v", err))

		return nil, libHTTP.CursorPagination{}, err
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
			&operation.Direction,
			&operation.RouteID,
			&operation.RouteCode,
			&operation.RouteDescription,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(operations) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)

	operations = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.Direction, operations, filter.Limit)

	cur := libHTTP.CursorPagination{}
	if len(operations) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.Direction, operations[0].ID, operations[len(operations)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to calculate cursor: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}

// FindLastOperationBeforeTimestamp finds the last operation for a specific balance before a given timestamp.
// This is used for point-in-time balance queries to determine the balance state at a specific moment.
func (r *OperationPostgreSQLRepository) FindLastOperationBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID, balanceID uuid.UUID, timestamp time.Time) (*Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_last_operation_before_timestamp")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	// Build query to find the last operation for this balance before the timestamp
	// Uses optimized column list (9 columns vs 26) to match idx_operation_account_balance_pit
	findQuery := squirrel.Select(operationPointInTimeColumns...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"account_id": accountID}).
		Where(squirrel.Eq{"balance_id": balanceID}).
		Where(squirrel.LtOrEq{"created_at": timestamp}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC", "balance_version_after DESC", "id DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("FindLastOperationBeforeTimestamp query: %s with args: %v", query, args))

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
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No operation found before timestamp", err)
			logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("No operation found for balance %s before timestamp %s", balanceID, timestamp))

			return nil, nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return nil, err
	}

	return operation.ToEntity(), nil
}

// FindLastOperationsForAccountBeforeTimestamp finds the last operation for each balance of an account before a given timestamp.
// This is used for point-in-time account balance queries to get all balance states at a specific moment.
func (r *OperationPostgreSQLRepository) FindLastOperationsForAccountBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_last_operations_for_account_before_timestamp")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

	// Cursor pagination setup
	decodedCursor := libHTTP.Cursor{Direction: libHTTP.CursorDirectionNext}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode cursor", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to decode cursor: %v", err))

			return nil, libHTTP.CursorPagination{}, err
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

	outerQuery, err = applyCursorPagination(outerQuery, decodedCursor, orderDirection, filter.Limit)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to apply cursor pagination", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to apply cursor pagination: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, err := outerQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build outer query", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build outer query: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("FindLastOperationsForAccountBeforeTimestamp query: %s with args: %v", query, args))

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_last_operations_for_account_before_timestamp.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to query database", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to query database: %v", err))

		return nil, libHTTP.CursorPagination{}, err
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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(operations) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)

	operations = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.Direction, operations, filter.Limit)

	cur := libHTTP.CursorPagination{}
	if len(operations) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.Direction, operations[0].ID, operations[len(operations)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to calculate cursor: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}
