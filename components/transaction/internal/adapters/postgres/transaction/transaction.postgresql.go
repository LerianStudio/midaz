package transaction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
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
}

const (
	whereOrgIDOffset = 2
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
	assert.NotNil(pc, "PostgreSQL connection must not be nil", "repository", "TransactionPostgreSQLRepository")

	db, err := pc.GetDB()
	assert.NoError(err, "database connection required for TransactionPostgreSQLRepository",
		"repository", "TransactionPostgreSQLRepository")
	assert.NotNil(db, "database handle must not be nil", "repository", "TransactionPostgreSQLRepository")

	return &TransactionPostgreSQLRepository{
		connection: pc,
		tableName:  "transaction",
	}
}

// Create a new Transaction entity into Postgresql and returns it.
func (r *TransactionPostgreSQLRepository) Create(ctx context.Context, transaction *Transaction) (*Transaction, error) {
	assert.NotNil(transaction, "transaction entity must not be nil for Create",
		"repository", "TransactionPostgreSQLRepository")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	record := &TransactionPostgreSQLModel{}
	record.FromEntity(transaction)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute insert transaction query", err)

			logger.Errorf("Failed to execute insert transaction query: %v", err)

			return nil, pkg.ValidateInternalError(err, "Transaction")
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction. Rows affected is 0", err)

		logger.Warnf("Failed to create transaction. Rows affected is 0: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// buildTransactionFindAllQuery constructs the SQL query for finding all transactions
func buildTransactionFindAllQuery(r *TransactionPostgreSQLRepository, organizationID, ledgerID uuid.UUID, filter http.Pagination, decodedCursor libHTTP.Cursor, orderDirection string) (string, []any, string, error) {
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
		return "", nil, "", pkg.ValidateInternalError(err, "Transaction")
	}

	return query, args, orderDirection, nil
}

// scanTransactionRows scans rows into transaction entities
func scanTransactionRows(rows *sql.Rows) ([]*Transaction, error) {
	transactions := make([]*Transaction, 0)

	for rows.Next() {
		var (
			transaction TransactionPostgreSQLModel
			body        *string
		)

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
			return nil, pkg.ValidateInternalError(err, "Transaction")
		}

		if !libCommons.IsNilOrEmpty(body) {
			if err := json.Unmarshal([]byte(*body), &transaction.Body); err != nil {
				return nil, pkg.ValidateInternalError(err, "Transaction")
			}
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	return transactions, nil
}

// decodeCursorOrDefault decodes the cursor from the filter or returns a default cursor
func decodeCursorOrDefault(filter http.Pagination) (libHTTP.Cursor, string, error) {
	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		var err error

		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			return libHTTP.Cursor{}, "", pkg.ValidateInternalError(err, "Transaction")
		}
	}

	return decodedCursor, orderDirection, nil
}

// queryContextExecutor is an interface for types that can execute queries
type queryContextExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// executeTransactionQuery executes the query and returns the scanned transactions
func (r *TransactionPostgreSQLRepository) executeTransactionQuery(ctx context.Context, db queryContextExecutor, query string, args []any) ([]*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)
		logger.Errorf("Failed to execute query: %v", err)
		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}
	defer rows.Close()

	spanQuery.End()

	transactions, err := scanTransactionRows(rows)
	if err != nil {
		logger.Errorf("Failed to scan rows: %v", err)
		return nil, err
	}

	return transactions, nil
}

// paginateTransactions applies pagination logic to the transactions list
func (r *TransactionPostgreSQLRepository) paginateTransactions(transactions []*Transaction, filter http.Pagination, decodedCursor libHTTP.Cursor, orderDirection string) []*Transaction {
	hasPagination := len(transactions) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	return libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions, filter.Limit, orderDirection)
}

