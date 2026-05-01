// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v5/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/repository"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

var transactionColumnList = []string{
	"id",
	"parent_transaction_id",
	"description",
	"status",
	"status_description",
	"amount",
	"asset_code",
	"chart_of_accounts_group_name",
	"ledger_id",
	"organization_id",
	"body",
	"created_at",
	"updated_at",
	"deleted_at",
	"route",
	"route_id",
}

var transactionColumnListPrefixed = []string{
	"t.id",
	"t.parent_transaction_id",
	"t.description",
	"t.status",
	"t.status_description",
	"t.amount",
	"t.asset_code",
	"t.chart_of_accounts_group_name",
	"t.ledger_id",
	"t.organization_id",
	"t.body",
	"t.created_at",
	"t.updated_at",
	"t.deleted_at",
	"t.route",
	"t.route_id",
}

// operationColumnListPrefixed mirrors operation.operationColumnList with the "o."
// table prefix for JOIN queries. Shared by FindWithOperations and
// FindOrListAllWithOperations so that column-set drift is impossible.
// When operation.operationColumnList gains a column, add the prefixed entry here
// at the same trailing position to keep scan-site alignment.
var operationColumnListPrefixed = []string{
	"o.id", "o.transaction_id", "o.description", "o.type", "o.asset_code",
	"o.amount", "o.available_balance", "o.on_hold_balance", "o.available_balance_after",
	"o.on_hold_balance_after", "o.status", "o.status_description", "o.account_id",
	"o.account_alias", "o.balance_id", "o.chart_of_accounts", "o.organization_id",
	"o.ledger_id", "o.created_at", "o.updated_at", "o.deleted_at", "o.route",
	"o.balance_affected", "o.balance_key", "o.balance_version_before", "o.balance_version_after",
	"o.direction", "o.route_id", "o.route_code", "o.route_description",
	"o.snapshot",
}

// Repository provides an interface for operations related to transaction template entities.
// It defines methods for creating, retrieving, updating, and deleting transactions.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=transaction.postgresql_mock.go --package=transaction . Repository
type Repository interface {
	Create(ctx context.Context, transaction *Transaction) (*Transaction, error)
	CreateBulk(ctx context.Context, transactions []*Transaction) (*repository.BulkInsertResult, error)
	CreateBulkTx(ctx context.Context, tx repository.DBExecutor, transactions []*Transaction) (*repository.BulkInsertResult, error)
	UpdateBulk(ctx context.Context, transactions []*Transaction) (*repository.BulkUpdateResult, error)
	UpdateBulkTx(ctx context.Context, tx repository.DBExecutor, transactions []*Transaction) (*repository.BulkUpdateResult, error)
	BeginTx(ctx context.Context) (repository.DBTransaction, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
	FindByParentID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*Transaction, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Transaction, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *Transaction) (*Transaction, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	FindWithOperations(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
	FindOrListAllWithOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error)
	CountByFilters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter CountFilter) (int64, error)
}

// transactionColumns is derived from transactionColumnList for use with squirrel.Select.
var transactionColumns = strings.Join(transactionColumnList, ", ")

// TransactionPostgreSQLRepository is a Postgresql-specific implementation of the TransactionRepository.
type TransactionPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewTransactionPostgreSQLRepository returns a new instance of TransactionPostgreSQLRepository using the given Postgres connection.
func NewTransactionPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *TransactionPostgreSQLRepository {
	c := &TransactionPostgreSQLRepository{
		connection: pc,
		tableName:  "transaction",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *TransactionPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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

// BeginTx starts a new database transaction for atomic multi-table operations.
// The caller is responsible for calling Commit() or Rollback() on the returned transaction.
// This enables atomic bulk inserts across transactions and operations tables.
func (r *TransactionPostgreSQLRepository) BeginTx(ctx context.Context) (repository.DBTransaction, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	return tx, nil
}

// Create a new Transaction entity into Postgresql and returns it.
func (r *TransactionPostgreSQLRepository) Create(ctx context.Context, transaction *Transaction) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_transaction")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	record := &TransactionPostgreSQLModel{}
	record.FromEntity(transaction)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, `INSERT INTO transaction VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *`,
		record.ID,
		record.ParentTransactionID,
		record.Description,
		record.Status,
		record.StatusDescription,
		record.Amount,
		record.AssetCode,
		record.ChartOfAccountsGroupName,
		record.LedgerID,
		record.OrganizationID,
		record.Body,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
		record.Route,
		record.RouteID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			libOpentelemetry.HandleSpanEvent(spanExec, "Transaction already exists, skipping duplicate insert (idempotent retry)")

			logger.Log(ctx, libLog.LevelInfo, "Transaction already exists, skipping duplicate insert (idempotent retry)")

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows affected: %v", err))

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to create transaction. Rows affected is 0: %v", err))

		return nil, err
	}

	return record.ToEntity(), nil
}

