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
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v4/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
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
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Balance, error)
	ListByAliases(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error)
	ListByAliasesWithKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, aliasesWithKeys []string) ([]*mmodel.Balance, error)
	BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) (*mmodel.Balance, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	DeleteAllByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) error
	Sync(ctx context.Context, organizationID, ledgerID uuid.UUID, b mmodel.BalanceRedis) (bool, error)
	SyncBatch(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []mmodel.BalanceRedis) (int64, error)
	UpdateAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, balance mmodel.UpdateBalance) error
	ListByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.Balance, error)
	ListByAccountIDAtTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time) ([]*mmodel.Balance, error)
}

// BalancePostgreSQLRepository is a Postgresql-specific implementation of the BalanceRepository.
type BalancePostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewBalancePostgreSQLRepository returns a new instance of BalancePostgreSQLRepository using the given Postgres connection.
func NewBalancePostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *BalancePostgreSQLRepository {
	c := &BalancePostgreSQLRepository{
		connection: pc,
		tableName:  "balance",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *BalancePostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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

func (r *BalancePostgreSQLRepository) Create(ctx context.Context, balance *mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balances")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

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
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create balance. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to create balance. Rows affected is 0: %v", err))

		return err
	}

	return nil
}

// ListByAccountIDs list Balances entity from the database using the provided accountIDs.
func (r *BalancePostgreSQLRepository) ListByAccountIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIds []uuid.UUID) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
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
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate rows: %v", err))

		return nil, err
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

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
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
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("ListByIDs query: %s with args: %v", sqlQuery, args))

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate rows: %v", err))

		return nil, err
	}

	return balances, nil
}

// ListAll list Balances entity from the database.
func (r *BalancePostgreSQLRepository) ListAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_balances")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	balances := make([]*mmodel.Balance, 0)

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

	findAll := squirrel.Select(balanceColumnList...).
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

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to get operations on repo", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get operations on repo: %v", err))

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate rows: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(balances) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)

	balances = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.Direction, balances, filter.Limit)

	cur := libHTTP.CursorPagination{}
	if len(balances) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.Direction, balances[0].ID, balances[len(balances)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to calculate cursor: %v", err))

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

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	balances := make([]*mmodel.Balance, 0)

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

	findAll := squirrel.Select(balanceColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("account_id = ?", accountID)).
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

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_account_id.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to get operations on repo", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get operations on repo: %v", err))

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate rows: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(balances) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)

	balances = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.Direction, balances, filter.Limit)

	cur := libHTTP.CursorPagination{}
	if len(balances) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.Direction, balances[0].ID, balances[len(balances)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to calculate cursor: %v", err))

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

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
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
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate rows: %v", err))

		return nil, err
	}

	return balances, nil
}

// ListByAliasesWithKeys list Balances entity from the database using the provided alias#key pairs.
func (r *BalancePostgreSQLRepository) ListByAliasesWithKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, aliasesWithKeys []string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_aliases_with_keys")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	if len(aliasesWithKeys) == 0 {
		return []*mmodel.Balance{}, nil
	}

	orConditions := squirrel.Or{}

	for _, aliasWithKey := range aliasesWithKeys {
		parts := strings.Split(aliasWithKey, "#")
		if len(parts) != 2 {
			libOpentelemetry.HandleSpanError(span, "Invalid alias#key format", errors.New("expected format: alias#key"))

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid alias#key format: %s", aliasWithKey))

			return nil, errors.New("invalid alias#key format")
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
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_aliases_with_keys.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate rows: %v", err))

		return nil, err
	}

	return balances, nil
}