// calculateTransactionCursor calculates the cursor pagination for transactions
func (r *TransactionPostgreSQLRepository) calculateTransactionCursor(ctx context.Context, transactions []*Transaction, filter http.Pagination, decodedCursor libHTTP.Cursor) (libHTTP.CursorPagination, error) {
	logger, tracer, spanCtx, spanTracer := libCommons.NewTrackingFromContext(ctx)
	_ = tracer
	_ = spanCtx
	_ = spanTracer

	cur := libHTTP.CursorPagination{}
	if len(transactions) == 0 {
		return cur, nil
	}

	hasPagination := len(transactions) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	var err error

	cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions[0].ID, transactions[len(transactions)-1].ID)
	if err != nil {
		logger.Errorf("Failed to calculate cursor: %v", err)
		return libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "Transaction")
	}

	return cur, nil
}

// FindAll retrieves Transactions entities from the database.
func (r *TransactionPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_transactions")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "Transaction")
	}

	decodedCursor, orderDirection, err := decodeCursorOrDefault(filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)
		logger.Errorf("Failed to decode cursor: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, orderDirection, err := buildTransactionFindAllQuery(r, organizationID, ledgerID, filter, decodedCursor, orderDirection)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)
		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	transactions, err := r.executeTransactionQuery(ctx, db, query, args)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	transactions = r.paginateTransactions(transactions, filter, decodedCursor, orderDirection)

	cur, err := r.calculateTransactionCursor(ctx, transactions, filter, decodedCursor)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	return transactions, cur, nil
}

// ListByIDs retrieves Transaction entities from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_transactions_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	var transactions []*Transaction

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")
	defer spanQuery.End()

	listByIDs := squirrel.Select(transactionColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := listByIDs.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(Transaction{}).Name())
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
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

			logger.Errorf("Failed to scan row: %v", err)

			return nil, pkg.ValidateInternalError(err, "Transaction")
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &transaction.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				logger.Errorf("Failed to unmarshal body: %v", err)

				return nil, pkg.ValidateInternalError(err, "Transaction")
			}
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	return transactions, nil
}

// Find retrieves a Transaction entity from the database using the provided ID.
func (r *TransactionPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	transaction := &TransactionPostgreSQLModel{}

	var body *string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	find := squirrel.Select(transactionColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ?", id)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := find.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(Transaction{}).Name())
	}

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
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction not found", err)

			logger.Warnf("Transaction not found: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	if !libCommons.IsNilOrEmpty(body) {
		err = json.Unmarshal([]byte(*body), &transaction.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

			logger.Errorf("Failed to unmarshal body: %v", err)

			return nil, pkg.ValidateInternalError(err, "Transaction")
		}
	}

	return transaction.ToEntity(), nil
}

// FindByParentID retrieves a Transaction entity from the database using the provided parent ID.
func (r *TransactionPostgreSQLRepository) FindByParentID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	transaction := &TransactionPostgreSQLModel{}

	var body *string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	findByParent := squirrel.Select(transactionColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("parent_transaction_id = ?", parentID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findByParent.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(Transaction{}).Name())
	}

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
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "No transaction found", err)

			logger.Errorf("No transaction found: %v", err)

			return nil, nil
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	if !libCommons.IsNilOrEmpty(body) {
		err = json.Unmarshal([]byte(*body), &transaction.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

			logger.Errorf("Failed to unmarshal body: %v", err)

			return nil, pkg.ValidateInternalError(err, "Transaction")
		}
	}

	return transaction.ToEntity(), nil
}

// Update a Transaction entity into Postgresql and returns the Transaction updated.
func (r *TransactionPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *Transaction) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
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
		` WHERE organization_id = $` + strconv.Itoa(len(args)-whereOrgIDOffset) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction. Rows affected is 0", err)

		logger.Warnf("Failed to update transaction. Rows affected is 0: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Transaction entity from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_transaction")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return pkg.ValidateInternalError(err, "Transaction")
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, "UPDATE transaction SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return pkg.ValidateInternalError(err, "Transaction")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return pkg.ValidateInternalError(err, "Transaction")
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete transaction. Rows affected is 0", err)

		logger.Warnf("Failed to delete transaction. Rows affected is 0: %v", err)

		return err
	}

	return nil
}