// CreateBulk inserts multiple transactions in bulk using multi-row INSERT with ON CONFLICT DO NOTHING.
// Returns BulkInsertResult with counts of attempted, inserted, and ignored (duplicate) rows.
// Transactions are sorted by ID before insert to prevent deadlocks in concurrent scenarios.
// Large bulks are automatically chunked to stay within PostgreSQL's parameter limits.
//
// NOTE: Chunks are committed independently. If chunk N fails, chunks 1 to N-1 remain committed.
// On error, partial results are returned along with the error. Retry is safe due to idempotency.
// On error, only Inserted is reliable; Ignored remains 0 since unprocessed chunks are not duplicates.
//
// NOTE: The input slice is sorted in-place by ID. Callers should not rely on original order after this call.
func (r *TransactionPostgreSQLRepository) CreateBulk(ctx context.Context, transactions []*Transaction) (*repository.BulkInsertResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_bulk_transactions")
	defer span.End()

	// Early return for empty input before acquiring DB connection
	if len(transactions) == 0 {
		return &repository.BulkInsertResult{}, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	return r.createBulkInternal(ctx, db, transactions, "postgres.create_bulk_transactions_internal", "")
}

// CreateBulkTx inserts multiple transactions in bulk using a caller-provided transaction.
// This allows the caller to control transaction boundaries for atomic multi-table operations.
// Returns BulkInsertResult with counts of attempted, inserted, and ignored (duplicate) rows.
// Transactions are sorted by ID before insert to prevent deadlocks in concurrent scenarios.
// Large bulks are automatically chunked to stay within PostgreSQL's parameter limits.
//
// NOTE: The caller is responsible for calling Commit() or Rollback() on the transaction.
// On error, partial results are returned along with the error. The caller should rollback.
// On error, only Inserted is reliable; Ignored remains 0 since unprocessed chunks are not duplicates.
//
// NOTE: The input slice is sorted in-place by ID. Callers should not rely on original order after this call.
func (r *TransactionPostgreSQLRepository) CreateBulkTx(ctx context.Context, tx repository.DBExecutor, transactions []*Transaction) (*repository.BulkInsertResult, error) {
	if tx == nil {
		return nil, repository.ErrNilDBExecutor
	}

	return r.createBulkInternal(ctx, tx, transactions, "postgres.create_bulk_transactions_tx", " (tx)")
}

// createBulkInternal contains the shared logic for CreateBulk and CreateBulkTx.
// It validates input, sorts transactions by ID to prevent deadlocks, and inserts in chunks.
// Returns partial results on error with Attempted/Inserted counts.
func (r *TransactionPostgreSQLRepository) createBulkInternal(
	ctx context.Context,
	db repository.DBExecutor,
	transactions []*Transaction,
	spanName string,
	logSuffix string,
) (*repository.BulkInsertResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	if len(transactions) == 0 {
		return &repository.BulkInsertResult{}, nil
	}

	// Validate no nil elements to prevent panic during sort or insert
	for i, txn := range transactions {
		if txn == nil {
			err := fmt.Errorf("nil transaction at index %d", i)
			libOpentelemetry.HandleSpanError(span, "Invalid input: nil transaction", err)

			return nil, err
		}
	}

	// Sort by ID (string UUID) to prevent deadlocks in concurrent bulk operations
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].ID < transactions[j].ID
	})

	result := &repository.BulkInsertResult{
		Attempted:   int64(len(transactions)),
		InsertedIDs: make([]string, 0, len(transactions)),
	}

	// Chunk into bulks of ~1,000 rows to stay within PostgreSQL's parameter limit
	// Transaction has 15 columns, so 1000 rows = 15,000 parameters (under 65,535 limit)
	const chunkSize = 1000

	for i := 0; i < len(transactions); i += chunkSize {
		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			libOpentelemetry.HandleSpanError(span, "Context cancelled during bulk insert", ctx.Err())
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Context cancelled during bulk insert: %v", ctx.Err()))

			// Return partial result; Ignored stays 0 since remaining items were not processed
			return result, ctx.Err()
		default:
		}

		end := min(i+chunkSize, len(transactions))

		chunkResult, err := r.insertTransactionChunk(ctx, db, transactions[i:end])
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to insert transaction chunk", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to insert transaction chunk: %v", err))

			// Return partial result; Ignored stays 0 since remaining items were not processed (not duplicates)
			return result, err
		}

		result.Inserted += chunkResult.inserted
		result.InsertedIDs = append(result.InsertedIDs, chunkResult.insertedIDs...)
	}

	result.Ignored = result.Attempted - result.Inserted

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Bulk insert transactions%s: attempted=%d, inserted=%d, ignored=%d",
		logSuffix, result.Attempted, result.Inserted, result.Ignored))

	return result, nil
}