// BalancesUpdate updates the balances in the database.
func (r *BalancePostgreSQLRepository) BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "postgres.update_balances")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to init balances", err)

		return err
	}

	defer func() {
		if err != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to init balances", rollbackErr)

				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("err on rollback: %v", rollbackErr))
			}
		} else {
			commitErr := tx.Commit()
			if commitErr != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to init balances", commitErr)

				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("err on commit: %v", commitErr))
			}
		}
	}()

	for _, balance := range balances {
		ctxBalance, spanUpdate := tracer.Start(ctx, "postgres.update_balance")

		updates := make([]string, 0, 4)

		args := make([]any, 0, 8)

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
			libOpentelemetry.HandleSpanError(spanUpdate, "Err on result exec content", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Err on result exec content: %v", err))

			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			libOpentelemetry.HandleSpanError(spanUpdate, "Err ", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Err: %v", err))

			return err
		}

		if rowsAffected == 0 {
			logger.Log(ctx, libLog.LevelInfo, "Zero rows affected")

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

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
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
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

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

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return nil, err
	}

	return balance.ToEntity(), nil
}

// FindByAccountIDAndKey retrieves a balance record based on accountID and key within the specified organization and ledger.
func (r *BalancePostgreSQLRepository) FindByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_balance_by_account_id_and_key")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

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

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return nil, err
	}

	return balance.ToEntity(), nil
}

// ExistsByAccountIDAndKey returns true if a balance exists for the given accountID and key within the specified organization and ledger.
func (r *BalancePostgreSQLRepository) ExistsByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.exists_balance_by_account_id_and_key")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

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
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return false, err
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	var exists bool
	if err := row.Scan(&exists); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return false, err
	}

	return exists, nil
}

