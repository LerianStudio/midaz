// Package balance provides PostgreSQL repository implementation for account balances.
//
// This file contains the Repository interface and BalancePostgreSQLRepository implementation,
// providing comprehensive data access operations for balance management in the ledger system.
//
// Key Features:
//   - Multi-tenant isolation via organization_id and ledger_id filtering
//   - Cursor-based pagination for efficient large dataset traversal
//   - Optimistic locking via version field for concurrent update safety
//   - Soft delete pattern for audit trail and recovery
//   - Bulk operations for performance (batch updates, batch deletes)
//   - Multiple lookup strategies (by ID, alias, account, key)
//
// Query Patterns:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                    Balance Queries                           │
//	├─────────────────────────────────────────────────────────────┤
//	│  Find(id)               → Single balance by ID               │
//	│  FindByAccountIDAndKey  → Balance by account + key           │
//	│  ListByAccountIDs       → Balances for multiple accounts     │
//	│  ListByAliases          → Balances matching aliases          │
//	│  ListByAliasesWithKeys  → Balances by alias#key pairs        │
//	│  ListAll                → Paginated list for ledger          │
//	│  ListAllByAccountID     → Paginated list for account         │
//	└─────────────────────────────────────────────────────────────┘
//
// Concurrency Model:
// Balance updates use optimistic locking. Each balance has a version counter that
// increments on every update. Update queries include "WHERE version < new_version"
// to prevent lost updates when multiple processes modify the same balance.
//
// Redis Sync:
// The Sync method supports eventual consistency between Redis (hot cache) and
// PostgreSQL (source of truth). Redis serves high-frequency reads while PostgreSQL
// maintains durability. Version comparison ensures only newer data is persisted.
//
// Related Packages:
//   - balance.go: Contains BalancePostgreSQLModel and entity conversion
//   - mmodel: Domain model definitions
//   - transaction: Uses this repository for balance mutations
package balance

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Repository defines the interface for balance persistence operations.
//
// This interface follows the repository pattern from Domain-Driven Design, providing
// a collection-like abstraction over the balance data store. All methods enforce
// multi-tenant isolation through organizationID and ledgerID parameters.
//
// Thread Safety:
// Implementations must be safe for concurrent use. The PostgreSQL implementation
// uses connection pooling and optimistic locking to handle concurrent access.
//
// Error Handling:
// Methods return domain errors from pkg/constant for business rule violations
// (e.g., ErrEntityNotFound) and infrastructure errors for system failures.
//
// Mock Generation:
//
//go:generate mockgen --destination=balance.postgresql_mock.go --package=balance . Repository
type Repository interface {
	// Create inserts a new balance into the database.
	// Returns error if the balance already exists or on database failure.
	Create(ctx context.Context, balance *mmodel.Balance) error

	// Find retrieves a single balance by its unique ID.
	// Returns ErrEntityNotFound if the balance doesn't exist or is soft-deleted.
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Balance, error)

	// FindByAccountIDAndKey retrieves a balance by account and key combination.
	// This is the primary lookup for transaction processing.
	FindByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (*mmodel.Balance, error)

	// ExistsByAccountIDAndKey checks if a balance exists without loading it.
	// More efficient than FindByAccountIDAndKey when only existence matters.
	ExistsByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (bool, error)

	// ListAll returns all balances in a ledger with cursor-based pagination.
	// Results are ordered by created_at and filtered by date range.
	ListAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error)

	// ListAllByAccountID returns all balances for a specific account with pagination.
	// Use this to display all balance keys for an account (default, savings, etc.).
	ListAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error)

	// ListByAccountIDs returns balances for multiple accounts in a single query.
	// Used for batch operations like transaction processing involving multiple accounts.
	ListByAccountIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Balance, error)

	// ListByAliases returns balances matching the provided account aliases.
	// Used when Gold DSL references accounts by alias rather than ID.
	ListByAliases(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error)

	// ListByAliasesWithKeys returns balances matching alias#key pairs.
	// Format: ["account1#default", "account2#savings"]
	// Used for precise balance targeting in multi-key scenarios.
	ListByAliasesWithKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, aliasesWithKeys []string) ([]*mmodel.Balance, error)

	// BalancesUpdate performs batch update of balance amounts with optimistic locking.
	// Uses version comparison to prevent lost updates in concurrent scenarios.
	// Skips balances where version in DB >= version in request (already updated).
	BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error

	// Update modifies allow_sending and allow_receiving flags for a balance.
	// Used for enabling/disabling debits and credits on specific balances.
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) error

	// Delete performs soft delete on a balance by setting deleted_at.
	// The balance remains in the database for audit but is excluded from queries.
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error

	// DeleteAllByIDs performs bulk soft delete within a transaction.
	// Fails atomically if any balance cannot be deleted.
	DeleteAllByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) error

	// Sync persists a balance from Redis cache to PostgreSQL.
	// Only updates if the Redis version is newer than the database version.
	// Returns true if the update was applied, false if skipped due to version.
	Sync(ctx context.Context, organizationID, ledgerID uuid.UUID, b mmodel.BalanceRedis) (bool, error)

	// UpdateAllByAccountID updates all balances for an account.
	// Used when account-level changes affect all balance keys.
	UpdateAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, balance mmodel.UpdateBalance) error

	// ListByAccountID returns all balances for an account without pagination.
	// Simpler alternative to ListAllByAccountID when pagination isn't needed.
	ListByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.Balance, error)
}