// chunkInsertResult holds the result of inserting a chunk of rows.
type chunkInsertResult struct {
	inserted    int64
	insertedIDs []string
}

// insertTransactionChunk inserts a chunk of transactions using multi-row INSERT.
// Uses repository.DBExecutor to work with both dbresolver.DB and dbresolver.Tx.
// Returns the count of inserted rows and their IDs for downstream filtering.
func (r *TransactionPostgreSQLRepository) insertTransactionChunk(ctx context.Context, db repository.DBExecutor, transactions []*Transaction) (*chunkInsertResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.insert_transaction_chunk")
	defer span.End()

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Inserting chunk of %d transactions", len(transactions)))

	builder := squirrel.Insert(r.tableName).
		Columns(transactionColumnList...).
		PlaceholderFormat(squirrel.Dollar)

	for _, tx := range transactions {
		record := &TransactionPostgreSQLModel{}
		record.FromEntity(tx)

		builder = builder.Values(
			record.ID,
			record.ParentTransactionID,
			record.Description,
			record.Status,
			record.StatusDescription,
			record.Amount,
			record.AssetCode,
			record.ChartOfAccountsGroupName,
			record.LedgerID,
			record.OrganizationID,
			record.Body,
			record.CreatedAt,
			record.UpdatedAt,
			record.DeletedAt,
			record.Route,
			record.RouteID,
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

	result := &chunkInsertResult{
		insertedIDs: make([]string, 0, len(transactions)),
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

// UpdateBulk updates multiple transactions in bulk using multi-row UPDATE with ON CONFLICT.
// This is used for status transitions (e.g., PENDING -> APPROVED/CANCELED).
// Returns BulkUpdateResult with counts of attempted, updated, and unchanged rows.
// Transactions are sorted by ID before update to prevent deadlocks in concurrent scenarios.
// Large bulks are automatically chunked to stay within PostgreSQL's parameter limits.
//
// NOTE: Chunks are committed independently. If chunk N fails, chunks 1 to N-1 remain committed.
// On error, partial results are returned along with the error. Retry is safe due to idempotency.
//
// NOTE: The input slice is sorted in-place by ID. Callers should not rely on original order after this call.
func (r *TransactionPostgreSQLRepository) UpdateBulk(ctx context.Context, transactions []*Transaction) (*repository.BulkUpdateResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_bulk_transactions")
	defer span.End()

	// Early return for empty input before acquiring DB connection
	if len(transactions) == 0 {
		return &repository.BulkUpdateResult{}, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	return r.updateBulkInternal(ctx, db, transactions, "postgres.update_bulk_transactions_internal", "")
}

// UpdateBulkTx updates multiple transactions in bulk using a caller-provided transaction.
// This allows the caller to control transaction boundaries for atomic multi-table operations.
// Returns BulkUpdateResult with counts of attempted, updated, and unchanged rows.
// Transactions are sorted by ID before update to prevent deadlocks in concurrent scenarios.
// Large bulks are automatically chunked to stay within PostgreSQL's parameter limits.
//
// NOTE: The caller is responsible for calling Commit() or Rollback() on the transaction.
// On error, partial results are returned along with the error. The caller should rollback.
//
// NOTE: The input slice is sorted in-place by ID. Callers should not rely on original order after this call.
func (r *TransactionPostgreSQLRepository) UpdateBulkTx(ctx context.Context, tx repository.DBExecutor, transactions []*Transaction) (*repository.BulkUpdateResult, error) {
	if tx == nil {
		return nil, repository.ErrNilDBExecutor
	}

	return r.updateBulkInternal(ctx, tx, transactions, "postgres.update_bulk_transactions_tx", " (tx)")
}

// updateBulkInternal contains the shared logic for UpdateBulk and UpdateBulkTx.
// It validates input, sorts transactions by ID to prevent deadlocks, and updates in chunks.
// Returns partial results on error with Attempted/Updated counts.
func (r *TransactionPostgreSQLRepository) updateBulkInternal(
	ctx context.Context,
	db repository.DBExecutor,
	transactions []*Transaction,
	spanName string,
	logSuffix string,
) (*repository.BulkUpdateResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	if len(transactions) == 0 {
		return &repository.BulkUpdateResult{}, nil
	}

	// Validate no nil elements to prevent panic during sort or update
	for i, txn := range transactions {
		if txn == nil {
			err := fmt.Errorf("nil transaction at index %d", i)
			libOpentelemetry.HandleSpanError(span, "Invalid input: nil transaction", err)

			return nil, err
		}
	}

	// Sort by ID (string UUID) to prevent deadlocks in concurrent bulk operations
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].ID < transactions[j].ID
	})

	result := &repository.BulkUpdateResult{}

	// Chunk into bulks of ~500 rows to stay within PostgreSQL's parameter limit
	// Transaction update uses 6 columns (id, organization_id, ledger_id, status, status_description, updated_at)
	// so 500 rows = 3,000 parameters (well under PostgreSQL's 65535 limit)
	const chunkSize = 500

	for i := 0; i < len(transactions); i += chunkSize {
		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			libOpentelemetry.HandleSpanError(span, "Context cancelled during bulk update", ctx.Err())
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Context cancelled during bulk update: %v", ctx.Err()))

			// Return partial result with accurate Attempted count
			return result, ctx.Err()
		default:
		}

		end := min(i+chunkSize, len(transactions))
		chunkSize64 := int64(end - i)

		// Increment Attempted before executing to accurately reflect rows submitted
		result.Attempted += chunkSize64

		chunkUpdated, err := r.updateTransactionChunk(ctx, db, transactions[i:end])
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to update transaction chunk", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update transaction chunk: %v", err))

			// Return partial result
			return result, err
		}

		result.Updated += chunkUpdated
	}

	result.Unchanged = result.Attempted - result.Updated

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Bulk update transactions%s: attempted=%d, updated=%d, unchanged=%d",
		logSuffix, result.Attempted, result.Updated, result.Unchanged))

	return result, nil
}

