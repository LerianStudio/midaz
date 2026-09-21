// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/repository"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

// OperationFilter holds optional filters for listing operations.
type OperationFilter struct {
	OperationType *string
	Direction     *string
	RouteID       *string
	RouteCode     *string
}

// Repository provides an interface for operations related to operation template entities.
// It defines methods for creating, retrieving, updating, and deleting operation templates.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=operation.postgresql_mock.go --package=operation . Repository
type Repository interface {
	// Create preserves a caller-provided engine apply time in recorded_at and
	// otherwise stamps the repository clock once for the inserted row.
	Create(ctx context.Context, operation *Operation) (*Operation, error)
	CreateBulk(ctx context.Context, operations []*Operation) (*repository.BulkInsertResult, error)
	// CreateBulkTx applies one repository timestamp to every row that does not
	// already carry the engine apply time.
	CreateBulkTx(ctx context.Context, tx repository.DBExecutor, operations []*Operation) (*repository.BulkInsertResult, error)
	FindAll(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
	FindAllByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, opFilter OperationFilter, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
	Find(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID) (*Operation, error)
	FindByAccount(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*Operation, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Operation, error)
	Update(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID, operation *Operation) (*Operation, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	// Point-in-time balance queries
	// ListLatestByBalances returns, for each requested balance, the operation holding
	// its high-water mark: the newest balance-affecting, non-deleted operation of that
	// balance, carrying the after-values and overdraft snapshot it left behind. The
	// result is keyed by balance ID; a balance with no eligible operation is ABSENT
	// from the map rather than an error, which is what a brand-new account looks like.
	//
	// The read always targets the primary: its purpose is to tell whether the balance
	// row is behind the operation trail, and a lagging replica would report a stale
	// high-water mark and blind that check.
	ListLatestByBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, refs []BalanceHWMRef) (map[string]*Operation, error)
	FindLastOperationBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID, balanceID uuid.UUID, timestamp time.Time) (*Operation, error)
	FindLastOperationsForAccountBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error)
}

// BalanceHWMRef addresses one balance in a high-water-mark lookup. The account ID
// travels with the balance ID because it is the leading column of
// idx_operation_account_balance_pit_recorded: without it the lookup cannot match the index
// prefix.
type BalanceHWMRef struct {
	AccountID uuid.UUID
	BalanceID uuid.UUID
}

// OperationPostgreSQLRepository is a Postgresql-specific implementation of the OperationRepository.
type OperationPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
	clock         func() time.Time
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
	// snapshot is a JSONB column (NOT NULL DEFAULT '{}') carrying system-generated
	// per-operation context (overdraft before/after, future audit fields).
	// Appended at the end to minimize scan-site churn — every site extends by
	// one trailing field.
	"snapshot",
	"recorded_at",
}

// operationColumns is derived from operationColumnList for use with squirrel.Select.
var operationColumns = strings.Join(operationColumnList, ", ")

// PointInTimeRecordedAtExpression is the authoritative PIT axis. Legacy rows
// written before recorded_at was introduced retain their created_at behavior.
const PointInTimeRecordedAtExpression = "COALESCE(recorded_at, created_at)"

// operationPointInTimeColumns contains only the columns needed for point-in-time balance queries.
// These columns are served by idx_operation_account_balance_pit_recorded via heap fetches (the index
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
	PointInTimeRecordedAtExpression + " AS recorded_at",
	// snapshot propagates through point-in-time queries so historical balance
	// reconstruction surfaces the same overdraft context as live reads.
	"snapshot",
}

var operationPointInTimeProjectedColumns = []string{
	"id", "balance_id", "account_id", "asset_code", "balance_key",
	"available_balance_after", "on_hold_balance_after", "balance_version_after",
	"recorded_at", "snapshot",
}