// FindWithOperations retrieves a Transaction and Operations entity from the database using the provided ID .
func (r *TransactionPostgreSQLRepository) FindWithOperations(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_with_operations")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_transaction_with_operations.query")
	defer spanQuery.End()

	operationColumnListPrefixed := []string{
		"o.id", "o.transaction_id", "o.description", "o.type", "o.asset_code",
		"o.amount", "o.available_balance", "o.on_hold_balance", "o.available_balance_after",
		"o.on_hold_balance_after", "o.status", "o.status_description", "o.account_id",
		"o.account_alias", "o.balance_id", "o.chart_of_accounts", "o.organization_id",
		"o.ledger_id", "o.created_at", "o.updated_at", "o.deleted_at", "o.route",
		"o.balance_affected", "o.balance_key", "o.balance_version_before", "o.balance_version_after",
	}

	selectColumns := slices.Concat(transactionColumnListPrefixed, operationColumnListPrefixed)

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
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(Transaction{}).Name())
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
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
			&op.BalanceAffected,
			&op.BalanceKey,
			&op.VersionBalance,
			&op.VersionBalanceAfter,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

			logger.Errorf("Failed to scan rows: %v", err)

			return nil, pkg.ValidateInternalError(err, "Transaction")
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &tran.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				logger.Errorf("Failed to unmarshal body: %v", err)

				return nil, pkg.ValidateInternalError(err, "Transaction")
			}
		}

		newTransaction = tran.ToEntity()
		operations = append(operations, op.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	newTransaction.Operations = operations

	return newTransaction, nil
}

// operationNullableRow holds nullable fields from operation scan.
type operationNullableRow struct {
	ID                    sql.NullString
	TransactionID         sql.NullString
	Description           sql.NullString
	Type                  sql.NullString
	AssetCode             sql.NullString
	Amount                decimal.NullDecimal
	AvailableBalance      decimal.NullDecimal
	OnHoldBalance         decimal.NullDecimal
	AvailableBalanceAfter decimal.NullDecimal
	OnHoldBalanceAfter    decimal.NullDecimal
	Status                sql.NullString
	StatusDescription     sql.NullString
	AccountID             sql.NullString
	AccountAlias          sql.NullString
	BalanceID             sql.NullString
	ChartOfAccounts       sql.NullString
	OrganizationID        sql.NullString
	LedgerID              sql.NullString
	CreatedAt             sql.NullTime
	UpdatedAt             sql.NullTime
	DeletedAt             sql.NullTime
	Route                 sql.NullString
	BalanceAffected       sql.NullBool
	BalanceKey            sql.NullString
	VersionBalance        sql.NullInt64
	VersionBalanceAfter   sql.NullInt64
}

// assignOperationBalanceFields assigns nullable balance-related fields to the operation model.
func assignOperationBalanceFields(op *operation.OperationPostgreSQLModel, opRow *operationNullableRow) {
	if opRow.Amount.Valid {
		op.Amount = &opRow.Amount.Decimal
	}

	if opRow.AvailableBalance.Valid {
		op.AvailableBalance = &opRow.AvailableBalance.Decimal
	}

	if opRow.OnHoldBalance.Valid {
		op.OnHoldBalance = &opRow.OnHoldBalance.Decimal
	}

	if opRow.AvailableBalanceAfter.Valid {
		op.AvailableBalanceAfter = &opRow.AvailableBalanceAfter.Decimal
	}

	if opRow.OnHoldBalanceAfter.Valid {
		op.OnHoldBalanceAfter = &opRow.OnHoldBalanceAfter.Decimal
	}

	if opRow.VersionBalance.Valid {
		op.VersionBalance = &opRow.VersionBalance.Int64
	}

	if opRow.VersionBalanceAfter.Valid {
		op.VersionBalanceAfter = &opRow.VersionBalanceAfter.Int64
	}
}

// assignOperationMetadataFields assigns nullable metadata fields to the operation model.
func assignOperationMetadataFields(op *operation.OperationPostgreSQLModel, opRow *operationNullableRow) {
	if opRow.StatusDescription.Valid {
		op.StatusDescription = &opRow.StatusDescription.String
	}

	if opRow.Route.Valid {
		op.Route = &opRow.Route.String
	}

	if opRow.BalanceAffected.Valid {
		op.BalanceAffected = opRow.BalanceAffected.Bool
	}

	if opRow.CreatedAt.Valid {
		op.CreatedAt = opRow.CreatedAt.Time
	}

	if opRow.UpdatedAt.Valid {
		op.UpdatedAt = opRow.UpdatedAt.Time
	}
}

