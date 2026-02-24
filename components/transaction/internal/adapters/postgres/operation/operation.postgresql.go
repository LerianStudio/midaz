// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// Repository provides an interface for operations related to operation template entities.
// It defines methods for creating, retrieving, updating, and deleting operation templates.
//
//go:generate mockgen --destination=operation.postgresql_mock.go --package=operation . Repository
type Repository interface {
	Create(ctx context.Context, operation *Operation) (*Operation, error)
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

// operationColumns defines the explicit column list for operation table queries.
// This ensures backward compatibility when new columns are added in future versions.
const operationColumns = "id, transaction_id, description, type, asset_code, amount, available_balance, on_hold_balance, available_balance_after, on_hold_balance_after, status, status_description, account_id, account_alias, balance_id, chart_of_accounts, organization_id, ledger_id, created_at, updated_at, deleted_at, route, balance_affected, balance_key, balance_version_before, balance_version_after"

// OperationPostgreSQLRepository is a Postgresql-specific implementation of the OperationRepository.
type OperationPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
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
func NewOperationPostgreSQLRepository(pc *libPostgres.PostgresConnection) *OperationPostgreSQLRepository {
	c := &OperationPostgreSQLRepository{
		connection: pc,
		tableName:  "operation",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
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
		).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := insert.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to build insert query", err)

		logger.Errorf("Failed to build insert query: %v", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			libOpentelemetry.HandleSpanEvent(&spanExec, "Operation already exists, skipping duplicate insert (idempotent retry)")

			logger.Infof("Operation already exists, skipping duplicate insert (idempotent retry)")

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation. Rows affected is 0", err)

		logger.Warnf("Failed to create operation. Rows affected is 0: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
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

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

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

	findAll := squirrel.Select(operationColumns).
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

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
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

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_ids.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, err
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
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

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
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation not found", err)

			logger.Warnf("Operation not found: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
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
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

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
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation not found", err)

			logger.Warnf("Operation not found: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
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
		libOpentelemetry.HandleSpanError(&span, "Failed to build update query", err)

		logger.Errorf("Failed to build update query: %v", err)

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation. Rows affected is 0", err)

		logger.Warnf("Failed to update operation. Rows affected is 0: %v", err)

		return nil, err
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

		return err
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

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute database query", err)

		logger.Errorf("Failed to execute database query: %v", err)

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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete operation. Rows affected is 0", err)

		logger.Warnf("Failed to delete operation. Rows affected is 0: %v", err)

		return err
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

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

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

	findAll := squirrel.Select(operationColumns).
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

		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Debugf("FindAllByAccount query: %s with args: %v", query, args)

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all_by_account.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		logger.Errorf("Failed to query database: %v", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
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

			return nil, libHTTP.CursorPagination{}, err
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

		return nil, err
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

		return nil, err
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

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, err
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

	outerQuery, orderDirection = libHTTP.ApplyCursorPagination(outerQuery, decodedCursor, orderDirection, filter.Limit)

	query, args, err := outerQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build outer query", err)
		logger.Errorf("Failed to build outer query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Debugf("FindLastOperationsForAccountBeforeTimestamp query: %s with args: %v", query, args)

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_last_operations_for_account_before_timestamp.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)
		logger.Errorf("Failed to query database: %v", err)

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
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)
			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)
		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
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

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}
