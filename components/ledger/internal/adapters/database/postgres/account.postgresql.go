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
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// AccountPostgreSQLRepository is a Postgresql-specific implementation of the AccountRepository.
type AccountPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
}

// NewAccountPostgreSQLRepository returns a new instance of AccountPostgreSQLRepository using the given Postgres connection.
func NewAccountPostgreSQLRepository(pc *mpostgres.PostgresConnection) *AccountPostgreSQLRepository {
	c := &AccountPostgreSQLRepository{
		connection: pc,
	}

	_, err := c.connection.GetDB(context.Background())
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new account entity into Postgresql and returns it.
func (r *AccountPostgreSQLRepository) Create(ctx context.Context, account *a.Account) (*a.Account, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	record := &a.AccountPostgreSQLModel{}
	record.FromEntity(account)

	result, err := db.ExecContext(ctx, `INSERT INTO account VALUES 
        (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
        )
		RETURNING *`,
		record.ID,
		record.Name,
		record.ParentAccountID,
		record.EntityID,
		record.InstrumentCode,
		record.OrganizationID,
		record.LedgerID,
		record.PortfolioID,
		record.ProductID,
		record.AvailableBalance,
		record.OnHoldBalance,
		record.BalanceScale,
		record.Status,
		record.StatusDescription,
		record.AllowSending,
		record.AllowReceiving,
		record.Alias,
		record.Type,
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
			EntityType: reflect.TypeOf(a.Account{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// FindAll retrieves an Account entities from the database.
func (r *AccountPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID) ([]*a.Account, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	var accounts []*a.Account

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND portfolio_id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, portfolioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var acc a.AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.InstrumentCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.ProductID,
			&acc.AvailableBalance,
			&acc.OnHoldBalance,
			&acc.BalanceScale,
			&acc.Status,
			&acc.StatusDescription,
			&acc.AllowSending,
			&acc.AllowReceiving,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
		); err != nil {
			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

// Find retrieves an Account entity from the database using the provided ID.
func (r *AccountPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID) (*a.Account, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	account := &a.AccountPostgreSQLModel{}

	row := db.QueryRowContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND portfolio_id = $3 AND id = $4 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, portfolioID, id)
	if err := row.Scan(
		&account.ID,
		&account.Name,
		&account.ParentAccountID,
		&account.EntityID,
		&account.InstrumentCode,
		&account.OrganizationID,
		&account.LedgerID,
		&account.PortfolioID,
		&account.ProductID,
		&account.AvailableBalance,
		&account.OnHoldBalance,
		&account.BalanceScale,
		&account.Status,
		&account.StatusDescription,
		&account.AllowSending,
		&account.AllowReceiving,
		&account.Alias,
		&account.Type,
		&account.CreatedAt,
		&account.UpdatedAt,
		&account.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(a.Account{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
		}

		return nil, err
	}

	return account.ToEntity(), nil
}

// ListByIDs retrieves Accounts entities from the database using the provided IDs.
func (r *AccountPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, ids []uuid.UUID) ([]*a.Account, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	var accounts []*a.Account

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND portfolio_id = $3 AND id = ANY($4) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, portfolioID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var acc a.AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.InstrumentCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.ProductID,
			&acc.AvailableBalance,
			&acc.OnHoldBalance,
			&acc.BalanceScale,
			&acc.Status,
			&acc.StatusDescription,
			&acc.AllowSending,
			&acc.AllowReceiving,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
		); err != nil {
			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

// Update an Account entity into Postgresql and returns the Account updated.
func (r *AccountPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID, account *a.Account) (*a.Account, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	record := &a.AccountPostgreSQLModel{}
	record.FromEntity(account)

	var updates []string

	var args []any

	if account.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if !account.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	if account.Alias != "" {
		updates = append(updates, "alias = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Alias)
	}

	if account.AllowSending != record.AllowSending {
		updates = append(updates, "allow_sending = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.AllowSending)
	}

	if account.AllowReceiving != record.AllowReceiving {
		updates = append(updates, "allow_receiving = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.AllowReceiving)
	}

	if account.ProductID != "" {
		updates = append(updates, "product_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.ProductID)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, portfolioID, id)

	query := `UPDATE account SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-3) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-2) +
		` AND portfolio_id = $` + strconv.Itoa(len(args)-1) +
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
			EntityType: reflect.TypeOf(a.Account{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// Delete removes an Account entity from the database using the provided IDs.
func (r *AccountPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, portfolioID, id uuid.UUID) error {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, `UPDATE account SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND portfolio_id = $3 AND id = $4 AND deleted_at IS NULL`,
		organizationID, ledgerID, portfolioID, id); err != nil {
		return common.EntityNotFoundError{
			EntityType: reflect.TypeOf(a.Account{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return nil
}