// updateTransactionChunk updates a chunk of transactions using a single batched UPDATE statement.
// Uses UPDATE...FROM (VALUES...) to update all rows in one database round-trip.
// Each transaction is updated only if its status differs from the new status.
// Uses repository.DBExecutor to work with both dbresolver.DB and dbresolver.Tx.
func (r *TransactionPostgreSQLRepository) updateTransactionChunk(ctx context.Context, db repository.DBExecutor, transactions []*Transaction) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_transaction_chunk")
	defer span.End()

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Updating chunk of %d transactions", len(transactions)))

	if len(transactions) == 0 {
		return 0, nil
	}

	// Build parameterized VALUES list for the batched update
	// Each row has 6 values: id, organization_id, ledger_id, status, status_description, updated_at
	// This ensures updates are scoped to the correct organization and ledger
	updatedAt := time.Now()
	args := make([]any, 0, len(transactions)*6)
	valuesClauses := make([]string, 0, len(transactions))

	for i, tx := range transactions {
		record := &TransactionPostgreSQLModel{}
		record.FromEntity(tx)

		// Calculate parameter positions (1-indexed for PostgreSQL)
		baseIdx := i * 6
		valuesClauses = append(valuesClauses, fmt.Sprintf("($%d::uuid, $%d::uuid, $%d::uuid, $%d, $%d, $%d::timestamp)",
			baseIdx+1, baseIdx+2, baseIdx+3, baseIdx+4, baseIdx+5, baseIdx+6))

		args = append(args, record.ID, record.OrganizationID, record.LedgerID, record.Status, record.StatusDescription, updatedAt)
	}

	// Build the batched UPDATE query using UPDATE...FROM (VALUES...)
	// This performs all updates in a single database round-trip
	// The WHERE clause includes organization_id and ledger_id to prevent cross-tenant updates
	query := fmt.Sprintf(`UPDATE %s t
		SET status = v.new_status,
		    status_description = v.new_status_description,
		    updated_at = v.new_updated_at
		FROM (VALUES %s) AS v(id, organization_id, ledger_id, new_status, new_status_description, new_updated_at)
		WHERE t.id = v.id
		  AND t.organization_id = v.organization_id
		  AND t.ledger_id = v.ledger_id
		  AND t.status != v.new_status
		  AND t.deleted_at IS NULL`,
		r.tableName,
		strings.Join(valuesClauses, ", "))

	execResult, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute batched update", err)

		return 0, err
	}

	rowsAffected, err := execResult.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return 0, err
	}

	return rowsAffected, nil
}

