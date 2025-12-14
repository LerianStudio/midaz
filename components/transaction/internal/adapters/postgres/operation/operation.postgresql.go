package operation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
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
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
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
}

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

// NewOperationPostgreSQLRepository returns a new instance of OperationPostgreSQLRepository using the given Postgres connection.
func NewOperationPostgreSQLRepository(pc *libPostgres.PostgresConnection) *OperationPostgreSQLRepository {
	assert.NotNil(pc, "PostgreSQL connection must not be nil", "repository", "OperationPostgreSQLRepository")

	db, err := pc.GetDB()
	assert.NoError(err, "database connection required for OperationPostgreSQLRepository",
		"repository", "OperationPostgreSQLRepository")
	assert.NotNil(db, "database handle must not be nil", "repository", "OperationPostgreSQLRepository")

	return &OperationPostgreSQLRepository{
		connection: pc,
		tableName:  "operation",
	}
}

// Create a new Operation entity into Postgresql and returns it.
func (r *OperationPostgreSQLRepository) Create(ctx context.Context, operation *Operation) (*Operation, error) {
	assert.NotNil(operation, "operation entity must not be nil for Create",
		"repository", "OperationPostgreSQLRepository")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, fmt.Errorf("failed: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation. Rows affected is 0", err)

		logger.Warnf("Failed to create operation. Rows affected is 0: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
	}

	return record.ToEntity(), nil
}

// scanOperationRows scans operation rows from database result set.
func scanOperationRows(rows *sql.Rows) ([]*Operation, error) {
	operations := make([]*Operation, 0)

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
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get rows: %w", err)
	}

	return operations, nil
}

// calculateOperationPagination calculates pagination cursor for operation results.
// hasPagination must be calculated BEFORE trimming results with PaginateRecords.
func calculateOperationPagination(operations []*Operation, filter http.Pagination, decodedCursor libHTTP.Cursor, hasPagination bool) (libHTTP.CursorPagination, error) {
	if len(operations) == 0 {
		return libHTTP.CursorPagination{}, nil
	}

	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	cursor, err := libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, operations[0].ID, operations[len(operations)-1].ID)
	if err != nil {
		return libHTTP.CursorPagination{}, fmt.Errorf("failed to calculate cursor: %w", err)
	}

	return cursor, nil
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)
		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	operations, err := scanOperationRows(rows)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)
		logger.Errorf("Failed to scan rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	operations, hasPagination := paginateOperations(operations, filter, decodedCursor, orderDirection)

	cur, err := calculateOperationPagination(operations, filter, decodedCursor, hasPagination)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)
		logger.Errorf("Failed to calculate cursor: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		logger.Errorf("Failed to get operations on repo: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

			return nil, fmt.Errorf("failed: %w", err)
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
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

		return nil, fmt.Errorf("failed: %w", err)
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation. Rows affected is 0", err)

		logger.Warnf("Failed to update operation. Rows affected is 0: %v", err)

		return nil, fmt.Errorf("failed: %w", err)
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

		return fmt.Errorf("failed: %w", err)
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

		return fmt.Errorf("failed: %w", err)
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute database query", err)

		logger.Errorf("Failed to execute database query: %v", err)

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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete operation. Rows affected is 0", err)

		logger.Warnf("Failed to delete operation. Rows affected is 0: %v", err)

		return fmt.Errorf("failed: %w", err)
	}

	return nil
}

// decodeCursorWithDefault decodes the cursor from the filter or returns a default cursor
func decodeCursorWithDefault(filter http.Pagination) (libHTTP.Cursor, string, error) {
	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		var err error

		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			return libHTTP.Cursor{}, "", fmt.Errorf("failed to decode cursor: %w", err)
		}
	}

	return decodedCursor, orderDirection, nil
}

// queryContextExecutor is an interface for types that can execute queries
type queryContextExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// executeOperationQuery executes the query and returns the scanned operations
func (r *OperationPostgreSQLRepository) executeOperationQuery(ctx context.Context, db queryContextExecutor, query string, args []any) ([]*Operation, error) {
	logger, tracer, spanCtx, spanTracer := libCommons.NewTrackingFromContext(ctx)
	_ = logger
	_ = spanCtx
	_ = spanTracer

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all_by_account.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)
		spanQuery.End()

		return nil, fmt.Errorf("failed: %w", err)
	}
	defer rows.Close()

	spanQuery.End()

	return scanOperationRows(rows)
}

// paginateOperations applies pagination logic to the operations list
// paginateOperations trims operation results and returns hasPagination flag for cursor calculation.
func paginateOperations(operations []*Operation, filter http.Pagination, decodedCursor libHTTP.Cursor, orderDirection string) ([]*Operation, bool) {
	hasPagination := len(operations) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	trimmed := libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, operations, filter.Limit, orderDirection)
	return trimmed, hasPagination
}

// buildOperationByAccountQuery constructs the SQL query for finding operations by account
func buildOperationByAccountQuery(r *OperationPostgreSQLRepository, organizationID, ledgerID, accountID uuid.UUID, operationType *string, filter http.Pagination, decodedCursor libHTTP.Cursor, orderDirection string) (string, []any, string, error) {
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
		return "", nil, "", fmt.Errorf("failed to build SQL query: %w", err)
	}

	return query, args, orderDirection, nil
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

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	decodedCursor, orderDirection, err := decodeCursorWithDefault(filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)
		logger.Errorf("Failed to decode cursor: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	query, args, orderDirection, err := buildOperationByAccountQuery(r, organizationID, ledgerID, accountID, operationType, filter, decodedCursor, orderDirection)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)
		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	operations, err := r.executeOperationQuery(ctx, db, query, args)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute query", err)
		logger.Errorf("Failed to execute query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	operations, hasPagination := paginateOperations(operations, filter, decodedCursor, orderDirection)

	cur, err := calculateOperationPagination(operations, filter, decodedCursor, hasPagination)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)
		logger.Errorf("Failed to calculate cursor: %v", err)

		return nil, libHTTP.CursorPagination{}, fmt.Errorf("failed: %w", err)
	}

	return operations, cur, nil
}