// Delete marks a balance as deleted in the database using the ID provided
func (r *BalancePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_balance")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

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
		libOpentelemetry.HandleSpanError(span, "failed to execute delete query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to execute delete query: %v", err))

		return err
	}

	spanQuery.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows affected: %v", err))

		return err
	}

	if rowsAffected == 0 {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balance. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to delete balance. Rows affected is 0: %v", err))

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

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to begin transaction for bulk delete", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to begin transaction for bulk delete: %v", err))

		return err
	}

	committed := false

	defer func() {
		if committed {
			return
		}

		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			libOpentelemetry.HandleSpanError(span, "failed to rollback transaction for bulk delete", rollbackErr)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to rollback transaction for bulk delete: %v", rollbackErr))
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
		libOpentelemetry.HandleSpanError(spanExec, "failed to execute bulk delete query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to execute bulk delete query: %v", err))

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected on bulk delete", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows affected on bulk delete: %v", err))

		return err
	}

	if rowsAffected != int64(len(ids)) {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balances. Rows affected mismatch", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to delete balances. Rows affected mismatch: %v", err))

		return err
	}

	if err = tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to commit transaction for bulk delete", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("failed to commit transaction for bulk delete: %v", err))

		return err
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

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
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
	args = append(args, time.Now(), organizationID, ledgerID, id)

	queryUpdate := `UPDATE balance SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
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
			err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance not found", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Balance not found: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to update balance", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update balance: %v", err))

		return nil, err
	}

	return record.ToEntity(), nil
}

func (r *BalancePostgreSQLRepository) Sync(ctx context.Context, organizationID, ledgerID uuid.UUID, b mmodel.BalanceRedis) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.sync_balance")
	defer span.End()

	id, err := uuid.Parse(b.ID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "invalid balance ID", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("invalid balance ID: %v", err))

		return false, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return false, err
	}

	res, err := db.ExecContext(ctx, `
		UPDATE balance
		SET available = $1, on_hold = $2, version = $3, updated_at = $4
		WHERE organization_id = $5 AND ledger_id = $6 AND id = $7 AND deleted_at IS NULL AND version < $3
	`, b.Available, b.OnHold, b.Version, time.Now(), organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update balance from redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update balance from redis: %v", err))

		return false, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read rows affected", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to read rows affected: %v", err))

		return false, err
	}

	return affected > 0, nil
}

// SyncBatch persists multiple balances from cache to database in a single UPDATE statement.
// Uses a VALUES clause to send all balances in one round-trip, which is significantly
// faster than individual UPDATEs (1 round-trip vs N).
//
// Optimistic locking: only updates rows where version < incoming version.
// A single statement is atomic in PostgreSQL — no explicit transaction needed.
// Returns count of actually updated rows (rows with stale versions are skipped).
func (r *BalancePostgreSQLRepository) SyncBatch(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
	if len(balances) == 0 {
		return 0, nil
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.sync_batch")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return 0, err
	}

	// Validate all IDs upfront before building the query.
	ids := make([]uuid.UUID, len(balances))
	for i, balance := range balances {
		id, parseErr := uuid.Parse(balance.ID)
		if parseErr != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid balance ID", parseErr)
			logger.Log(ctx, libLog.LevelError, "Invalid balance ID in batch",
				libLog.String("balance_id", balance.ID), libLog.Err(parseErr))

			return 0, parseErr
		}

		ids[i] = id
	}

	// Build a single UPDATE ... FROM (VALUES ...) statement.
	// Each balance contributes 4 parameters: (id, available, on_hold, version).
	// Shared parameters (updated_at, organization_id, ledger_id) are appended at the end.
	now := time.Now()
	valuesClauses := make([]string, len(balances))
	args := make([]any, 0, len(balances)*4+3)
	paramIdx := 1

	for i, balance := range balances {
		valuesClauses[i] = fmt.Sprintf("($%d::uuid, $%d::numeric, $%d::numeric, $%d::bigint)",
			paramIdx, paramIdx+1, paramIdx+2, paramIdx+3)
		args = append(args, ids[i], balance.Available, balance.OnHold, balance.Version)
		paramIdx += 4
	}

	// Shared parameters: updated_at, organization_id, ledger_id
	nowIdx := paramIdx
	orgIdx := paramIdx + 1
	ledgerIdx := paramIdx + 2
	args = append(args, now, organizationID, ledgerID)

	query := fmt.Sprintf(`
		UPDATE balance AS b
		SET available = v.available,
		    on_hold = v.on_hold,
		    version = v.version,
		    updated_at = $%d
		FROM (VALUES %s) AS v(id, available, on_hold, version)
		WHERE b.id = v.id
		  AND b.organization_id = $%d
		  AND b.ledger_id = $%d
		  AND b.version < v.version
		  AND b.deleted_at IS NULL
	`, nowIdx, strings.Join(valuesClauses, ", "), orgIdx, ledgerIdx)

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute batch sync", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute batch sync", libLog.Err(err))

		return 0, err
	}

	totalUpdated, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected", libLog.Err(err))

		return 0, err
	}

	logger.Log(ctx, libLog.LevelInfo, "SyncBatch completed",
		libLog.Int("updated", int(totalUpdated)),
		libLog.Int("total", len(balances)),
	)

	return totalUpdated, nil
}

func (r *BalancePostgreSQLRepository) UpdateAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, balance mmodel.UpdateBalance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_all_by_account_id")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update_all_by_account_id.exec")
	defer spanExec.End()

	if balance.AllowSending == nil {
		err := errors.New("allow_sending value is required")

		libOpentelemetry.HandleSpanError(spanExec, "allow_sending value is required", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("allow_sending value is required: %v", err))

		return err
	}

	if balance.AllowReceiving == nil {
		err := errors.New("allow_receiving value is required")

		libOpentelemetry.HandleSpanError(spanExec, "allow_receiving value is required", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("allow_receiving value is required: %v", err))

		return err
	}

	query := `UPDATE balance SET allow_sending = $1, allow_receiving = $2, updated_at = NOW() WHERE organization_id = $3 AND ledger_id = $4 AND account_id = $5 AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, *balance.AllowSending, *balance.AllowReceiving, organizationID, ledgerID, accountID)
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to update balances. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to update all balances by account id. Rows affected is 0: %v", err))

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

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
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
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate rows: %v", err))

		return nil, err
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

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
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
		From("operation").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"account_id": accountID}).
		Where(squirrel.LtOrEq{"created_at": timestamp}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("balance_id", "created_at DESC", "balance_version_after DESC", "id DESC")

	latestOpsSql, latestOpsArgs, err := latestOpsSubquery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build CTE subquery", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build CTE subquery: %v", err))

		return nil, err
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
		From("balance b").
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
		libOpentelemetry.HandleSpanError(span, "Failed to build main query", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build main query: %v", err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("ListByAccountIDAtTimestamp query: %s with args: %v", sqlQuery, args))

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_balances_by_account_id_at_timestamp.query")

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return nil, err
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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate rows: %v", err))

		return nil, err
	}

	return balances, nil
}
