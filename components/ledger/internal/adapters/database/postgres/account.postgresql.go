package postgres

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// AccountPostgreSQLRepository is a Postgresql-specific implementation of the AccountRepository.
type AccountPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewAccountPostgreSQLRepository returns a new instance of AccountPostgreSQLRepository using the given Postgres connection.
func NewAccountPostgreSQLRepository(pc *mpostgres.PostgresConnection) *AccountPostgreSQLRepository {
	c := &AccountPostgreSQLRepository{
		connection: pc,
		tableName:  "account",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new account entity into Postgresql and returns it.
func (r *AccountPostgreSQLRepository) Create(ctx context.Context, acc *a.Account) (*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &a.AccountPostgreSQLModel{}
	record.FromEntity(acc)

	result, err := db.ExecContext(ctx, `INSERT INTO account VALUES 
        (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
        )
		RETURNING *`,
		record.ID,
		record.Name,
		record.ParentAccountID,
		record.EntityID,
		record.AssetCode,
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(a.Account{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(a.Account{}).Name())
	}

	return record.ToEntity(), nil
}

// FindAll retrieves an Account entities from the database (including soft-deleted ones) with pagination.
func (r *AccountPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, limit, page int) ([]*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var accounts []*a.Account

	findAll := sqrl.Select("*").
		From(r.tableName).
		Where(sqrl.Expr("organization_id = ?", organizationID)).
		Where(sqrl.Expr("ledger_id = ?", ledgerID))

	if portfolioID != nil && *portfolioID != uuid.Nil {
		findAll = findAll.Where(sqrl.Expr("portfolio_id = ?", portfolioID))
	}

	findAll = findAll.OrderBy("created_at DESC").
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
		var acc a.AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.AssetCode,
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
func (r *AccountPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	query := "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	if portfolioID != nil && *portfolioID != uuid.Nil {
		query += " AND portfolio_id = $4"

		args = append(args, portfolioID)
	}

	query += " ORDER BY created_at DESC"

	account := &a.AccountPostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&account.ID,
		&account.Name,
		&account.ParentAccountID,
		&account.EntityID,
		&account.AssetCode,
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
			return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(a.Account{}).Name())
		}

		return nil, err
	}

	return account.ToEntity(), nil
}

// FindWithDeleted retrieves an Account entity from the database using the provided ID (including soft-deleted ones).
func (r *AccountPostgreSQLRepository) FindWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	query := "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND id = $3"
	args := []any{organizationID, ledgerID, id}

	if portfolioID != nil && *portfolioID != uuid.Nil {
		query += " AND portfolio_id = $4"

		args = append(args, portfolioID)
	}

	query += " ORDER BY created_at DESC"

	account := &a.AccountPostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&account.ID,
		&account.Name,
		&account.ParentAccountID,
		&account.EntityID,
		&account.AssetCode,
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
			return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(a.Account{}).Name())
		}

		return nil, err
	}

	return account.ToEntity(), nil
}

// FindByAlias find account from the database using Organization and Ledger id and Alias. Returns true and ErrAliasUnavailability error if the alias is already taken.
func (r *AccountPostgreSQLRepository) FindByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (bool, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return false, err
	}

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND alias LIKE $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, alias)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if rows.Next() {
		return true, common.ValidateBusinessError(cn.ErrAliasUnavailability, reflect.TypeOf(a.Account{}).Name(), alias)
	}

	return false, nil
}

// ListByIDs retrieves Accounts entities from the database (including soft-deleted ones) using the provided IDs.
func (r *AccountPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, ids []uuid.UUID) ([]*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var accounts []*a.Account

	query := "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3)"
	args := []any{organizationID, ledgerID, ids}

	if portfolioID != nil && *portfolioID != uuid.Nil {
		query += " AND portfolio_id = $4"

		args = append(args, portfolioID)
	}

	query += " ORDER BY created_at DESC"

	rows, err := db.QueryContext(ctx, query, args...)
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
			&acc.AssetCode,
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

// ListByAlias retrieves Accounts entities from the database using the provided alias.
func (r *AccountPostgreSQLRepository) ListByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias []string) ([]*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var accounts []*a.Account

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND portfolio_id = $3 AND alias = ANY($4) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, portfolioID, pq.Array(alias))
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
			&acc.AssetCode,
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
func (r *AccountPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, acc *a.Account) (*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &a.AccountPostgreSQLModel{}
	record.FromEntity(acc)

	var updates []string

	var args []any

	if acc.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if !acc.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)

		updates = append(updates, "allow_sending = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.AllowSending)

		updates = append(updates, "allow_receiving = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.AllowReceiving)
	}

	if !common.IsNilOrEmpty(acc.Alias) {
		updates = append(updates, "alias = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Alias)
	}

	if !common.IsNilOrEmpty(acc.ProductID) {
		updates = append(updates, "product_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.ProductID)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE account SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	if portfolioID != nil && *portfolioID != uuid.Nil {
		args = append(args, portfolioID)
		query += ` AND portfolio_id = $` + strconv.Itoa(len(args))
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(a.Account{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(a.Account{}).Name())
	}

	return record.ToEntity(), nil
}

// Delete an Account entity from the database (soft delete) using the provided ID.
func (r *AccountPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	db, err := r.connection.GetDB()
	if err != nil {
		return err
	}

	query := "UPDATE account SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	if portfolioID != nil && *portfolioID != uuid.Nil {
		query += " AND portfolio_id = $4"

		args = append(args, portfolioID)
	}

	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		return common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(a.Account{}).Name())
	}

	return nil
}

// ListAccountsByIDs list Accounts entity from the database using the provided IDs.
func (r *AccountPostgreSQLRepository) ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var accounts []*a.Account

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC", organizationID, ledgerID, pq.Array(ids))
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
			&acc.AssetCode,
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

// ListAccountsByAlias list Accounts entity from the database using the provided alias.
func (r *AccountPostgreSQLRepository) ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var accounts []*a.Account

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND alias = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC", organizationID, ledgerID, pq.Array(aliases))
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
			&acc.AssetCode,
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

// UpdateAccountByID an update Account entity by ID only into Postgresql and returns the Account updated.
func (r *AccountPostgreSQLRepository) UpdateAccountByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, acc *a.Account) (*a.Account, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &a.AccountPostgreSQLModel{}
	record.FromEntity(acc)

	var updates []string

	var args []any

	if !acc.Balance.IsEmpty() {
		updates = append(updates, "available_balance = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.AvailableBalance)

		updates = append(updates, "on_hold_balance = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.OnHoldBalance)

		updates = append(updates, "balance_scale = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.BalanceScale)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE account SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(a.Account{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(a.Account{}).Name())
	}

	return record.ToEntity(), nil
}