// BalancePostgreSQLRepository is the PostgreSQL implementation of the Repository interface.
//
// This repository provides all balance persistence operations using PostgreSQL as the
// backing store. It uses squirrel for query building and lib-commons for connection
// management, tracing, and observability.
//
// Connection Management:
// The repository holds a reference to a PostgresConnection which manages the connection
// pool. Database connections are obtained per-operation via GetDB() to support
// connection pooling and health checks.
//
// Observability:
// All methods create OpenTelemetry spans for distributed tracing. Span names follow
// the pattern "postgres.{operation}" (e.g., "postgres.find_balance").
//
// Thread Safety:
// The repository is safe for concurrent use. Each operation obtains its own database
// connection from the pool and uses separate prepared statements.
type BalancePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection // Connection pool manager
	tableName  string                          // Table name: "balance"
}

// NewBalancePostgreSQLRepository creates a new balance repository with the given connection.
//
// This constructor validates the database connection by calling GetDB(). If the
// connection cannot be established, it panics to prevent the application from
// starting with a broken database connection.
//
// Panic Behavior:
// This is one of the few places where panic is acceptable because:
//  1. It runs during application startup (not request handling)
//  2. A failed database connection is unrecoverable
//  3. Failing fast prevents silent failures
//
// Parameters:
//   - pc: PostgreSQL connection pool manager from lib-commons
//
// Returns:
//   - *BalancePostgreSQLRepository: Ready-to-use repository instance
//
// Panics:
//   - If database connection cannot be established
func NewBalancePostgreSQLRepository(pc *libPostgres.PostgresConnection) *BalancePostgreSQLRepository {
	c := &BalancePostgreSQLRepository{
		connection: pc,
		tableName:  "balance",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

func (r *BalancePostgreSQLRepository) Create(ctx context.Context, balance *mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balances")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	record := &BalancePostgreSQLModel{}
	record.FromEntity(balance)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	result, err := db.ExecContext(ctx, `INSERT INTO balance VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *`,
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
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create balance. Rows affected is 0", err)

		logger.Warnf("Failed to create balance. Rows affected is 0: %v", err)

		return err
	}

	return nil
}

// ListByAccountIDs list Balances entity from the database using the provided accountIDs.
func (r *BalancePostgreSQLRepository) ListByAccountIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIds []uuid.UUID) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	rows, err := db.QueryContext(
		ctx,
		"SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND account_id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID,
		ledgerID,
		pq.Array(accountIds),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err := rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
			&balance.Key,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return balances, nil
}

// ListAll list Balances entity from the database.
func (r *BalancePostgreSQLRepository) ListAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_balances")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	balances := make([]*mmodel.Balance, 0)

	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err = rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
			&balance.Key,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(balances) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	balances = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, balances, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(balances) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, balances[0].ID, balances[len(balances)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return balances, cur, nil
}

// ListAllByAccountID list Balances entity from the database using the provided accountID.
func (r *BalancePostgreSQLRepository) ListAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_balances_by_account_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	balances := make([]*mmodel.Balance, 0)

	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("account_id = ?", accountID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_account_id.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err = rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
			&balance.Key,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(balances) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	balances = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, balances, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(balances) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, balances[0].ID, balances[len(balances)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return balances, cur, nil
}

// ListByAliases list Balances entity from the database using the provided aliases.
func (r *BalancePostgreSQLRepository) ListByAliases(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_aliases")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_aliases.query")

	rows, err := db.QueryContext(
		ctx,
		"SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND alias = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID,
		ledgerID,
		pq.Array(aliases),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err := rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
			&balance.Key,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return balances, nil
}

// ListByAliasesWithKeys list Balances entity from the database using the provided alias#key pairs.
func (r *BalancePostgreSQLRepository) ListByAliasesWithKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, aliasesWithKeys []string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_aliases_with_keys")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	if len(aliasesWithKeys) == 0 {
		return []*mmodel.Balance{}, nil
	}

	orConditions := squirrel.Or{}

	for _, aliasWithKey := range aliasesWithKeys {
		parts := strings.Split(aliasWithKey, "#")
		if len(parts) != 2 {
			libOpentelemetry.HandleSpanError(&span, "Invalid alias#key format", errors.New("expected format: alias#key"))

			logger.Errorf("Invalid alias#key format: %s", aliasWithKey)

			return nil, errors.New("invalid alias#key format")
		}

		alias := parts[0]
		key := parts[1]

		orConditions = append(orConditions, squirrel.And{
			squirrel.Eq{"alias": alias},
			squirrel.Eq{"key": key},
		})
	}

	findQuery := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(orConditions).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_aliases_with_keys.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err := rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
			&balance.Key,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return balances, nil
}

