package transaction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/operation"
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

// Repository provides an interface for operations related to transaction template entities.
// It defines methods for creating, retrieving, updating, and deleting transactions.
type Repository interface {
	Create(ctx context.Context, transaction *Transaction) (*Transaction, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
	FindByParentID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*Transaction, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Transaction, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *Transaction) (*Transaction, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	FindWithOperations(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
	FindOrListAllWithOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error)
}

// TransactionPostgreSQLRepository is a Postgresql-specific implementation of the TransactionRepository.
type TransactionPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewTransactionPostgreSQLRepository returns a new instance of TransactionPostgreSQLRepository using the given Postgres connection.
func NewTransactionPostgreSQLRepository(pc *libPostgres.PostgresConnection) *TransactionPostgreSQLRepository {
	c := &TransactionPostgreSQLRepository{
		connection: pc,
		tableName:  "transaction",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new Transaction entity into Postgresql and returns it.
func (r *TransactionPostgreSQLRepository) Create(ctx context.Context, transaction *Transaction) (*Transaction, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &TransactionPostgreSQLModel{}
	record.FromEntity(transaction)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "transaction_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert transaction record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO transaction VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING *`,
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
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindAll retrieves Transactions entities from the database.
func (r *TransactionPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_transactions")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	transactions := make([]*Transaction, 0)

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
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &transaction.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				return nil, libHTTP.CursorPagination{}, err
			}
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(transactions) > filter.Limit

	transactions = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(transactions) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions[0].ID, transactions[len(transactions)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return transactions, cur, nil
}

// ListByIDs retrieves Transaction entities from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Transaction, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_transactions_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var transactions []*Transaction

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, "SELECT * FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &transaction.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				return nil, err
			}
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return transactions, nil
}

// Find retrieves a Transaction entity from the database using the provided ID.
func (r *TransactionPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	transaction := &TransactionPostgreSQLModel{}

	var body *string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	row := db.QueryRowContext(ctx, "SELECT * FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)

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
	); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())
		}

		return nil, err
	}

	if !libCommons.IsNilOrEmpty(body) {
		err = json.Unmarshal([]byte(*body), &transaction.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

			return nil, err
		}
	}

	return transaction.ToEntity(), nil
}

// FindByParentID retrieves a Transaction entity from the database using the provided parent ID.
func (r *TransactionPostgreSQLRepository) FindByParentID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*Transaction, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	transaction := &TransactionPostgreSQLModel{}

	var body *string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	row := db.QueryRowContext(ctx, "SELECT * FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND parent_transaction_id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, parentID)

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
	); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	if !libCommons.IsNilOrEmpty(body) {
		err = json.Unmarshal([]byte(*body), &transaction.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

			return nil, err
		}
	}

	return transaction.ToEntity(), nil
}

// Update a Transaction entity into Postgresql and returns the Transaction updated.
func (r *TransactionPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *Transaction) (*Transaction, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

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

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "transaction_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert transaction record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to update transaction. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Transaction entity from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, `UPDATE transaction SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to delete transaction. Rows affected is 0", err)

		return err
	}

	return nil
}

// FindWithOperations retrieves a Transaction and Operations entity from the database using the provided ID .
func (r *TransactionPostgreSQLRepository) FindWithOperations(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_with_operations")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_transaction_with_operations.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, "SELECT * FROM transaction t INNER JOIN operation o ON t.id = o.transaction_id WHERE t.organization_id = $1 AND t.ledger_id = $2 AND t.id = $3 AND t.deleted_at IS NULL",
		organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

			return nil, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &tran.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				return nil, err
			}
		}

		newTransaction = tran.ToEntity()
		operations = append(operations, op.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	newTransaction.Operations = operations

	return newTransaction, nil
}

// FindOrListAllWithOperations retrieves a list of transactions from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) FindOrListAllWithOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_or_list_all_with_operations")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

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

	subQuery := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDate(filter.StartDate, libPointers.Int(-1))}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDate(filter.EndDate, libPointers.Int(1))}).
		PlaceholderFormat(squirrel.Dollar)

	if len(ids) > 0 {
		subQuery = subQuery.Where(squirrel.Expr("id = ANY(?)", pq.Array(ids)))
	}

	subQuery, orderDirection = libHTTP.ApplyCursorPagination(subQuery, decodedCursor, orderDirection, filter.Limit)

	findAll := squirrel.
		Select("*").
		FromSelect(subQuery, "t").
		LeftJoin("operation o ON t.id = o.transaction_id").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	transactions := make([]*Transaction, 0)
	transactionsMap := make(map[uuid.UUID]*Transaction)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &tran.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				return nil, libHTTP.CursorPagination{}, err
			}
		}

		t, exists := transactionsMap[uuid.MustParse(tran.ID)]
		if !exists {
			t = tran.ToEntity()

			t.Operations = append(t.Operations, op.ToEntity())
			transactionsMap[t.IDtoUUID()] = t
		} else {
			t.Operations = append(t.Operations, op.ToEntity())
		}
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	for _, t := range transactionsMap {
		transactions = append(transactions, t)
	}

	hasPagination := len(transactions) > filter.Limit

	transactions = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(transactions) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions[0].ID, transactions[len(transactions)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return transactions, cur, nil
}
