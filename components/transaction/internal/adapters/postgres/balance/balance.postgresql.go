// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

const (
	// balanceParamsPerRow is the number of parameters per balance in the batch UPDATE VALUES clause.
	balanceParamsPerRow = 5
	// balanceGlobalParams is the fixed parameters (organization_id, ledger_id) appended after all rows.
	balanceGlobalParams = 2
	// maxBalanceBatchSize is the maximum balances per batch chunk.
	maxBalanceBatchSize     = (constant.MaxPGParams - balanceGlobalParams) / balanceParamsPerRow
	balanceUpdateMaxRetries = 4
	balanceUpdateRetryDelay = 5 * time.Millisecond

	pgErrCodeDeadlockDetected   = "40P01"
	pgErrCodeSerializationError = "40001"
	pgErrCodeLockNotAvailable   = "55P03"
)

var (
	errAllowSendingRequired        = errors.New("allow_sending value is required")
	errAllowReceivingRequired      = errors.New("allow_receiving value is required")
	errExpectedAliasKeyFormat      = errors.New("expected format: alias#key")
	errInvalidAliasKeyFormat       = errors.New("invalid alias#key format")
	errBatchUpdateRetriesExhausted = errors.New("batch balance update: exhausted retries")
)

var balanceColumnList = []string{
	"id",
	"organization_id",
	"ledger_id",
	"account_id",
	"alias",
	"asset_code",
	"available",
	"on_hold",
	"version",
	"account_type",
	"allow_sending",
	"allow_receiving",
	"created_at",
	"updated_at",
	"deleted_at",
	"key",
}

// Repository provides an interface for operations related to balance template entities.
// It defines methods for creating, finding, listing, updating, and deleting balance templates.
//
//go:generate mockgen --destination=balance.postgresql_mock.go --package=balance . Repository
type Repository interface {
	Create(ctx context.Context, balance *mmodel.Balance) error
	CreateIfNotExists(ctx context.Context, balance *mmodel.Balance) error
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Balance, error)
	FindByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (*mmodel.Balance, error)
	ExistsByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (bool, error)
	ListAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error)
	ListAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error)
	ListByAccountIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Balance, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Balance, error)
	ListByAliases(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error)
	ListByAliasesWithKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, aliasesWithKeys []string) ([]*mmodel.Balance, error)
	BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) (*mmodel.Balance, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	DeleteAllByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) error
	Sync(ctx context.Context, organizationID, ledgerID uuid.UUID, b mmodel.BalanceRedis) (bool, error)
	UpdateAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, balance mmodel.UpdateBalance) error
	ListByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.Balance, error)
	ListByAccountIDAtTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time) ([]*mmodel.Balance, error)
}

// BalancePostgreSQLRepository is a Postgresql-specific implementation of the BalanceRepository.
type BalancePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

type balanceUpdateTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// NewBalancePostgreSQLRepository returns a new instance of BalancePostgreSQLRepository using the given Postgres connection.
//
// NOTE: tableName is schema-qualified ("public.balance") so every squirrel
// builder emits FROM/INSERT/UPDATE against public.balance regardless of the
// session's search_path. This matters after migration 000023
// (staged_cutover/000023_atomic_swap_balance.up.sql) renames
// balance → balance_legacy and balance_partitioned → balance. See the
// detailed rationale on NewOperationPostgreSQLRepository — the same silent
// redirection risk applies. TestRepository_QueriesUseSchemaQualifiedTableNames
// enforces the invariant going forward.
func NewBalancePostgreSQLRepository(pc *libPostgres.PostgresConnection) (*BalancePostgreSQLRepository, error) {
	c := &BalancePostgreSQLRepository{
		connection: pc,
		tableName:  "public.balance",
	}

	if _, err := c.connection.GetDB(); err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	return c, nil
}

// Create inserts a new balance record into the database.
func (r *BalancePostgreSQLRepository) Create(ctx context.Context, balance *mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balances")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	record := &BalancePostgreSQLModel{}
	record.FromEntity(balance)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, `INSERT INTO public.balance VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *`,
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

		return fmt.Errorf("failed to insert balance: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create balance. Rows affected is 0", err)

		logger.Warnf("Failed to create balance. Rows affected is 0: %v", err)

		return err //nolint:wrapcheck
	}

	return nil
}