// FindAll retrieves Transactions entities from the database.
func (r *TransactionPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_transactions")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	transactions := make([]*Transaction, 0)

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

	findAll := squirrel.Select(transactionColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
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
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var transaction TransactionPostgreSQLModel

		var body *string

		if err := rows.Scan(
			&transaction.ID,
			&transaction.ParentTransactionID,
			&transaction.Description,
			&transaction.Status,
			&transaction.StatusDescription,
			&transaction.Amount,
			&transaction.AssetCode,
			&transaction.ChartOfAccountsGroupName,
			&transaction.LedgerID,
			&transaction.OrganizationID,
			&body,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
			&transaction.DeletedAt,
			&transaction.Route,
			&transaction.RouteID,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &transaction.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to unmarshal body", err)

				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to unmarshal body: %v", err))

				return nil, libHTTP.CursorPagination{}, err
			}
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(transactions) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)

	transactions = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.Direction, transactions, filter.Limit)

	cur := libHTTP.CursorPagination{}
	if len(transactions) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.Direction, transactions[0].ID, transactions[len(transactions)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to calculate cursor: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return transactions, cur, nil
}

// ListByIDs retrieves Transaction entities from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_transactions_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	var transactions []*Transaction

	findAll := squirrel.Select(transactionColumns).
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

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var transaction TransactionPostgreSQLModel

		var body *string

		if err := rows.Scan(
			&transaction.ID,
			&transaction.ParentTransactionID,
			&transaction.Description,
			&transaction.Status,
			&transaction.StatusDescription,
			&transaction.Amount,
			&transaction.AssetCode,
			&transaction.ChartOfAccountsGroupName,
			&transaction.LedgerID,
			&transaction.OrganizationID,
			&body,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
			&transaction.DeletedAt,
			&transaction.Route,
			&transaction.RouteID,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &transaction.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to unmarshal body", err)

				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to unmarshal body: %v", err))

				return nil, err
			}
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows: %v", err))

		return nil, err
	}

	return transactions, nil
}

