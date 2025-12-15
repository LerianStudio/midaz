package balance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
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

const (
	aliasKeyParts            = 2
	whereOrgIDOffset         = 3
	whereLedgerIDOffset      = 2
	whereOrgIDOnlyOffset     = 2
	placeholderStartIndex    = 2
	balanceUpdateFieldCount  = 4
	balanceUpdateArgCapacity = 7
)

var (
	// ErrInvalidAliasKeyFormat is returned when alias#key format is invalid
	ErrInvalidAliasKeyFormat = errors.New("invalid alias#key format")
	// ErrAllowSendingRequired is returned when allow_sending value is required
	ErrAllowSendingRequired = errors.New("allow_sending value is required")
	// ErrAllowReceivingRequired is returned when allow_receiving value is required
	ErrAllowReceivingRequired = errors.New("allow_receiving value is required")
	// ErrNoBalancesUpdated is returned when no balances were successfully updated
	ErrNoBalancesUpdated = errors.New("no balances updated: all stale or missing rows")
)

// Repository provides an interface for operations related to balance template entities.
// It defines methods for creating, finding, listing, updating, and deleting balance templates.
//
//go:generate mockgen --destination=balance.postgresql_mock.go --package=balance . Repository
type Repository interface {
	Create(ctx context.Context, balance *mmodel.Balance) error
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Balance, error)
	FindByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (*mmodel.Balance, error)
	ExistsByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (bool, error)
	ListAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error)
	ListAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error)
	ListByAccountIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Balance, error)
	ListByAliases(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error)
	ListByAliasesWithKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, aliasesWithKeys []string) ([]*mmodel.Balance, error)
	BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) error
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	DeleteAllByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) error
	Sync(ctx context.Context, organizationID, ledgerID uuid.UUID, b mmodel.BalanceRedis) (bool, error)
	UpdateAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, balance mmodel.UpdateBalance) error
	ListByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.Balance, error)
}

// BalancePostgreSQLRepository is a Postgresql-specific implementation of the BalanceRepository.
type BalancePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewBalancePostgreSQLRepository returns a new instance of BalancePostgreSQLRepository using the given Postgres connection.
func NewBalancePostgreSQLRepository(pc *libPostgres.PostgresConnection) *BalancePostgreSQLRepository {
	assert.NotNil(pc, "PostgreSQL connection must not be nil", "repository", "BalancePostgreSQLRepository")

	db, err := pc.GetDB()
	assert.NoError(err, "database connection required for BalancePostgreSQLRepository",
		"repository", "BalancePostgreSQLRepository")
	assert.NotNil(db, "database handle must not be nil", "repository", "BalancePostgreSQLRepository")

	return &BalancePostgreSQLRepository{
		connection: pc,
		tableName:  "balance",
	}
}

func (r *BalancePostgreSQLRepository) Create(ctx context.Context, balance *mmodel.Balance) error {
	assert.NotNil(balance, "balance entity must not be nil for Create",
		"repository", "BalancePostgreSQLRepository")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balances")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return fmt.Errorf("failed: %w", err)
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

		return fmt.Errorf("failed: %w", err)
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return fmt.Errorf("failed: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create balance. Rows affected is 0", err)

		logger.Warnf("Failed to create balance. Rows affected is 0: %v", err)

		return fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, fmt.Errorf("failed: %w", err)
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
	}

	return balances, nil
}

// scanBalanceRows scans balance rows from database result set.
func scanBalanceRows(rows *sql.Rows) ([]*mmodel.Balance, error) {
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
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return balances, nil
}