// CreateIfNotExists inserts a balance row using ON CONFLICT DO NOTHING.
// If a row with the same (organization_id, ledger_id, alias, key) already exists (where deleted_at IS NULL),
// the INSERT is silently skipped and no error is returned. This prevents duplicate-row bugs when
// concurrent pods race to materialize the same external pre-split balance.
func (r *BalancePostgreSQLRepository) CreateIfNotExists(ctx context.Context, balance *mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balance_if_not_exists")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	record := &BalancePostgreSQLModel{}
	record.FromEntity(balance)

	ctx, spanExec := tracer.Start(ctx, "postgres.create_if_not_exists.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, `INSERT INTO public.balance VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (organization_id, ledger_id, alias, key) WHERE deleted_at IS NULL DO NOTHING`,
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

		return fmt.Errorf("failed to execute query: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		logger.Infof("Balance already exists for alias=%s key=%s, skipped insert", balance.Alias, balance.Key)
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

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	query := squirrel.Select(balanceColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("account_id = ANY(?)", pq.Array(accountIds))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
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

			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed to perform database operation: %w", err)
	}

	return balances, nil
}

// ListByIDs retrieves balances by their balance IDs.
func (r *BalancePostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_balance_ids")
	defer span.End()

	if len(ids) == 0 {
		return []*mmodel.Balance{}, nil
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_balance_ids.query")

	query := squirrel.Select(balanceColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)
		logger.Errorf("Failed to build query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.Debugf("ListByIDs query: %s with args: %v", sqlQuery, args)

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)
		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
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

			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)
		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed to perform database operation: %w", err)
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to get database connection: %w", err)
	}

	balances := make([]*mmodel.Balance, 0)

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

	findAll := squirrel.Select(balanceColumnList...).
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to build query: %w", err)
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to execute query: %w", err)
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

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to perform database operation: %w", err)
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

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to calculate cursor: %w", err)
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to get database connection: %w", err)
	}

	balances := make([]*mmodel.Balance, 0)

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

	findAll := squirrel.Select(balanceColumnList...).
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to build query: %w", err)
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_account_id.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to execute query: %w", err)
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

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to perform database operation: %w", err)
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

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed to calculate cursor: %w", err)
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

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_aliases.query")

	query := squirrel.Select(balanceColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("alias = ANY(?)", pq.Array(aliases))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
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

			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed to perform database operation: %w", err)
	}

	return balances, nil
}

// ListExternalByOrganizationLedger returns all non-deleted external balances for
// a specific organization and ledger.
func (r *BalancePostgreSQLRepository) ListExternalByOrganizationLedger(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_external_balances")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := squirrel.Select(balanceColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Like{"alias": "@external/%"}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("alias ASC", "key ASC").
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	balances := make([]*mmodel.Balance, 0)

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

			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed to perform database operation: %w", err)
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

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	if len(aliasesWithKeys) == 0 {
		return []*mmodel.Balance{}, nil
	}

	orConditions := squirrel.Or{}

	for _, aliasWithKey := range aliasesWithKeys {
		parts := strings.Split(aliasWithKey, "#")
		if len(parts) != 2 { //nolint:mnd
			libOpentelemetry.HandleSpanError(&span, "Invalid alias#key format", errExpectedAliasKeyFormat)

			logger.Errorf("Invalid alias#key format: %s", aliasWithKey)

			return nil, errInvalidAliasKeyFormat
		}

		alias := parts[0]
		key := parts[1]

		orConditions = append(orConditions, squirrel.And{
			squirrel.Eq{"alias": alias},
			squirrel.Eq{"key": key},
		})
	}

	findQuery := squirrel.Select(balanceColumnList...).
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

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_aliases_with_keys.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
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

			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed to perform database operation: %w", err)
	}

	return balances, nil
}