// NewOperationPostgreSQLRepository returns a new instance of OperationPostgreSQLRepository using the given Postgres connection.
func NewOperationPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *OperationPostgreSQLRepository {
	c := &OperationPostgreSQLRepository{
		connection: pc,
		tableName:  "operation",
		clock:      time.Now,
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

func (r *OperationPostgreSQLRepository) now() time.Time {
	if r.clock == nil {
		return time.Now()
	}

	return r.clock()
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
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operation")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &OperationPostgreSQLModel{}
	record.FromEntity(operation)

	if !record.RecordedAt.Valid {
		record.RecordedAt = sql.NullTime{Time: r.now(), Valid: true}
	}

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
			record.Snapshot,
			record.RecordedAt,
		).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := insert.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build insert query", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			libOpentelemetry.HandleSpanEvent(span, "Operation already exists, skipping duplicate insert (idempotent retry)")

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to execute query", err)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityOperation)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create operation. Rows affected is 0", err)

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
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_bulk_operations")
	defer span.End()

	// Early return for empty input before acquiring DB connection
	if len(operations) == 0 {
		return &repository.BulkInsertResult{}, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
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
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

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

	recordedAt := r.now()

	// Sort by ID (string UUID) to prevent deadlocks in concurrent bulk operations
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].ID < operations[j].ID
	})

	result := &repository.BulkInsertResult{
		Attempted:   int64(len(operations)),
		InsertedIDs: make([]string, 0, len(operations)),
	}

	// Chunk into bulks of ~1,000 rows to stay within PostgreSQL's parameter limit
	// Operation has 32 columns, so 1000 rows = 32,000 parameters (under 65,535 limit)
	const chunkSize = 1000

	for i := 0; i < len(operations); i += chunkSize {
		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			libOpentelemetry.HandleSpanError(span, "Context cancelled during bulk insert", ctx.Err())
			// Return partial result; Ignored stays 0 since remaining items were not processed
			return result, ctx.Err()
		default:
		}

		end := min(i+chunkSize, len(operations))

		chunkResult, err := r.insertOperationChunk(ctx, db, operations[i:end], recordedAt)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to insert operation chunk", err)
			// Return partial result; Ignored stays 0 since remaining items were not processed (not duplicates)
			return result, err
		}

		result.Inserted += chunkResult.inserted
		result.InsertedIDs = append(result.InsertedIDs, chunkResult.insertedIDs...)
	}

	result.Ignored = result.Attempted - result.Inserted

	logger.Log(ctx, libLog.LevelDebug, "Bulk insert operations completed",
		libLog.String("scope", logSuffix),
		libLog.Int("attempted", int(result.Attempted)),
		libLog.Int("inserted", int(result.Inserted)),
		libLog.Int("ignored", int(result.Ignored)))

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
func (r *OperationPostgreSQLRepository) insertOperationChunk(ctx context.Context, db repository.DBExecutor, operations []*Operation, recordedAt time.Time) (*operationChunkInsertResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.insert_operation_chunk")
	defer span.End()

	logger.Log(ctx, libLog.LevelDebug, "Inserting chunk of operations", libLog.Int("count", len(operations)))

	builder := squirrel.Insert(r.tableName).
		Columns(operationColumnList...).
		PlaceholderFormat(squirrel.Dollar)

	for _, op := range operations {
		record := &OperationPostgreSQLModel{}
		record.FromEntity(op)

		if !record.RecordedAt.Valid {
			record.RecordedAt = sql.NullTime{Time: recordedAt, Valid: true}
		}

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
			record.Snapshot,
			record.RecordedAt,
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
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

	decodedCursor := libHTTP.Cursor{Direction: libHTTP.CursorDirectionNext}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select(operationColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("transaction_id = ?", transactionID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if !filter.StartDate.IsZero() {
		findAll = findAll.
			Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
			Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)})
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

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get operations on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			operation OperationPostgreSQLModel
			direction sql.NullString
		)

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
			&operation.RecordedAt,
			&operation.UpdatedAt,
			&operation.DeletedAt,
			&operation.Route,
			&operation.BalanceAffected,
			&operation.BalanceKey,
			&operation.VersionBalance,
			&operation.VersionBalanceAfter,
			&direction,
			&operation.RouteID,
			&operation.RouteCode,
			&operation.RouteDescription,
			&operation.Snapshot,
			&operation.RecordedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operation.Direction = direction.String

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

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

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}