// BalancesUpdate updates the balances in the database.
func (r *BalancePostgreSQLRepository) BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "postgres.update_balances")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to init balances", err)

		return err
	}

	defer func() {
		if err != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to init balances", rollbackErr)

				logger.Errorf("err on rollback: %v", rollbackErr)
			}
		} else {
			commitErr := tx.Commit()
			if commitErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to init balances", commitErr)

				logger.Errorf("err on commit: %v", commitErr)
			}
		}
	}()

	for _, balance := range balances {
		ctxBalance, spanUpdate := tracer.Start(ctx, "postgres.update_balance")

		var updates []string

		var args []any

		updates = append(updates, "available = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.Available)

		updates = append(updates, "on_hold = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.OnHold)

		updates = append(updates, "version = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.Version)

		updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
		args = append(args, time.Now(), organizationID, ledgerID, balance.ID, balance.Version)

		queryUpdate := `UPDATE balance SET ` + strings.Join(updates, ", ") +
			` WHERE organization_id = $` + strconv.Itoa(len(args)-3) +
			` AND ledger_id = $` + strconv.Itoa(len(args)-2) +
			` AND id = $` + strconv.Itoa(len(args)-1) +
			` AND version < $` + strconv.Itoa(len(args)) +
			` AND deleted_at IS NULL`

		result, err := tx.ExecContext(ctxBalance, queryUpdate, args...)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdate, "Err on result exec content", err)

			logger.Errorf("Err on result exec content: %v", err)

			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdate, "Err ", err)

			logger.Errorf("Err: %v", err)

			return err
		}

		if rowsAffected == 0 {
			logger.Infof("Zero rows affected")

			continue
		}

		spanUpdate.End()
	}

	return nil
}

