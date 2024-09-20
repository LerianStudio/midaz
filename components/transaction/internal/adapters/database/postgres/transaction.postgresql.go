package postgres

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// TransactionPostgreSQLRepository is a Postgresql-specific implementation of the TransactionRepository.
type TransactionPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewTransactionPostgreSQLRepository returns a new instance of TransactionPostgreSQLRepository using the given Postgres connection.
func NewTransactionPostgreSQLRepository(pc *mpostgres.PostgresConnection) *TransactionPostgreSQLRepository {
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
func (r *TransactionPostgreSQLRepository) Create(ctx context.Context, transaction *t.Transaction) (*t.Transaction, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &t.TransactionPostgreSQLModel{}
	record.FromEntity(transaction)

	result, err := db.ExecContext(ctx, `INSERT INTO transaction VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING *`,
		record.ID,
		record.ParentTransactionID,
		record.Description,
		record.Template,
		record.Status,
		record.Amount,
		record.AmountScale,
		record.AssetCode,
		record.ChartOfAccountsGroupName,
		record.LedgerID,
		record.OrganizationID,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(t.Transaction{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// FindAll retrieves Transactions entities from the database.
func (r *TransactionPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*t.Transaction, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var transactions []*t.Transaction

	findAll := sqrl.Select("*").
		From(r.tableName).
		Where(sqrl.Expr("organization_id = ?", organizationID)).
		Where(sqrl.Expr("ledger_id = ?", ledgerID)).
		Where(sqrl.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		Limit(common.SafeIntToUint64(limit)).
		Offset(common.SafeIntToUint64((page - 1) * limit)).
		PlaceholderFormat(sqrl.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var transaction t.TransactionPostgreSQLModel
		if err := rows.Scan(
			&transaction.ID,
			&transaction.ParentTransactionID,
			&transaction.Description,
			&transaction.Template,
			&transaction.Status,
			&transaction.Amount,
			&transaction.AmountScale,
			&transaction.AssetCode,
			&transaction.ChartOfAccountsGroupName,
			&transaction.LedgerID,
			&transaction.OrganizationID,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
			&transaction.DeletedAt,
		); err != nil {
			return nil, err
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return transactions, nil
}

// ListByIDs retrieves Transaction entities from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*t.Transaction, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var transactions []*t.Transaction

	rows, err := db.QueryContext(ctx, "SELECT * FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var transaction t.TransactionPostgreSQLModel
		if err := rows.Scan(
			&transaction.ID,
			&transaction.ParentTransactionID,
			&transaction.Description,
			&transaction.Template,
			&transaction.Status,
			&transaction.Amount,
			&transaction.AmountScale,
			&transaction.AssetCode,
			&transaction.ChartOfAccountsGroupName,
			&transaction.LedgerID,
			&transaction.OrganizationID,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
			&transaction.DeletedAt,
		); err != nil {
			return nil, err
		}

		transactions = append(transactions, transaction.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return transactions, nil
}

// Find retrieves a Transaction entity from the database using the provided ID.
func (r *TransactionPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*t.Transaction, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	transaction := &t.TransactionPostgreSQLModel{}

	row := db.QueryRowContext(ctx, "SELECT * FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)
	if err := row.Scan(
		&transaction.ID,
		&transaction.ParentTransactionID,
		&transaction.Description,
		&transaction.Template,
		&transaction.Status,
		&transaction.Amount,
		&transaction.AmountScale,
		&transaction.AssetCode,
		&transaction.ChartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&transaction.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(t.Transaction{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
		}

		return nil, err
	}

	return transaction.ToEntity(), nil
}

// Update a Transaction entity into Postgresql and returns the Transaction updated.
func (r *TransactionPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *t.Transaction) (*t.Transaction, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &t.TransactionPostgreSQLModel{}
	record.FromEntity(transaction)

	var updates []string

	var args []any

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE transaction SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(t.Transaction{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// Delete removes a Transaction entity from the database using the provided IDs.
func (r *TransactionPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	db, err := r.connection.GetDB()
	if err != nil {
		return err
	}

	result, err := db.ExecContext(ctx, `UPDATE transaction SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return common.EntityNotFoundError{
			EntityType: reflect.TypeOf(t.Transaction{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return nil
}