// ListByIDs retrieves Operation entities from the database using the provided IDs.
func (r *OperationPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Operation, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_operations_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

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

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get operations on repo", err)

		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			operation OperationPostgreSQLModel
			direction sql.NullString
		)

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
			&direction,
			&operation.RouteID,
			&operation.RouteCode,
			&operation.RouteDescription,
			&operation.Snapshot,
			&operation.RecordedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, err
		}

		operation.Direction = direction.String

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		return nil, err
	}

	return operations, nil
}

// Find retrieves a Operation entity from the database using the provided ID.
func (r *OperationPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID) (*Operation, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

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

		return nil, err
	}

	operation := &OperationPostgreSQLModel{}

	var direction sql.NullString

	row := db.QueryRowContext(ctx, query, args...)

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
		&direction,
		&operation.RouteID,
		&operation.RouteCode,
		&operation.RouteDescription,
		&operation.Snapshot,
		&operation.RecordedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityOperation)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		return nil, err
	}

	operation.Direction = direction.String

	return operation.ToEntity(), nil
}

// FindByAccount retrieves a Operation entity from the database using the provided account ID.
func (r *OperationPostgreSQLRepository) FindByAccount(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*Operation, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations_by_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

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

		return nil, err
	}

	operation := &OperationPostgreSQLModel{}

	var direction sql.NullString

	row := db.QueryRowContext(ctx, query, args...)

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
		&direction,
		&operation.RouteID,
		&operation.RouteCode,
		&operation.RouteDescription,
		&operation.Snapshot,
		&operation.RecordedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityOperation)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		return nil, err
	}

	operation.Direction = direction.String

	return operation.ToEntity(), nil
}

// Update an Operation entity into Postgresql and returns the Operation updated.
func (r *OperationPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID, operation *Operation) (*Operation, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_operation")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

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

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute query", err)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityOperation)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update operation. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Operation entity from the database using the provided IDs.
func (r *OperationPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_operation")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

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

		return err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute database query", err)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityOperation)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete operation. Rows affected is 0", err)

		return err
	}

	return nil
}

func applyDirectionFallbackFilter(findAll squirrel.SelectBuilder, direction string) squirrel.SelectBuilder {
	switch strings.ToLower(direction) {
	case constant.DirectionDebit:
		return findAll.Where(squirrel.Expr(
			"(direction = ? OR ((direction IS NULL OR direction = '') AND UPPER(type) IN (?, ?)))",
			constant.DirectionDebit,
			constant.DEBIT,
			constant.ONHOLD,
		))
	case constant.DirectionCredit:
		return findAll.Where(squirrel.Expr(
			"(direction = ? OR ((direction IS NULL OR direction = '') AND UPPER(type) IN (?, ?)))",
			constant.DirectionCredit,
			constant.CREDIT,
			constant.RELEASE,
		))
	default:
		return findAll.Where(squirrel.Expr("direction = ?", direction))
	}
}

// FindAllByAccount retrieves Operations entities from the database using the provided account ID.
func (r *OperationPostgreSQLRepository) FindAllByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, opFilter OperationFilter, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations_by_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

	decodedCursor := libHTTP.Cursor{Direction: libHTTP.CursorDirectionNext}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select(operationColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("account_id = ?", accountID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if !filter.StartDate.IsZero() {
		findAll = findAll.
			Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
			Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)})
	}

	if !libCommons.IsNilOrEmpty(opFilter.OperationType) {
		findAll = findAll.Where(squirrel.Expr("type = ?", *opFilter.OperationType))
	}

	if !libCommons.IsNilOrEmpty(opFilter.Direction) {
		findAll = applyDirectionFallbackFilter(findAll, *opFilter.Direction)
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

	logger.Log(ctx, libLog.LevelDebug, "FindAllByAccount query assembled", libLog.String("query", query))

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to query database", err)

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			operation OperationPostgreSQLModel
			direction sql.NullString
		)

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
			&direction,
			&operation.RouteID,
			&operation.RouteCode,
			&operation.RouteDescription,
			&operation.Snapshot,
			&operation.RecordedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operation.Direction = direction.String

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

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

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}