// buildOperationFromRow constructs an OperationPostgreSQLModel from nullable row data.
func buildOperationFromRow(opRow *operationNullableRow) *operation.OperationPostgreSQLModel {
	op := &operation.OperationPostgreSQLModel{
		ID:              opRow.ID.String,
		TransactionID:   opRow.TransactionID.String,
		Description:     opRow.Description.String,
		Type:            opRow.Type.String,
		AssetCode:       opRow.AssetCode.String,
		Status:          opRow.Status.String,
		AccountID:       opRow.AccountID.String,
		AccountAlias:    opRow.AccountAlias.String,
		BalanceID:       opRow.BalanceID.String,
		BalanceKey:      opRow.BalanceKey.String,
		ChartOfAccounts: opRow.ChartOfAccounts.String,
		OrganizationID:  opRow.OrganizationID.String,
		LedgerID:        opRow.LedgerID.String,
		DeletedAt:       opRow.DeletedAt,
	}

	assignOperationBalanceFields(op, opRow)
	assignOperationMetadataFields(op, opRow)

	return op
}

// scanTransactionWithOperationRow scans a transaction with operation join row.
func scanTransactionWithOperationRow(rows *sql.Rows) (*TransactionPostgreSQLModel, *operation.OperationPostgreSQLModel, *string, error) {
	tran := &TransactionPostgreSQLModel{}
	opRow := &operationNullableRow{}

	var body *string

	err := rows.Scan(
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
		&opRow.ID,
		&opRow.TransactionID,
		&opRow.Description,
		&opRow.Type,
		&opRow.AssetCode,
		&opRow.Amount,
		&opRow.AvailableBalance,
		&opRow.OnHoldBalance,
		&opRow.AvailableBalanceAfter,
		&opRow.OnHoldBalanceAfter,
		&opRow.Status,
		&opRow.StatusDescription,
		&opRow.AccountID,
		&opRow.AccountAlias,
		&opRow.BalanceID,
		&opRow.ChartOfAccounts,
		&opRow.OrganizationID,
		&opRow.LedgerID,
		&opRow.CreatedAt,
		&opRow.UpdatedAt,
		&opRow.DeletedAt,
		&opRow.Route,
		&opRow.BalanceAffected,
		&opRow.BalanceKey,
		&opRow.VersionBalance,
		&opRow.VersionBalanceAfter,
	)
	if err != nil {
		return nil, nil, nil, pkg.ValidateInternalError(err, "Transaction")
	}

	if !opRow.ID.Valid {
		return tran, nil, body, nil
	}

	return tran, buildOperationFromRow(opRow), body, nil
}

// unmarshalTransactionBody unmarshals transaction body JSON.
func unmarshalTransactionBody(body *string, tran *TransactionPostgreSQLModel) error {
	if libCommons.IsNilOrEmpty(body) {
		return nil
	}

	if err := json.Unmarshal([]byte(*body), &tran.Body); err != nil {
		return pkg.ValidateInternalError(err, "Transaction")
	}

	return nil
}

// groupTransactionsByID groups transactions with their operations.
func groupTransactionsByID(rows *sql.Rows) (map[uuid.UUID]*Transaction, []uuid.UUID, error) {
	transactionsMap := make(map[uuid.UUID]*Transaction)
	transactionOrder := make([]uuid.UUID, 0)

	for rows.Next() {
		tran, op, body, err := scanTransactionWithOperationRow(rows)
		if err != nil {
			return nil, nil, err
		}

		if err := unmarshalTransactionBody(body, tran); err != nil {
			return nil, nil, err
		}

		assert.That(assert.ValidUUID(tran.ID),
			"transaction ID from database must be valid UUID",
			"id", tran.ID)

		transactionUUID := uuid.MustParse(tran.ID)

		t, exists := transactionsMap[transactionUUID]
		if !exists {
			t = tran.ToEntity()
			transactionsMap[transactionUUID] = t
			transactionOrder = append(transactionOrder, transactionUUID)
		}

		if op != nil {
			t.Operations = append(t.Operations, op.ToEntity())
		}
	}

	if err := rows.Err(); err != nil {
		return nil, nil, pkg.ValidateInternalError(err, "Transaction")
	}

	return transactionsMap, transactionOrder, nil
}