// BalancesUpdate updates the balances in the database using a single batch UPDATE
// with a VALUES clause. This replaces the previous per-row UPDATE loop, reducing
// N round-trips to PostgreSQL to a single statement.
//
// At 50K TPS with 2 balances per transaction, this changes 100K individual UPDATEs/sec
// into 50K single-statement batch UPDATEs/sec — a 2-10x throughput improvement depending
// on transaction shape.
func (r *BalancePostgreSQLRepository) BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) (err error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "postgres.update_balances_batch")
	defer span.End()

	if len(balances) == 0 {
		return nil
	}

	normalizedBalances := normalizeBalancesForUpdate(balances, logger)
	if len(normalizedBalances) == 0 {
		return nil
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	for attempt := 1; attempt <= balanceUpdateMaxRetries; attempt++ {
		tx, txErr := db.BeginTx(ctx, nil)
		if txErr != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", txErr)

			return fmt.Errorf("failed to begin transaction: %w", txErr)
		}

		totalRowsAffected, execErr := r.executeBatchBalanceUpdateTx(ctx, tx, tracer, organizationID, ledgerID, normalizedBalances)
		if execErr != nil {
			rollbackTx(tx, logger, "err on rollback")

			if shouldRetry, waitErr := waitForBalanceUpdateRetry(ctx, execErr, attempt, logger, "transient PostgreSQL error"); shouldRetry {
				if waitErr != nil {
					return waitErr
				}

				continue
			}

			libOpentelemetry.HandleSpanError(&span, "Failed to execute batch balance update", execErr)
			logger.Errorf("Failed to execute batch balance update: %v", execErr)

			return execErr
		}

		if commitErr := tx.Commit(); commitErr != nil {
			rollbackTx(tx, logger, "err on rollback after commit failure")

			if shouldRetry, waitErr := waitForBalanceUpdateRetry(ctx, commitErr, attempt, logger, "transient commit error"); shouldRetry {
				if waitErr != nil {
					return waitErr
				}

				continue
			}

			libOpentelemetry.HandleSpanError(&span, "Failed to commit", commitErr)
			logger.Errorf("err on commit: %v", commitErr)

			return fmt.Errorf("failed to commit batch balance update: %w", commitErr)
		}

		logBatchBalanceUpdateResult(logger, totalRowsAffected, normalizedBalances)

		return nil
	}

	// Unreachable: the loop always returns from within (either success or error).
	// This explicit error makes intent clear and guards against future refactors.
	return fmt.Errorf("%w: %d attempts", errBatchUpdateRetriesExhausted, balanceUpdateMaxRetries)
}

// BalancesUpdateWithTx updates balances using an existing SQL transaction.
// This is used by consumer micro-batching to persist a whole flush in a single
// database transaction across balances, transactions, and operations.
func (r *BalancePostgreSQLRepository) BalancesUpdateWithTx(ctx context.Context, tx balanceUpdateTx, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "postgres.update_balances_batch.with_tx")
	defer span.End()

	if tx == nil || len(balances) == 0 {
		return nil
	}

	normalizedBalances := normalizeBalancesForUpdate(balances, logger)
	if len(normalizedBalances) == 0 {
		return nil
	}

	totalRowsAffected, err := r.executeBatchBalanceUpdateTx(ctx, tx, tracer, organizationID, ledgerID, normalizedBalances)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute batch balance update", err)
		logger.Errorf("Failed to execute batch balance update (with tx): %v", err)

		return fmt.Errorf("failed to execute query: %w", err)
	}

	logBatchBalanceUpdateResult(logger, totalRowsAffected, normalizedBalances)

	return nil
}

// rollbackTx performs a transaction rollback and logs any rollback error.
func rollbackTx(tx interface{ Rollback() error }, logger libLog.Logger, msg string) {
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		logger.Errorf("%s: %v", msg, rollbackErr)
	}
}

// waitForBalanceUpdateRetry checks whether the given error is retryable and the attempt
// budget allows retrying. When retryable, it waits with exponential back-off and returns
// (true, nil). If the context is cancelled during the wait, it returns (true, ctx.Err()).
// For non-retryable errors or exhausted attempts it returns (false, nil).
func waitForBalanceUpdateRetry(ctx context.Context, err error, attempt int, logger libLog.Logger, reason string) (shouldRetry bool, waitErr error) {
	if !isRetryableBatchBalanceUpdateError(err) || attempt >= balanceUpdateMaxRetries {
		return false, nil
	}

	retryDelay := time.Duration(attempt*attempt) * balanceUpdateRetryDelay
	logger.Warnf("Retrying batch balance update after %s (attempt %d/%d, delay=%s): %v", reason, attempt, balanceUpdateMaxRetries, retryDelay, err)

	select {
	case <-ctx.Done():
		return true, fmt.Errorf("balance update retry wait cancelled: %w", ctx.Err())
	case <-time.After(retryDelay):
	}

	return true, nil
}