// ListLatestByBalances resolves the high-water-mark operation of each requested balance.
//
// The high-water mark is decided by balance_version_after, the only field that grows
// monotonically with the balance: it is assigned inside the serialized Lua execution,
// while created_at is stamped in Go BEFORE that execution. Two concurrent transactions on
// one balance can therefore land in the opposite order in the two fields, and picking the
// newest created_at would then elect an intermediate version as the mark — which reads as
// "the row is only slightly behind" and rebuilds the seed short, permanently.
//
// Cost of that choice, accepted deliberately: idx_operation_account_balance_pit_recorded orders by
// recording time, so the balance's entries under the (organization, ledger, account, balance)
// prefix are scanned and sorted for a top-1 instead of being read in index order. The query
// runs only on a cache miss; a second index dedicated to the version-first HWM path is outside
// this change's point-in-time scope.
func (r *OperationPostgreSQLRepository) ListLatestByBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, refs []BalanceHWMRef) (hwm map[string]*Operation, err error) {
	if len(refs) == 0 {
		return map[string]*Operation{}, nil
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	db, err := r.getDB(ctx)
	if err != nil {
		return nil, err
	}

	// A read-only transaction is what makes dbresolver target the primary. Opening it is
	// fail-closed: no replica fallback, because a stale high-water mark reads as "the
	// balance row is up to date" and silently disarms the caller's guard.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open read-only primary transaction for balance high-water marks: %w", err)
	}

	defer func() {
		// A failed read ends the transaction by rolling it back: committing work that
		// produced nothing is the habit this closure would teach whoever copies it. A
		// rollback that itself fails joins the error it could not undo, so the caller
		// still sees the original failure.
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback read-only primary transaction for balance high-water marks: %w", rollbackErr))
			}

			return
		}

		if commitErr := tx.Commit(); commitErr != nil {
			err = fmt.Errorf("close read-only primary transaction for balance high-water marks: %w", commitErr)
		}
	}()

	query, args, err := buildBalanceHWMQuery(r.tableName, organizationID, ledgerID, refs)
	if err != nil {
		return nil, fmt.Errorf("build balance high-water mark query: %w", err)
	}

	logger.Log(ctx, libLog.LevelDebug, "ListLatestByBalances query assembled", libLog.String("query", query))

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query balance high-water marks: %w", err)
	}

	defer rows.Close()

	result := make(map[string]*Operation, len(refs))

	for rows.Next() {
		var operation OperationPointInTimeModel

		if scanErr := rows.Scan(
			&operation.ID,
			&operation.BalanceID,
			&operation.AccountID,
			&operation.AssetCode,
			&operation.BalanceKey,
			&operation.AvailableBalanceAfter,
			&operation.OnHoldBalanceAfter,
			&operation.VersionBalanceAfter,
			&operation.RecordedAt,
			&operation.Snapshot,
		); scanErr != nil {
			return nil, fmt.Errorf("scan balance high-water mark: %w", scanErr)
		}

		result[operation.BalanceID] = operation.ToEntity()
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate balance high-water marks: %w", rowsErr)
	}

	return result, nil
}