// buildTransactionWithOperationsQuery constructs the query for fetching transactions with operations
func buildTransactionWithOperationsQuery(r *TransactionPostgreSQLRepository, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination, decodedCursor libHTTP.Cursor, orderDirection string) (string, []any, string, error) {
	subQuery := squirrel.Select("*").
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

	subQuery, orderDirection = libHTTP.ApplyCursorPagination(subQuery, decodedCursor, orderDirection, filter.Limit)

	operationColumnListPrefixed := []string{
		"o.id", "o.transaction_id", "o.description", "o.type", "o.asset_code",
		"o.amount", "o.available_balance", "o.on_hold_balance", "o.available_balance_after",
		"o.on_hold_balance_after", "o.status", "o.status_description", "o.account_id",
		"o.account_alias", "o.balance_id", "o.chart_of_accounts", "o.organization_id",
		"o.ledger_id", "o.created_at", "o.updated_at", "o.deleted_at", "o.route",
		"o.balance_affected", "o.balance_key", "o.balance_version_before", "o.balance_version_after",
	}

	selectColumns := slices.Concat(transactionColumnListPrefixed, operationColumnListPrefixed)

	findAll := squirrel.
		Select(selectColumns...).
		FromSelect(subQuery, "t").
		LeftJoin("operation o ON t.id = o.transaction_id").
		PlaceholderFormat(squirrel.Dollar).
		OrderBy("t.id " + orderDirection)

	query, args, err := findAll.ToSql()
	if err != nil {
		return "", nil, "", pkg.ValidateInternalError(err, "Transaction")
	}

	return query, args, orderDirection, nil
}

// orderedTransactionsFromMap converts the transaction map to an ordered slice
func orderedTransactionsFromMap(transactionsMap map[uuid.UUID]*Transaction, transactionOrder []uuid.UUID) []*Transaction {
	transactions := make([]*Transaction, 0, len(transactionOrder))
	for _, transactionUUID := range transactionOrder {
		transactions = append(transactions, transactionsMap[transactionUUID])
	}

	return transactions
}

// executeAndGroupTransactions executes the query and groups transactions by ID
func (r *TransactionPostgreSQLRepository) executeAndGroupTransactions(ctx context.Context, db queryContextExecutor, query string, args []any) (map[uuid.UUID]*Transaction, []uuid.UUID, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)
		logger.Errorf("Failed to execute query: %v", err)
		spanQuery.End()

		return nil, nil, pkg.ValidateInternalError(err, "Transaction")
	}
	defer rows.Close()

	spanQuery.End()

	transactionsMap, transactionOrder, err := groupTransactionsByID(rows)
	if err != nil {
		logger.Errorf("Failed to group transactions: %v", err)
		return nil, nil, err
	}

	return transactionsMap, transactionOrder, nil
}

// FindOrListAllWithOperations retrieves a list of transactions from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) FindOrListAllWithOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_or_list_all_with_operations")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "Transaction")
	}

	decodedCursor, orderDirection, err := decodeCursorOrDefault(filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)
		logger.Errorf("Failed to decode cursor: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, orderDirection, err := buildTransactionWithOperationsQuery(r, organizationID, ledgerID, ids, filter, decodedCursor, orderDirection)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)
		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	transactionsMap, transactionOrder, err := r.executeAndGroupTransactions(ctx, db, query, args)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	transactions := orderedTransactionsFromMap(transactionsMap, transactionOrder)
	transactions = r.paginateTransactions(transactions, filter, decodedCursor, orderDirection)

	cur, err := r.calculateTransactionCursor(ctx, transactions, filter, decodedCursor)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	return transactions, cur, nil
}