// logBatchBalanceUpdateResult logs the outcome of a batch balance update, including a
// warning when fewer rows were affected than expected (stale versions).
func logBatchBalanceUpdateResult(logger libLog.Logger, totalRowsAffected int64, normalizedBalances []*mmodel.Balance) {
	if totalRowsAffected < int64(len(normalizedBalances)) {
		allIDs := collectAllBalanceIDs(normalizedBalances)

		// Note: we log ALL balance IDs since we can't know which specific ones were
		// skipped by the version guard without a RETURNING clause. The count tells
		// operators how many were stale; the IDs help with targeted investigation.
		logger.Warnf("Batch balance update: %d/%d rows affected (some versions were stale, candidate IDs: %v)", totalRowsAffected, len(normalizedBalances), allIDs)
	} else {
		logger.Infof("Batch balance update: %d/%d rows affected", totalRowsAffected, len(normalizedBalances))
	}
}

func (r *BalancePostgreSQLRepository) executeBatchBalanceUpdateTx(ctx context.Context, tx balanceUpdateTx, tracer trace.Tracer, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) (int64, error) {
	if err := lockBalancesForDeterministicUpdate(ctx, tx, organizationID, ledgerID, balances); err != nil {
		return 0, err
	}

	// Process balances in chunks to stay within PostgreSQL's 65535 parameter limit.
	// Each balance uses 5 params in the VALUES clause, plus 2 global params (org_id, ledger_id).
	// All chunks execute within the SAME transaction, so the entire batch is atomic.
	now := time.Now().UTC()

	var totalRowsAffected int64

	totalChunks := (len(balances) + maxBalanceBatchSize - 1) / maxBalanceBatchSize

	for chunkStart := 0; chunkStart < len(balances); chunkStart += maxBalanceBatchSize {
		chunkEnd := chunkStart + maxBalanceBatchSize
		if chunkEnd > len(balances) {
			chunkEnd = len(balances)
		}

		chunk := balances[chunkStart:chunkEnd]
		chunkIdx := chunkStart / maxBalanceBatchSize

		ctxChunk, spanChunk := tracer.Start(ctx, "postgres.batch_update_chunk")
		spanChunk.SetAttributes(
			attribute.Int("chunk.index", chunkIdx),
			attribute.Int("chunk.size", len(chunk)),
			attribute.Int("chunk.total", totalChunks),
		)

		query, args := buildBatchBalanceUpdateQuery(chunk, now, organizationID, ledgerID)
		if query == "" {
			spanChunk.End()

			continue
		}

		result, err := tx.ExecContext(ctxChunk, query, args...)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanChunk, "Failed to execute batch balance update chunk", err)
			spanChunk.End()

			return 0, fmt.Errorf("batch balance update chunk %d-%d: %w", chunkStart, chunkEnd, err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanChunk, "Failed to get rows affected for chunk", err)
			spanChunk.End()

			return 0, fmt.Errorf("batch balance update rows affected chunk %d-%d: %w", chunkStart, chunkEnd, err)
		}

		spanChunk.End()

		totalRowsAffected += rowsAffected
	}

	return totalRowsAffected, nil
}

func lockBalancesForDeterministicUpdate(ctx context.Context, tx balanceUpdateTx, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	ids := collectUniqueSortedBalanceIDs(balances)
	if len(ids) == 0 {
		return nil
	}

	rows, err := tx.QueryContext(
		ctx,
		`SELECT id
		 FROM public.balance
		 WHERE organization_id = $1
		   AND ledger_id = $2
		   AND id = ANY($3::uuid[])
		   AND deleted_at IS NULL
		 ORDER BY id
		 FOR UPDATE`,
		organizationID,
		ledgerID,
		pq.Array(ids),
	)
	if err != nil {
		return fmt.Errorf("failed to perform database operation: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate lock rows: %w", err)
	}

	return nil
}