// buildBalanceHWMQuery assembles the high-water-mark lookup. DISTINCT ON keeps the first
// row per balance, and the ORDER BY that decides which row that is leads with
// balance_version_after — deliberately diverging from idx_operation_account_balance_pit_recorded,
// which leads with recording time. See ListLatestByBalances for why the version has to win
// and what that ordering costs.
func buildBalanceHWMQuery(tableName string, organizationID, ledgerID uuid.UUID, refs []BalanceHWMRef) (string, []any, error) {
	pairs := make(squirrel.Or, 0, len(refs))
	for _, ref := range refs {
		pairs = append(pairs, squirrel.Eq{
			"account_id": ref.AccountID,
			"balance_id": ref.BalanceID,
		})
	}

	return squirrel.Select("DISTINCT ON (balance_id) "+strings.Join(operationPointInTimeColumns, ", ")).
		From(tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(pairs).
		Where(squirrel.Eq{"deleted_at": nil}).
		// Annotation rows move no money, so they hold no balance state to compare against.
		Where(squirrel.Eq{"balance_affected": true}).
		// Version first: it is the balance's monotonic clock. created_at and id only
		// break a tie, which distinct operations of one balance can reach solely on an
		// already-forked trail.
		OrderBy("balance_id", "balance_version_after DESC", "created_at DESC", "id DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
}

// FindLastOperationBeforeTimestamp finds the last operation for a specific balance before a given timestamp.
// This is used for point-in-time balance queries to determine the balance state at a specific moment.
func (r *OperationPostgreSQLRepository) FindLastOperationBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID, balanceID uuid.UUID, timestamp time.Time) (*Operation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_last_operation_before_timestamp")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		return nil, err
	}

	// Build query to find the last operation for this balance before the timestamp
	// Uses the optimized PIT column list aligned with idx_operation_account_balance_pit_recorded.
	findQuery := squirrel.Select(operationPointInTimeColumns...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"account_id": accountID}).
		Where(squirrel.Eq{"balance_id": balanceID}).
		Where(squirrel.Expr(PointInTimeRecordedAtExpression+" <= ?", timestamp)).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy(PointInTimeRecordedAtExpression+" DESC", "balance_version_after DESC", "id DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "FindLastOperationBeforeTimestamp query assembled", libLog.String("query", query))

	row := db.QueryRowContext(ctx, query, args...)

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
		&operation.RecordedAt,
		&operation.Snapshot,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No operation found before timestamp", err)

			return nil, nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		return nil, err
	}

	return operation.ToEntity(), nil
}

// FindLastOperationsForAccountBeforeTimestamp finds the last operation for each balance of an account before a given timestamp.
// This is used for point-in-time account balance queries to get all balance states at a specific moment.
func (r *OperationPostgreSQLRepository) FindLastOperationsForAccountBeforeTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_last_operations_for_account_before_timestamp")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
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
			return nil, libHTTP.CursorPagination{}, err
		}
	}

	// Build query using DISTINCT ON to get the last operation per balance_id
	// PostgreSQL DISTINCT ON returns the first row for each distinct value based on ORDER BY
	// Uses the optimized PIT column list aligned with the expression index.
	findQuery := squirrel.Select("DISTINCT ON (balance_id) "+strings.Join(operationPointInTimeColumns, ", ")).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"account_id": accountID}).
		Where(squirrel.Expr(PointInTimeRecordedAtExpression+" <= ?", timestamp)).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("balance_id", PointInTimeRecordedAtExpression+" DESC", "balance_version_after DESC", "id DESC").
		PlaceholderFormat(squirrel.Dollar)

	// Apply pagination on the outer query
	outerQuery := squirrel.Select(operationPointInTimeProjectedColumns...).
		FromSelect(findQuery, "sub").
		PlaceholderFormat(squirrel.Dollar)

	outerQuery, err = applyCursorPagination(outerQuery, decodedCursor, orderDirection, filter.Limit)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to apply cursor pagination", err)
		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, err := outerQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build outer query", err)
		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Log(ctx, libLog.LevelDebug, "FindLastOperationsForAccountBeforeTimestamp query assembled", libLog.String("query", query))

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to query database", err)
		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

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
			&operation.RecordedAt,
			&operation.Snapshot,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)
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
			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}