// Find retrieves a Transaction entity from the database using the provided ID.
func (r *TransactionPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	findOne := squirrel.Select(transactionColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ?", id)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findOne.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	transaction := &TransactionPostgreSQLModel{}

	var body *string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	row := db.QueryRowContext(ctx, query, args...)

	if err := row.Scan(
		&transaction.ID,
		&transaction.ParentTransactionID,
		&transaction.Description,
		&transaction.Status,
		&transaction.StatusDescription,
		&transaction.Amount,
		&transaction.AssetCode,
		&transaction.ChartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&body,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&transaction.DeletedAt,
		&transaction.Route,
		&transaction.RouteID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction not found", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Transaction not found: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return nil, err
	}

	if !libCommons.IsNilOrEmpty(body) {
		err = json.Unmarshal([]byte(*body), &transaction.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal body", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to unmarshal body: %v", err))

			return nil, err
		}
	}

	return transaction.ToEntity(), nil
}

// FindByParentID retrieves a Transaction entity from the database using the provided parent ID.
func (r *TransactionPostgreSQLRepository) FindByParentID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	findOne := squirrel.Select(transactionColumns).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("parent_transaction_id = ?", parentID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findOne.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	transaction := &TransactionPostgreSQLModel{}

	var body *string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	row := db.QueryRowContext(ctx, query, args...)

	if err := row.Scan(
		&transaction.ID,
		&transaction.ParentTransactionID,
		&transaction.Description,
		&transaction.Status,
		&transaction.StatusDescription,
		&transaction.Amount,
		&transaction.AssetCode,
		&transaction.ChartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&body,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&transaction.DeletedAt,
		&transaction.Route,
		&transaction.RouteID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No transaction found", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("No transaction found: %v", err))

			return nil, nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return nil, err
	}

	if !libCommons.IsNilOrEmpty(body) {
		err = json.Unmarshal([]byte(*body), &transaction.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal body", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to unmarshal body: %v", err))

			return nil, err
		}
	}

	return transaction.ToEntity(), nil
}

// Update a Transaction entity into Postgresql and returns the Transaction updated.
func (r *TransactionPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *Transaction) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_transaction")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	record := &TransactionPostgreSQLModel{}
	record.FromEntity(transaction)

	var updates []string

	var args []any

	if transaction.Body.IsEmpty() {
		updates = append(updates, "body = $"+strconv.Itoa(len(args)+1))
		args = append(args, nil)
	}

	if transaction.Description != "" {
		updates = append(updates, "description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Description)
	}

	if !transaction.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE transaction SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows affected: %v", err))

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to update transaction. Rows affected is 0: %v", err))

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Transaction entity from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_transaction")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, "UPDATE transaction SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows affected: %v", err))

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete transaction. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to delete transaction. Rows affected is 0: %v", err))

		return err
	}

	return nil
}