func collectUniqueSortedBalanceIDs(balances []*mmodel.Balance) []string {
	idsByValue := make(map[string]struct{}, len(balances))

	for _, balance := range balances {
		if balance == nil || balance.ID == "" {
			continue
		}

		idsByValue[balance.ID] = struct{}{}
	}

	ids := make([]string, 0, len(idsByValue))
	for id := range idsByValue {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}

func normalizeBalancesForUpdate(balances []*mmodel.Balance, logger libLog.Logger) []*mmodel.Balance {
	normalized := make([]*mmodel.Balance, 0, len(balances))

	for i, balance := range balances {
		if balance == nil {
			if logger != nil {
				logger.Warnf("normalizeBalancesForUpdate: dropping nil balance at index %d — Redis may have values that PG will not receive", i)
			}

			continue
		}

		if balance.ID == "" {
			if logger != nil {
				logger.Warnf("normalizeBalancesForUpdate: dropping balance with empty ID at index %d (alias=%s, asset=%s) — Redis may have values that PG will not receive", i, balance.Alias, balance.AssetCode)
			}

			continue
		}

		normalized = append(normalized, balance)
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].ID < normalized[j].ID
	})

	return normalized
}

func isRetryableBatchBalanceUpdateError(err error) bool {
	var pgxErr *pgconn.PgError
	if errors.As(err, &pgxErr) {
		switch pgxErr.Code {
		case pgErrCodeDeadlockDetected, pgErrCodeSerializationError, pgErrCodeLockNotAvailable:
			return true
		}
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch string(pqErr.Code) {
		case pgErrCodeDeadlockDetected, pgErrCodeSerializationError, pgErrCodeLockNotAvailable:
			return true
		}
	}

	return false
}

func buildBatchBalanceUpdateQuery(chunk []*mmodel.Balance, now time.Time, organizationID, ledgerID uuid.UUID) (string, []any) {
	args := make([]any, 0, len(chunk)*balanceParamsPerRow+balanceGlobalParams)
	valuesRows := make([]string, 0, len(chunk))
	paramIdx := 1

	for _, balance := range chunk {
		if balance == nil {
			continue
		}

		valuesRows = append(valuesRows,
			"($"+strconv.Itoa(paramIdx)+"::UUID, $"+strconv.Itoa(paramIdx+1)+"::DECIMAL, $"+strconv.Itoa(paramIdx+2)+"::DECIMAL, $"+strconv.Itoa(paramIdx+3)+"::BIGINT, $"+strconv.Itoa(paramIdx+4)+"::TIMESTAMPTZ)", //nolint:mnd
		)
		args = append(args, balance.ID, balance.Available, balance.OnHold, balance.Version, now)
		paramIdx += 5
	}

	if len(valuesRows) == 0 {
		return "", nil
	}

	orgParam := strconv.Itoa(paramIdx)
	ledgerParam := strconv.Itoa(paramIdx + 1)

	args = append(args, organizationID, ledgerID)

	query := `UPDATE public.balance AS b
	SET available = v.available,
	    on_hold = v.on_hold,
	    version = v.version,
	    updated_at = v.updated_at
	FROM (VALUES ` + strings.Join(valuesRows, ", ") + `) AS v(id, available, on_hold, version, updated_at)
	WHERE b.organization_id = $` + orgParam + `
	  AND b.ledger_id = $` + ledgerParam + `
	  AND b.id = v.id
	  AND b.version < v.version
	  AND b.deleted_at IS NULL`

	return query, args
}

func collectAllBalanceIDs(balances []*mmodel.Balance) []string {
	allIDs := make([]string, 0, len(balances))

	for _, b := range balances {
		if b == nil {
			continue
		}

		allIDs = append(allIDs, b.ID)
	}

	return allIDs
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

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	balance := &BalancePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	query := squirrel.Select(balanceColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	row := db.QueryRowContext(ctx, sqlQuery, args...)

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

			return nil, err //nolint:wrapcheck
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed to scan row: %w", err)
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

		return nil, fmt.Errorf("failed to get database connection: %w", err)
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
			FROM public.balance
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

			return nil, err //nolint:wrapcheck
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed to scan row: %w", err)
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

		return false, fmt.Errorf("failed to get database connection: %w", err)
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

		return false, fmt.Errorf("failed to build query: %w", err)
	}

	primaryDBs := db.PrimaryDBs()

	var row *sql.Row

	if len(primaryDBs) > 0 && primaryDBs[0] != nil {
		// Force primary read to guarantee read-after-write consistency
		// for balance existence checks used in synchronous creation flows.
		row = primaryDBs[0].QueryRowContext(ctx, query, args...)
	} else {
		row = db.QueryRowContext(ctx, query, args...)
	}

	spanQuery.End()

	var exists bool
	if err := row.Scan(&exists); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return false, fmt.Errorf("failed to scan row: %w", err)
	}

	return exists, nil
}

