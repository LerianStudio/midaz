package account

import (
	"context"
	"database/sql"
	"errors"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/pkg/mpointers"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// Repository provides an interface for operations related to account entities.
//
//go:generate mockgen --destination=account.mock.go --package=account . Repository
type Repository interface {
	Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.Pagination) ([]*mmodel.Account, error)
	Find(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error)
	FindWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error)
	FindByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (bool, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error)
	ListByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias []string) ([]*mmodel.Account, error)
	Update(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error)
	Delete(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error
	ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error)
	ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error)
	UpdateAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error)
	UpdateAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, acc []*account.Account) error
}

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
func (r *AccountPostgreSQLRepository) Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountPostgreSQLModel{}
	record.FromEntity(acc)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "account_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert account record from entity to JSON string", err)

		return nil, err
	}

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
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create account", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindAll retrieves an Account entities from the database (including soft-deleted ones) with pagination.
func (r *AccountPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, filter http.Pagination) ([]*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_accounts")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID))

	if portfolioID != nil && *portfolioID != uuid.Nil {
		findAll = findAll.Where(squirrel.Expr("portfolio_id = ?", portfolioID))
	}

	findAll = findAll.OrderBy("created_at " + strings.ToUpper(filter.SortOrder)).
		Where(squirrel.GtOrEq{"created_at": pkg.NormalizeDate(filter.StartDate, mpointers.Int(-1))}).
		Where(squirrel.LtOrEq{"created_at": pkg.NormalizeDate(filter.EndDate, mpointers.Int(1))})

	if len(filter.Alias) > 0 {
		findAll = findAll.Where(squirrel.Expr("alias = ?", filter.Alias))
	}

	findAll = findAll.Limit(pkg.SafeIntToUint64(filter.Limit)).
		Offset(pkg.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
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
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

// Find retrieves an Account entity from the database using the provided ID.
func (r *AccountPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	query := "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	if portfolioID != nil && *portfolioID != uuid.Nil {
		query += " AND portfolio_id = $4"

		args = append(args, portfolioID)
	}

	query += " ORDER BY created_at DESC"

	account := &AccountPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

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
		mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	return account.ToEntity(), nil
}

// FindWithDeleted retrieves an Account entity from the database using the provided ID (including soft-deleted ones).
func (r *AccountPostgreSQLRepository) FindWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_with_deleted_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	query := "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND id = $3"
	args := []any{organizationID, ledgerID, id}

	if portfolioID != nil && *portfolioID != uuid.Nil {
		query += " AND portfolio_id = $4"

		args = append(args, portfolioID)
	}

	query += " ORDER BY created_at DESC"

	account := &AccountPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_with_deleted.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

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
		mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	return account.ToEntity(), nil
}

// FindByAlias find account from the database using Organization and Ledger id and Alias. Returns true and ErrAliasUnavailability error if the alias is already taken.
func (r *AccountPostgreSQLRepository) FindByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (bool, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_by_alias")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_alias.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND alias LIKE $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, alias)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrAliasUnavailability, reflect.TypeOf(mmodel.Account{}).Name(), alias)

		mopentelemetry.HandleSpanError(&span, "Alias is already taken", err)

		return true, err
	}

	return false, nil
}

// ListByIDs retrieves Accounts entities from the database (including soft-deleted ones) using the provided IDs.
func (r *AccountPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	query := "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3)"
	args := []any{organizationID, ledgerID, ids}

	if portfolioID != nil && *portfolioID != uuid.Nil {
		query += " AND portfolio_id = $4"

		args = append(args, portfolioID)
	}

	query += " ORDER BY created_at DESC"

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
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
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

// ListByAlias retrieves Accounts entities from the database using the provided alias.
func (r *AccountPostgreSQLRepository) ListByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias []string) ([]*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_alias")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_alias.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND portfolio_id = $3 AND alias = ANY($4) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, portfolioID, pq.Array(alias))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
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
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

// Update an Account entity into Postgresql and returns the Account updated.
func (r *AccountPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountPostgreSQLModel{}
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

	if !pkg.IsNilOrEmpty(acc.Alias) {
		updates = append(updates, "alias = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Alias)
	}

	if !pkg.IsNilOrEmpty(acc.ProductID) {
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

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "account_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert account record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update account", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete an Account entity from the database (soft delete) using the provided ID.
func (r *AccountPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_account")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	query := "UPDATE account SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	if portfolioID != nil && *portfolioID != uuid.Nil {
		query += " AND portfolio_id = $4"

		args = append(args, portfolioID)
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())
	}

	spanExec.End()

	return nil
}

// ListAccountsByIDs list Accounts entity from the database using the provided IDs.
func (r *AccountPostgreSQLRepository) ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC", organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
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
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

// ListAccountsByAlias list Accounts entity from the database using the provided alias.
func (r *AccountPostgreSQLRepository) ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_alias")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_alias.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM account WHERE organization_id = $1 AND ledger_id = $2 AND alias = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC", organizationID, ledgerID, pq.Array(aliases))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var acc AccountPostgreSQLModel
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
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

// UpdateAccountByID an update Account entity by ID only into Postgresql and returns the Account updated.
func (r *AccountPostgreSQLRepository) UpdateAccountByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_account_by_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountPostgreSQLModel{}
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

	ctx, spanExec := tracer.Start(ctx, "postgres.update_account_by_id.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "account_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert account record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Account{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update account", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

func (r *AccountPostgreSQLRepository) UpdateAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, accounts []*account.Account) error {
	tracer := pkg.NewTracerFromContext(ctx)
	ctx, span0 := tracer.Start(ctx, "postgres.update_accounts.get_db")
	defer span0.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span0, "Failed to get database connection", err)

		return err
	}

	ctx, span1 := tracer.Start(ctx, "postgres.update_accounts.begin")
	defer span1.End()

	tx, err := db.Begin()
	if err != nil {
		mopentelemetry.HandleSpanError(&span1, "Failed to init transaction", err)

		return err
	}

	for _, acc := range accounts {
		var updates []string
		var args []any

		updates = append(updates, "available_balance = $"+strconv.Itoa(len(args)+1))
		args = append(args, acc.Balance.Available)

		updates = append(updates, "on_hold_balance = $"+strconv.Itoa(len(args)+1))
		args = append(args, acc.Balance.OnHold)

		updates = append(updates, "balance_scale = $"+strconv.Itoa(len(args)+1))
		args = append(args, acc.Balance.Scale)

		updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
		args = append(args, time.Now(), organizationID, ledgerID, acc.Id)

		query := `UPDATE account SET ` + strings.Join(updates, ", ") +
			` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
			` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
			` AND id = $` + strconv.Itoa(len(args)) +
			` AND deleted_at IS NULL`

		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			mopentelemetry.HandleSpanError(&span1, "Failed to update account", err)

			return err
		}
	}

	if err := tx.Commit().Error; err != nil {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		mopentelemetry.HandleSpanError(&span1, "Failed to commit accounts", err)

		return err
	}

	return nil
}