// FindWithOperations retrieves a Transaction and Operations entity from the database using the provided ID .
func (r *TransactionPostgreSQLRepository) FindWithOperations(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_with_operations")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_transaction_with_operations.query")
	defer spanQuery.End()

	selectColumns := append(transactionColumnListPrefixed, operationColumnListPrefixed...)

	findWithOps := squirrel.Select(selectColumns...).
		From(r.tableName + " t").
		InnerJoin("operation o ON t.id = o.transaction_id").
		Where(squirrel.Expr("t.organization_id = ?", organizationID)).
		Where(squirrel.Expr("t.ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("t.id = ?", id)).
		Where(squirrel.Eq{"t.deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findWithOps.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, err
	}
	defer rows.Close()

	newTransaction := &Transaction{}
	operations := make([]*operation.Operation, 0)

	for rows.Next() {
		tran := &TransactionPostgreSQLModel{}
		op := operation.OperationPostgreSQLModel{}

		var body *string

		if err := rows.Scan(
			&tran.ID,
			&tran.ParentTransactionID,
			&tran.Description,
			&tran.Status,
			&tran.StatusDescription,
			&tran.Amount,
			&tran.AssetCode,
			&tran.ChartOfAccountsGroupName,
			&tran.LedgerID,
			&tran.OrganizationID,
			&body,
			&tran.CreatedAt,
			&tran.UpdatedAt,
			&tran.DeletedAt,
			&tran.Route,
			&tran.RouteID,
			&op.ID,
			&op.TransactionID,
			&op.Description,
			&op.Type,
			&op.AssetCode,
			&op.Amount,
			&op.AvailableBalance,
			&op.OnHoldBalance,
			&op.AvailableBalanceAfter,
			&op.OnHoldBalanceAfter,
			&op.Status,
			&op.StatusDescription,
			&op.AccountID,
			&op.AccountAlias,
			&op.BalanceID,
			&op.ChartOfAccounts,
			&op.OrganizationID,
			&op.LedgerID,
			&op.CreatedAt,
			&op.UpdatedAt,
			&op.DeletedAt,
			&op.Route,
			&op.BalanceAffected,
			&op.BalanceKey,
			&op.VersionBalance,
			&op.VersionBalanceAfter,
			&op.Direction,
			&op.RouteID,
			&op.RouteCode,
			&op.RouteDescription,
			&op.Snapshot,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan rows: %v", err))

			return nil, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &tran.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to unmarshal body", err)

				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to unmarshal body: %v", err))

				return nil, err
			}
		}

		newTransaction = tran.ToEntity()
		operations = append(operations, op.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows: %v", err))

		return nil, err
	}

	newTransaction.Operations = operations

	return newTransaction, nil
}

// FindOrListAllWithOperations retrieves a list of transactions from the database using the provided IDs.
//

func (r *TransactionPostgreSQLRepository) FindOrListAllWithOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_or_list_all_with_operations")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

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

	subQuery := squirrel.Select(transactionColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	if len(ids) > 0 {
		subQuery = subQuery.Where(squirrel.Expr("id = ANY(?)", pq.Array(ids)))
	}

	subQuery, err = applyCursorPagination(subQuery, decodedCursor, orderDirection, filter.Limit)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to apply cursor pagination", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	selectColumns := append(transactionColumnListPrefixed, operationColumnListPrefixed...)

	findAll := squirrel.
		Select(selectColumns...).
		FromSelect(subQuery, "t").
		LeftJoin("operation o ON t.id = o.transaction_id").
		PlaceholderFormat(squirrel.Dollar).
		OrderBy("t.id " + orderDirection)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("FindOrListAllWithOperations query: %s with args: %v", query, args))

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	transactions := make([]*Transaction, 0)
	transactionsMap := make(map[uuid.UUID]*Transaction)
	transactionOrder := make([]uuid.UUID, 0)

	for rows.Next() {
		tran := &TransactionPostgreSQLModel{}

		var body *string

		// Nullable pointers for operation fields (LEFT JOIN may return NULL)
		var (
			opID, opTransactionID, opDescription, opType, opAssetCode    *string
			opStatus, opStatusDescription, opAccountID, opAccountAlias   *string
			opBalanceID, opChartOfAccounts, opOrganizationID, opLedgerID *string
			opRoute, opBalanceKey                                        *string
			opAmount, opAvailableBalance, opOnHoldBalance                *decimal.Decimal
			opAvailableBalanceAfter, opOnHoldBalanceAfter                *decimal.Decimal
			opCreatedAt, opUpdatedAt                                     *time.Time
			opDeletedAt                                                  sql.NullTime
			opBalanceAffected                                            *bool
			opVersionBalance, opVersionBalanceAfter                      *int64
			opDirection, opRouteID, opRouteCode, opRouteDescription      *string
			opSnapshot                                                   json.RawMessage
		)

		if err := rows.Scan(
			&tran.ID,
			&tran.ParentTransactionID,
			&tran.Description,
			&tran.Status,
			&tran.StatusDescription,
			&tran.Amount,
			&tran.AssetCode,
			&tran.ChartOfAccountsGroupName,
			&tran.LedgerID,
			&tran.OrganizationID,
			&body,
			&tran.CreatedAt,
			&tran.UpdatedAt,
			&tran.DeletedAt,
			&tran.Route,
			&tran.RouteID,
			&opID,
			&opTransactionID,
			&opDescription,
			&opType,
			&opAssetCode,
			&opAmount,
			&opAvailableBalance,
			&opOnHoldBalance,
			&opAvailableBalanceAfter,
			&opOnHoldBalanceAfter,
			&opStatus,
			&opStatusDescription,
			&opAccountID,
			&opAccountAlias,
			&opBalanceID,
			&opChartOfAccounts,
			&opOrganizationID,
			&opLedgerID,
			&opCreatedAt,
			&opUpdatedAt,
			&opDeletedAt,
			&opRoute,
			&opBalanceAffected,
			&opBalanceKey,
			&opVersionBalance,
			&opVersionBalanceAfter,
			&opDirection,
			&opRouteID,
			&opRouteCode,
			&opRouteDescription,
			&opSnapshot,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan rows: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &tran.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to unmarshal body", err)

				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to unmarshal body: %v", err))

				return nil, libHTTP.CursorPagination{}, err
			}
		}

		transactionUUID := uuid.MustParse(tran.ID)

		t, exists := transactionsMap[transactionUUID]
		if !exists {
			t = tran.ToEntity()
			t.Operations = make([]*operation.Operation, 0)
			transactionsMap[transactionUUID] = t

			transactionOrder = append(transactionOrder, transactionUUID)
		}

		// Only append operation if it exists (opID not NULL)
		if opID != nil {
			op := operation.OperationPostgreSQLModel{
				ID:                    *opID,
				TransactionID:         *opTransactionID,
				Description:           *opDescription,
				Type:                  *opType,
				AssetCode:             *opAssetCode,
				Amount:                opAmount,
				AvailableBalance:      opAvailableBalance,
				OnHoldBalance:         opOnHoldBalance,
				AvailableBalanceAfter: opAvailableBalanceAfter,
				OnHoldBalanceAfter:    opOnHoldBalanceAfter,
				Status:                *opStatus,
				StatusDescription:     opStatusDescription,
				AccountID:             *opAccountID,
				AccountAlias:          *opAccountAlias,
				BalanceID:             *opBalanceID,
				ChartOfAccounts:       *opChartOfAccounts,
				OrganizationID:        *opOrganizationID,
				LedgerID:              *opLedgerID,
				CreatedAt:             *opCreatedAt,
				UpdatedAt:             *opUpdatedAt,
				DeletedAt:             opDeletedAt,
				Route:                 opRoute,
				BalanceAffected:       *opBalanceAffected,
				BalanceKey:            *opBalanceKey,
				VersionBalance:        opVersionBalance,
				VersionBalanceAfter:   opVersionBalanceAfter,
				Direction:             derefString(opDirection),
				RouteID:               opRouteID,
				RouteCode:             opRouteCode,
				RouteDescription:      opRouteDescription,
				Snapshot:              opSnapshot,
			}

			t.Operations = append(t.Operations, op.ToEntity())
		}
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	for _, transactionUUID := range transactionOrder {
		transactions = append(transactions, transactionsMap[transactionUUID])
	}

	hasPagination := len(transactions) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)

	transactions = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.Direction, transactions, filter.Limit)

	cur := libHTTP.CursorPagination{}
	if len(transactions) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.Direction, transactions[0].ID, transactions[len(transactions)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to calculate cursor: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return transactions, cur, nil
}

// CountByFilters returns the number of transactions matching the given filters.
func (r *TransactionPostgreSQLRepository) CountByFilters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter CountFilter) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_transactions_by_filters")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return 0, err
	}

	countQuery := squirrel.Select("COUNT(*)").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.GtOrEq{"created_at": filter.StartDate}).
		Where(squirrel.LtOrEq{"created_at": filter.EndDate}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if filter.Route != "" {
		countQuery = countQuery.Where(squirrel.Eq{"route": filter.Route})
	}

	if filter.Status != "" {
		countQuery = countQuery.Where(squirrel.Eq{"status": filter.Status})
	}

	query, args, err := countQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return 0, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count_transactions_by_filters.query")
	defer spanQuery.End()

	var count int64

	err = db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute count query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute count query: %v", err))

		return 0, err
	}

	return count, nil
}

// derefString safely dereferences a *string, returning "" if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