// calculateBalancePagination calculates pagination cursor for balance results.
// hasPagination must be calculated BEFORE trimming results with PaginateRecords.
func calculateBalancePagination(balances []*mmodel.Balance, filter http.Pagination, decodedCursor libHTTP.Cursor, hasPagination bool) (libHTTP.CursorPagination, error) {
	if len(balances) == 0 {
		return libHTTP.CursorPagination{}, nil
	}

	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	cursor, err := libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, balances[0].ID, balances[len(balances)-1].ID)
	if err != nil {
		return libHTTP.CursorPagination{}, fmt.Errorf("failed to calculate cursor: %w", err)
	}

	return cursor, nil
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)
			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)
		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	balances, err := scanBalanceRows(rows)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)
		logger.Errorf("Failed to scan rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(balances) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext
	balances = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, balances, filter.Limit, orderDirection)

	cur, err := calculateBalancePagination(balances, filter, decodedCursor, hasPagination)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)
		logger.Errorf("Failed to calculate cursor: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)
			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_account_id.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)
		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	balances, err := scanBalanceRows(rows)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)
		logger.Errorf("Failed to scan rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(balances) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext
	balances = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, balances, filter.Limit, orderDirection)

	cur, err := calculateBalancePagination(balances, filter, decodedCursor, hasPagination)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)
		logger.Errorf("Failed to calculate cursor: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
	}

	if len(aliasesWithKeys) == 0 {
		return []*mmodel.Balance{}, nil
	}

	orConditions := squirrel.Or{}

	for _, aliasWithKey := range aliasesWithKeys {
		parts := strings.Split(aliasWithKey, "#")
		if len(parts) != aliasKeyParts {
			libOpentelemetry.HandleSpanError(&span, "Invalid alias#key format", fmt.Errorf("expected format: alias#key: %w", ErrInvalidAliasKeyFormat))

			logger.Errorf("Invalid alias#key format: %s", aliasWithKey)

			return nil, fmt.Errorf("parsing failed: %w", ErrInvalidAliasKeyFormat)
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

		return nil, fmt.Errorf("failed: %w", err)
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_aliases_with_keys.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
	}

	return balances, nil
}

// balanceUpdateResult holds the result of a single balance update operation
type balanceUpdateResult struct {
	updated bool
	err     error
}

// txExecutor abstracts transaction execution for balance updates
type txExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// executeBalanceUpdate performs a single balance update within a transaction
func (r *BalancePostgreSQLRepository) executeBalanceUpdate(ctx context.Context, tx txExecutor, organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) balanceUpdateResult {
	updates := make([]string, 0, balanceUpdateFieldCount)
	args := make([]any, 0, balanceUpdateArgCapacity)

	updates = append(updates, "available = $"+strconv.Itoa(len(args)+1))
	args = append(args, balance.Available)

	updates = append(updates, "on_hold = $"+strconv.Itoa(len(args)+1))
	args = append(args, balance.OnHold)

	updates = append(updates, "version = $"+strconv.Itoa(len(args)+1))
	args = append(args, balance.Version)

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
	args = append(args, time.Now(), organizationID, ledgerID, balance.ID, balance.Version)

	queryUpdate := `UPDATE balance SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-whereOrgIDOffset) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-whereLedgerIDOffset) +
		` AND id = $` + strconv.Itoa(len(args)-1) +
		` AND version < $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := tx.ExecContext(ctx, queryUpdate, args...)
	if err != nil {
		return balanceUpdateResult{false, fmt.Errorf("failed: %w", err)}
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return balanceUpdateResult{false, fmt.Errorf("failed: %w", err)}
	}

	return balanceUpdateResult{rowsAffected > 0, nil}
}

// processBalanceUpdates processes all balance updates within a transaction and returns the success count
func (r *BalancePostgreSQLRepository) processBalanceUpdates(ctx context.Context, tx txExecutor, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) (int, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	successCount := 0

	for _, balance := range balances {
		ctxBalance, spanUpdate := tracer.Start(ctx, "postgres.update_balance")
		result := r.executeBalanceUpdate(ctxBalance, tx, organizationID, ledgerID, balance)

		spanUpdate.End()

		if result.err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdate, "Err on balance update", result.err)
			logger.Errorf("Err on balance update: %v", result.err)

			return 0, result.err
		}

		if result.updated {
			successCount++
		} else {
			logger.Warnf("Balance update skipped (stale version): balance_id=%s, attempted_version=%d, possible_causes=[newer_version_in_database, concurrent_update, replay_protection]",
				balance.ID, balance.Version)
		}
	}

	return successCount, nil
}