// Delete marks a balance as deleted in the database using the ID provided.
func (r *BalancePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_balance")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `
		UPDATE public.balance
		SET deleted_at = NOW()
		WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to execute delete query", err)

		logger.Errorf("failed to execute delete query: %v", err)

		return fmt.Errorf("failed to execute query: %w", err)
	}

	spanQuery.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance. Rows affected is 0", err)

		logger.Warnf("Failed to delete balance. Rows affected is 0: %v", err)

		return err //nolint:wrapcheck
	}

	return nil
}

// DeleteAllByIDs marks all provided balances as deleted in the database using the IDs provided.
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

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to begin transaction for bulk delete", err)

		logger.Errorf("failed to begin transaction for bulk delete: %v", err)

		return fmt.Errorf("failed to begin transaction: %w", err)
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
		UPDATE public.balance
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

		return fmt.Errorf("failed to execute query: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected on bulk delete", err)

		logger.Errorf("Failed to get rows affected on bulk delete: %v", err)

		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected != int64(len(ids)) {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balances. Rows affected mismatch", err)

		logger.Warnf("Failed to delete balances. Rows affected mismatch: %v", err)

		return err //nolint:wrapcheck
	}

	if err = tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to commit transaction for bulk delete", err)

		logger.Errorf("failed to commit transaction for bulk delete: %v", err)

		return fmt.Errorf("failed to commit transaction for bulk delete: %w", err)
	}

	committed = true

	return nil
}

// Update updates the allow_sending and allow_receiving fields of a Balance in the database.
// Returns the updated balance to avoid a second query and potential replication lag issues.
func (r *BalancePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_balance")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.update.query")
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
	args = append(args, time.Now().UTC(), organizationID, ledgerID, id)

	queryUpdate := `UPDATE public.balance SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) + //nolint:mnd
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL` +
		` RETURNING id, organization_id, ledger_id, account_id, alias, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at, deleted_at, key`

	record := &BalancePostgreSQLModel{}

	row := db.QueryRowContext(ctx, queryUpdate, args...)
	if err = row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.AccountID,
		&record.Alias,
		&record.AssetCode,
		&record.Available,
		&record.OnHold,
		&record.Version,
		&record.AccountType,
		&record.AllowSending,
		&record.AllowReceiving,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
		&record.Key,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			bizErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance not found", bizErr)

			logger.Warnf("Balance not found: %v", bizErr)

			return nil, fmt.Errorf("balance not found: %w", bizErr)
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to update balance", err)

		logger.Errorf("Failed to update balance: %v", err)

		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	return record.ToEntity(), nil
}

// Sync updates a balance record from a Redis balance snapshot, applying version-guarded UPDATE.
func (r *BalancePostgreSQLRepository) Sync(ctx context.Context, organizationID, ledgerID uuid.UUID, b mmodel.BalanceRedis) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.sync_balance")
	defer span.End()

	id, err := uuid.Parse(b.ID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "invalid balance ID", err)

		logger.Errorf("invalid balance ID: %v", err)

		return false, fmt.Errorf("failed to parse balance id: %w", err)
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return false, fmt.Errorf("failed to get database connection: %w", err)
	}

	res, err := db.ExecContext(ctx, `
		UPDATE public.balance
		SET available = $1, on_hold = $2, version = $3, updated_at = $4
		WHERE organization_id = $5 AND ledger_id = $6 AND id = $7 AND deleted_at IS NULL AND version < $3
	`, b.Available, b.OnHold, b.Version, time.Now().UTC(), organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update balance from redis", err)

		logger.Errorf("Failed to update balance from redis: %v", err)

		return false, fmt.Errorf("failed to execute query: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to read rows affected", err)

		logger.Errorf("Failed to read rows affected: %v", err)

		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return affected > 0, nil
}

