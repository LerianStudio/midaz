package operation

import (
	"context"
	"database/sql"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Repository provides an interface for operations related to operation template entities.
// It defines methods for creating, retrieving, updating, and deleting operation templates.
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
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &OperationPostgreSQLModel{}
	record.FromEntity(operation)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "operation_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert operation record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO operation VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22) RETURNING *`,
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
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

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

		libOpentelemetry.HandleSpanError(&span, "Failed to create operation. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindAll retrieves Operations entities from the database.
func (r *OperationPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

	decodedCursor := libHTTP.Cursor{}
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !isFirstPage {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("transaction_id = ?", transactionID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDate(filter.StartDate, libPointers.Int(-1))}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDate(filter.EndDate, libPointers.Int(1))}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(operations) > filter.Limit

	operations = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, operations, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(operations) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, operations[0].ID, operations[len(operations)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}

// ListByIDs retrieves Operation entities from the database using the provided IDs.
func (r *OperationPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Operation, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_operations_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		return nil, err
	}

	var operations []*Operation

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM operation WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return operations, nil
}

// Find retrieves a Operation entity from the database using the provided ID.
func (r *OperationPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID) (*Operation, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	operation := &OperationPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM operation WHERE organization_id = $1 AND ledger_id = $2 AND transaction_id = $3 AND id = $4 AND deleted_at IS NULL",
		organizationID, ledgerID, transactionID, id)

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
	); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())
		}

		return nil, err
	}

	return operation.ToEntity(), nil
}

// FindByAccount retrieves a Operation entity from the database using the provided account ID.
func (r *OperationPostgreSQLRepository) FindByAccount(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*Operation, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations_by_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	operation := &OperationPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all_by_account.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM operation WHERE organization_id = $1 AND ledger_id = $2 AND account_id = $3 AND id = $4 AND deleted_at IS NULL",
		organizationID, ledgerID, accountID, id)

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
	); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())
		}

		return nil, err
	}

	return operation.ToEntity(), nil
}

// Update an Operation entity into Postgresql and returns the Operation updated.
func (r *OperationPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, transactionID, id uuid.UUID, operation *Operation) (*Operation, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &OperationPostgreSQLModel{}
	record.FromEntity(operation)

	var updates []string

	var args []any

	if operation.Description != "" {
		updates = append(updates, "description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Description)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, transactionID, id)

	query := `UPDATE operation SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-3) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-2) +
		` AND transaction_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "operation_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert operation record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

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

		libOpentelemetry.HandleSpanError(&span, "Failed to update operation. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Operation entity from the database using the provided IDs.
func (r *OperationPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	trace := libCommons.NewTracerFromContext(ctx)

	ctx, span := trace.Start(ctx, "postgres.delete_operation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := trace.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE operation SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute database query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Operation{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to delete operation. Rows affected is 0", err)

		return err
	}

	return nil
}

// FindAllByAccount retrieves Operations entities from the database using the provided account ID.
func (r *OperationPostgreSQLRepository) FindAllByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, operationType *string, filter http.Pagination) ([]*Operation, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operations_by_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	operations := make([]*Operation, 0)

	decodedCursor := libHTTP.Cursor{}
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !isFirstPage {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("account_id = ?", accountID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDate(filter.StartDate, libPointers.Int(-1))}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDate(filter.EndDate, libPointers.Int(2))}).
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

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all_by_account.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operations = append(operations, operation.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(operations) > filter.Limit

	operations = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, operations, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(operations) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, operations[0].ID, operations[len(operations)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operations, cur, nil
}