// BalancesUpdate updates the balances in the database.
func (r *BalancePostgreSQLRepository) BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "postgres.update_balances")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		return fmt.Errorf("failed: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to init balances", err)
		return fmt.Errorf("failed: %w", err)
	}

	committed := false

	defer func() {
		if committed {
			return
		}

		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			libOpentelemetry.HandleSpanError(&span, "Failed to rollback balances update", rollbackErr)
			logger.Errorf("err on rollback: %v", rollbackErr)
		}
	}()

	successCount, err := r.processBalanceUpdates(ctx, tx, organizationID, ledgerID, balances)
	if err != nil {
		return err
	}

	if successCount == 0 && len(balances) > 0 {
		return fmt.Errorf("%w: %d attempted, 0 succeeded", ErrNoBalancesUpdated, len(balances))
	}

	commitErr := tx.Commit()
	if commitErr != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit balances update", commitErr)
		logger.Errorf("err on commit: %v", commitErr)

		return fmt.Errorf("failed: %w", commitErr)
	}

	committed = true

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

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, err
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

		return false, fmt.Errorf("failed: %w", err)
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

		return false, fmt.Errorf("failed: %w", err)
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	var exists bool
	if err := row.Scan(&exists); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return false, fmt.Errorf("failed: %w", err)
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

		return fmt.Errorf("failed: %w", err)
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

		return fmt.Errorf("failed: %w", err)
	}

	spanQuery.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return fmt.Errorf("failed: %w", err)
	}

	if rowsAffected == 0 {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance. Rows affected is 0", err)

		logger.Warnf("Failed to delete balance. Rows affected is 0: %v", err)

		return fmt.Errorf("failed: %w", err)
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

		return fmt.Errorf("failed: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to begin transaction for bulk delete", err)

		logger.Errorf("failed to begin transaction for bulk delete: %v", err)

		return fmt.Errorf("failed: %w", err)
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

		return fmt.Errorf("failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected on bulk delete", err)

		logger.Errorf("Failed to get rows affected on bulk delete: %v", err)

		return fmt.Errorf("failed: %w", err)
	}

	if rowsAffected != int64(len(ids)) {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balances. Rows affected mismatch", err)

		logger.Warnf("Failed to delete balances. Rows affected mismatch: %v", err)

		return fmt.Errorf("failed: %w", err)
	}

	if err = tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to commit transaction for bulk delete", err)

		logger.Errorf("failed to commit transaction for bulk delete: %v", err)

		return fmt.Errorf("failed: %w", err)
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

		return fmt.Errorf("failed: %w", err)
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
		` WHERE organization_id = $` + strconv.Itoa(len(args)-whereOrgIDOnlyOffset) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, queryUpdate, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Err on result exec content", err)

		logger.Errorf("Err on result exec content: %v", err)

		return fmt.Errorf("failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		if err == nil {
			err = sql.ErrNoRows
		}

		libOpentelemetry.HandleSpanError(&span, "Err on rows affected", err)

		logger.Errorf("Failed to update balance. Rows affected is 0: %v", err)

		return fmt.Errorf("failed: %w", err)
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

		return false, fmt.Errorf("failed: %w", err)
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return false, fmt.Errorf("failed: %w", err)
	}

	res, err := db.ExecContext(ctx, `
		UPDATE balance
		SET available = $1, on_hold = $2, version = $3, updated_at = $4
		WHERE organization_id = $5 AND ledger_id = $6 AND id = $7 AND deleted_at IS NULL AND version < $3
	`, b.Available, b.OnHold, b.Version, time.Now(), organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update balance from redis", err)

		logger.Errorf("Failed to update balance from redis: %v", err)

		return false, fmt.Errorf("failed: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to read rows affected", err)

		logger.Errorf("Failed to read rows affected: %v", err)

		return false, fmt.Errorf("failed: %w", err)
	}

	if affected == 0 {
		logger.Warnf("Balance sync skipped (stale version): balance_id=%s, redis_version=%d, possible_causes=[already_synced, concurrent_sync, replay_protection]",
			b.ID, b.Version)
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

		return fmt.Errorf("failed: %w", err)
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update_all_by_account_id.exec")
	defer spanExec.End()

	if balance.AllowSending == nil {
		libOpentelemetry.HandleSpanError(&spanExec, "allow_sending value is required", ErrAllowSendingRequired)

		logger.Errorf("allow_sending value is required: %v", ErrAllowSendingRequired)

		return fmt.Errorf("validation failed: %w", ErrAllowSendingRequired)
	}

	if balance.AllowReceiving == nil {
		libOpentelemetry.HandleSpanError(&spanExec, "allow_receiving value is required", ErrAllowReceivingRequired)

		logger.Errorf("allow_receiving value is required: %v", ErrAllowReceivingRequired)

		return fmt.Errorf("validation failed: %w", ErrAllowReceivingRequired)
	}

	query := `UPDATE balance SET allow_sending = $1, allow_receiving = $2, updated_at = NOW() WHERE organization_id = $3 AND ledger_id = $4 AND account_id = $5 AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, *balance.AllowSending, *balance.AllowReceiving, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return fmt.Errorf("failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return fmt.Errorf("failed: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to update balances. Rows affected is 0", err)

		logger.Warnf("Failed to update all balances by account id. Rows affected is 0: %v", err)

		return fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)
		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)
		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
	}

	return balances, nil
}
