package transaction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	tenantmanager "github.com/LerianStudio/lib-commons/v2/commons/tenant-manager"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_transaction")
	defer span.End()

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
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
			libOpentelemetry.HandleSpanEvent(&spanExec, "Transaction already exists, skipping duplicate insert (idempotent retry)")

			logger.Infof("Transaction already exists, skipping duplicate insert (idempotent retry)")

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction. Rows affected is 0", err)

		logger.Warnf("Failed to create transaction. Rows affected is 0: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindAll retrieves Transactions entities from the database.
func (r *TransactionPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_transactions")
	defer span.End()

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	transactions := make([]*Transaction, 0)

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

	findAll := squirrel.Select(transactionColumnList...).
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

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

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

			logger.Errorf("Failed to scan row: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &transaction.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				logger.Errorf("Failed to unmarshal body: %v", err)

				return nil, libHTTP.CursorPagination{}, err
			}
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(transactions) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	transactions = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(transactions) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions[0].ID, transactions[len(transactions)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

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

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
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

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

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

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &transaction.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				logger.Errorf("Failed to unmarshal body: %v", err)

				return nil, err
			}
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, err
	}

	return transactions, nil
}

// Find retrieves a Transaction entity from the database using the provided ID.
func (r *TransactionPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction")
	defer span.End()

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
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

		return nil, err
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

		return nil, err
	}

	if !libCommons.IsNilOrEmpty(body) {
		err = json.Unmarshal([]byte(*body), &transaction.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

			logger.Errorf("Failed to unmarshal body: %v", err)

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

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
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

		return nil, err
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

		return nil, err
	}

	if !libCommons.IsNilOrEmpty(body) {
		err = json.Unmarshal([]byte(*body), &transaction.Body)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

			logger.Errorf("Failed to unmarshal body: %v", err)

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

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

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
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
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

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, "UPDATE transaction SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)
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

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
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
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

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
			&op.BalanceAffected,
			&op.BalanceKey,
			&op.VersionBalance,
			&op.VersionBalanceAfter,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

			logger.Errorf("Failed to scan rows: %v", err)

			return nil, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &tran.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				logger.Errorf("Failed to unmarshal body: %v", err)

				return nil, err
			}
		}

		newTransaction = tran.ToEntity()
		operations = append(operations, op.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, err
	}

	newTransaction.Operations = operations

	return newTransaction, nil
}

// FindOrListAllWithOperations retrieves a list of transactions from the database using the provided IDs.
//
//nolint:gocyclo // Complexity due to LEFT JOIN NULL handling for transactions without operations
func (r *TransactionPostgreSQLRepository) FindOrListAllWithOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_or_list_all_with_operations")
	defer span.End()

	db, err := tenantmanager.GetTransactionPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

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

	subQuery, orderDirection = libHTTP.ApplyCursorPagination(subQuery, decodedCursor, orderDirection, filter.Limit)

	operationColumnListPrefixed := []string{
		"o.id", "o.transaction_id", "o.description", "o.type", "o.asset_code",
		"o.amount", "o.available_balance", "o.on_hold_balance", "o.available_balance_after",
		"o.on_hold_balance_after", "o.status", "o.status_description", "o.account_id",
		"o.account_alias", "o.balance_id", "o.chart_of_accounts", "o.organization_id",
		"o.ledger_id", "o.created_at", "o.updated_at", "o.deleted_at", "o.route",
		"o.balance_affected", "o.balance_key", "o.balance_version_before", "o.balance_version_after",
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
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Debugf("FindOrListAllWithOperations query: %s with args: %v", query, args)

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

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
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

			logger.Errorf("Failed to scan rows: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		if !libCommons.IsNilOrEmpty(body) {
			err = json.Unmarshal([]byte(*body), &tran.Body)
			if err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal body", err)

				logger.Errorf("Failed to unmarshal body: %v", err)

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
			}

			t.Operations = append(t.Operations, op.ToEntity())
		}
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		logger.Errorf("Failed to get rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	for _, transactionUUID := range transactionOrder {
		transactions = append(transactions, transactionsMap[transactionUUID])
	}

	hasPagination := len(transactions) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	transactions = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(transactions) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, transactions[0].ID, transactions[len(transactions)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return transactions, cur, nil
}