// Find retrieves a balance entity from the database using the provided ID.
func (r *BalancePostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_balance")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	balance := &BalancePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)

	spanQuery.End()

	if err = row.Scan(
		&balance.ID,
		&balance.OrganizationID,
		&balance.LedgerID,
		&balance.AccountID,
		&balance.Alias,
		&balance.AssetCode,
		&balance.Available,
		&balance.OnHold,
		&balance.Version,
		&balance.AccountType,
		&balance.AllowSending,
		&balance.AllowReceiving,
		&balance.CreatedAt,
		&balance.UpdatedAt,
		&balance.DeletedAt,
		&balance.Key,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			logger.Warnf("Failed to scan row: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
	}

	return balance.ToEntity(), nil
}

// FindByAccountIDAndKey retrieves a balance record based on accountID and key within the specified organization and ledger.
func (r *BalancePostgreSQLRepository) FindByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_balance_by_account_id_and_key")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	balance := &BalancePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	query := `SELECT 
			   id,
			   organization_id,
			   ledger_id,
			   account_id,
			   alias,
			   key,
			   asset_code,
			   available,
			   on_hold,
			   version,
			   account_type,
			   allow_sending,
			   allow_receiving,
			   created_at,
			   updated_at,
			   deleted_at
			FROM balance 
			WHERE organization_id = $1 
			   AND ledger_id = $2 
			   AND account_id = $3 
			   AND key = $4 
			   AND deleted_at IS NULL`

	row := db.QueryRowContext(ctx, query, organizationID, ledgerID, accountID, key)

	spanQuery.End()

	if err = row.Scan(
		&balance.ID,
		&balance.OrganizationID,
		&balance.LedgerID,
		&balance.AccountID,
		&balance.Alias,
		&balance.Key,
		&balance.AssetCode,
		&balance.Available,
		&balance.OnHold,
		&balance.Version,
		&balance.AccountType,
		&balance.AllowSending,
		&balance.AllowReceiving,
		&balance.CreatedAt,
		&balance.UpdatedAt,
		&balance.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			logger.Warnf("Failed to scan row: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
	}

	return balance.ToEntity(), nil
}

// ExistsByAccountIDAndKey returns true if a balance exists for the given accountID and key within the specified organization and ledger.
func (r *BalancePostgreSQLRepository) ExistsByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.exists_balance_by_account_id_and_key")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.exists.query")

	existsQuery := squirrel.Select("1").
		Prefix("SELECT EXISTS (").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("account_id = ?", accountID)).
		Where(squirrel.Expr("key = ?", key)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Suffix(")").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := existsQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return false, err
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	var exists bool
	if err := row.Scan(&exists); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return false, err
	}

	return exists, nil
}

// Delete marks a balance as deleted in the database using the ID provided
func (r *BalancePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_balance")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `
		UPDATE balance 
		SET deleted_at = NOW()
		WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to execute delete query", err)

		logger.Errorf("failed to execute delete query: %v", err)

		return err
	}

	spanQuery.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return err
	}

	if rowsAffected == 0 {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance. Rows affected is 0", err)

		logger.Warnf("Failed to delete balance. Rows affected is 0: %v", err)

		return err
	}

	return nil
}

// DeleteAllByIDs marks all provided balances as deleted in the database using the IDs provided
func (r *BalancePostgreSQLRepository) DeleteAllByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_balances")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to begin transaction for bulk delete", err)

		logger.Errorf("failed to begin transaction for bulk delete: %v", err)

		return err
	}

	committed := false

	defer func() {
		if committed {
			return
		}

		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			libOpentelemetry.HandleSpanError(&span, "failed to rollback transaction for bulk delete", rollbackErr)

			logger.Errorf("failed to rollback transaction for bulk delete: %v", rollbackErr)
		}
	}()

	ctxExec, spanExec := tracer.Start(ctx, "postgres.delete_balances.exec")
	defer spanExec.End()

	result, err := tx.ExecContext(ctxExec, `
		UPDATE balance
		SET deleted_at = NOW()
		WHERE organization_id = $1
		  AND ledger_id = $2
		  AND id = ANY($3)
		  AND deleted_at IS NULL`,
		organizationID, ledgerID, pq.Array(ids),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "failed to execute bulk delete query", err)

		logger.Errorf("failed to execute bulk delete query: %v", err)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected on bulk delete", err)

		logger.Errorf("Failed to get rows affected on bulk delete: %v", err)

		return err
	}

	if rowsAffected != int64(len(ids)) {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balances. Rows affected mismatch", err)

		logger.Warnf("Failed to delete balances. Rows affected mismatch: %v", err)

		return err
	}

	if err = tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to commit transaction for bulk delete", err)

		logger.Errorf("failed to commit transaction for bulk delete: %v", err)

		return err
	}

	committed = true

	return nil
}