// UpdateAllByAccountID updates allow_sending and allow_receiving for all balances belonging to an account.
func (r *BalancePostgreSQLRepository) UpdateAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, balance mmodel.UpdateBalance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_all_by_account_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed to get database connection: %w", err)
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update_all_by_account_id.exec")
	defer spanExec.End()

	if balance.AllowSending == nil {
		err := errAllowSendingRequired

		libOpentelemetry.HandleSpanError(&spanExec, "allow_sending value is required", err)

		logger.Errorf("allow_sending value is required: %v", err)

		return fmt.Errorf("failed to perform database operation: %w", err)
	}

	if balance.AllowReceiving == nil {
		err := errAllowReceivingRequired

		libOpentelemetry.HandleSpanError(&spanExec, "allow_receiving value is required", err)

		logger.Errorf("allow_receiving value is required: %v", err)

		return fmt.Errorf("failed to perform database operation: %w", err)
	}

	query := `UPDATE public.balance SET allow_sending = $1, allow_receiving = $2, updated_at = NOW() WHERE organization_id = $3 AND ledger_id = $4 AND account_id = $5 AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, *balance.AllowSending, *balance.AllowReceiving, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return fmt.Errorf("failed to execute query: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to update balances. Rows affected is 0", err)

		logger.Warnf("Failed to update all balances by account id. Rows affected is 0: %v", err)

		return err //nolint:wrapcheck
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

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_account_id.query")

	query := squirrel.Select(balanceColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"account_id": accountID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)
		logger.Errorf("Failed to build query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)
		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
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

			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)
		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed to perform database operation: %w", err)
	}

	return balances, nil
}

// ListByAccountIDAtTimestamp retrieves all balances for an account at a specific point in time.
// It uses a single optimized query with LEFT JOIN to fetch balance states, avoiding multiple round-trips.
// Balances without operations at the timestamp are returned with zero values (initial state).
func (r *BalancePostgreSQLRepository) ListByAccountIDAtTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_account_id_at_timestamp")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	balances := make([]*mmodel.Balance, 0)

	// Build CTE subquery for latest operations per balance using DISTINCT ON
	// This gets the last operation for each balance before the timestamp
	// NOTE: Do NOT use PlaceholderFormat here - let the main query convert all ? to $1, $2, etc.
	latestOpsSubquery := squirrel.Select(
		"DISTINCT ON (balance_id) balance_id",
		"available_balance_after",
		"on_hold_balance_after",
		"balance_version_after",
		"created_at as op_created_at",
	).
		From("public.operation").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"account_id": accountID}).
		Where(squirrel.LtOrEq{"created_at": timestamp}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("balance_id", "created_at DESC")

	latestOpsSql, latestOpsArgs, err := latestOpsSubquery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build CTE subquery", err)
		logger.Errorf("Failed to build CTE subquery: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	// Build main query with LEFT JOIN using CTE
	// COALESCE handles balances without operations (returns 0 for initial state)
	mainQuery := squirrel.Select(
		"b.id",
		"b.organization_id",
		"b.ledger_id",
		"b.account_id",
		"b.alias",
		"b.key",
		"b.asset_code",
		"b.account_type",
		"b.created_at",
		"COALESCE(o.available_balance_after, 0) as available",
		"COALESCE(o.on_hold_balance_after, 0) as on_hold",
		"COALESCE(o.balance_version_after, 0) as version",
		"COALESCE(o.op_created_at, b.created_at) as updated_at",
	).
		Prefix("WITH latest_ops AS ("+latestOpsSql+")", latestOpsArgs...).
		From("public.balance b").
		LeftJoin("latest_ops o ON b.id = o.balance_id").
		Where(squirrel.Eq{"b.organization_id": organizationID}).
		Where(squirrel.Eq{"b.ledger_id": ledgerID}).
		Where(squirrel.Eq{"b.account_id": accountID}).
		Where(squirrel.Eq{"b.deleted_at": nil}).
		Where(squirrel.LtOrEq{"b.created_at": timestamp}).
		OrderBy("b.id ASC").
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := mainQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build main query", err)
		logger.Errorf("Failed to build main query: %v", err)

		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	logger.Debugf("ListByAccountIDAtTimestamp query: %s with args: %v", sqlQuery, args)

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_balances_by_account_id_at_timestamp.query")

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)
		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalanceAtTimestampModel
		if err := rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.Key,
			&balance.AssetCode,
			&balance.AccountType,
			&balance.CreatedAt,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.UpdatedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)
			logger.Errorf("Failed to scan row: %v", err)

			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)
		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed to perform database operation: %w", err)
	}

	return balances, nil
}