// Update updates the allow_sending and allow_receiving fields of a Balance in the database.
func (r *BalancePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_balance")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.update.exec")
	defer spanQuery.End()

	var updates []string

	var args []any

	if balance.AllowSending != nil {
		updates = append(updates, "allow_sending = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.AllowSending)
	}

	if balance.AllowReceiving != nil {
		updates = append(updates, "allow_receiving = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.AllowReceiving)
	}

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
	args = append(args, time.Now(), organizationID, ledgerID, id)

	queryUpdate := `UPDATE balance SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, queryUpdate, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Err on result exec content", err)

		logger.Errorf("Err on result exec content: %v", err)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		if err == nil {
			err = sql.ErrNoRows
		}

		libOpentelemetry.HandleSpanError(&span, "Err on rows affected", err)

		logger.Errorf("Failed to update balance. Rows affected is 0: %v", err)

		return err
	}

	return nil
}

func (r *BalancePostgreSQLRepository) Sync(ctx context.Context, organizationID, ledgerID uuid.UUID, b mmodel.BalanceRedis) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.sync_balance")
	defer span.End()

	id, err := uuid.Parse(b.ID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "invalid balance ID", err)

		logger.Errorf("invalid balance ID: %v", err)

		return false, err
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return false, err
	}

	res, err := db.ExecContext(ctx, `
		UPDATE balance
		SET available = $1, on_hold = $2, version = $3, updated_at = $4
		WHERE organization_id = $5 AND ledger_id = $6 AND id = $7 AND deleted_at IS NULL AND version < $3
	`, b.Available, b.OnHold, b.Version, time.Now(), organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update balance from redis", err)

		logger.Errorf("Failed to update balance from redis: %v", err)

		return false, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to read rows affected", err)

		logger.Errorf("Failed to read rows affected: %v", err)

		return false, err
	}

	return affected > 0, nil
}

func (r *BalancePostgreSQLRepository) UpdateAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, balance mmodel.UpdateBalance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_all_by_account_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update_all_by_account_id.exec")
	defer spanExec.End()

	if balance.AllowSending == nil {
		err := errors.New("allow_sending value is required")

		libOpentelemetry.HandleSpanError(&spanExec, "allow_sending value is required", err)

		logger.Errorf("allow_sending value is required: %v", err)

		return err
	}

	if balance.AllowReceiving == nil {
		err := errors.New("allow_receiving value is required")

		libOpentelemetry.HandleSpanError(&spanExec, "allow_receiving value is required", err)

		logger.Errorf("allow_receiving value is required: %v", err)

		return err
	}

	query := `UPDATE balance SET allow_sending = $1, allow_receiving = $2, updated_at = NOW() WHERE organization_id = $3 AND ledger_id = $4 AND account_id = $5 AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, *balance.AllowSending, *balance.AllowReceiving, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to update balances. Rows affected is 0", err)

		logger.Warnf("Failed to update all balances by account id. Rows affected is 0: %v", err)

		return err
	}

	return nil
}

// ListByAccountID list Balances entity from the database using the provided accountID.
// This method does not support pagination or date filtering.
func (r *BalancePostgreSQLRepository) ListByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_account_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_account_id.query")

	rows, err := db.QueryContext(
		ctx,
		"SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND account_id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID,
		ledgerID,
		accountID,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)
		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err := rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
			&balance.Key,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)
			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)
		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return balances, nil
}
