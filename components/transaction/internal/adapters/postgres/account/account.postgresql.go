package account

import (
	"context"
	"database/sql"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Repository provides an interface for operations related to account entities.
//
//go:generate mockgen --destination=account.mock.go --package=account . Repository
type Repository interface {
	ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error)
	ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error)
	UpdateAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, acc []*mmodel.Account) error
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
			&acc.Version,
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
			&acc.Version,
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

// UpdateAccounts an update all Accounts entity by ID only into Postgresql.
func (r *AccountPostgreSQLRepository) UpdateAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, accounts []*mmodel.Account) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_accounts")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	tx, err := db.Begin()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to init transaction", err)

		return err
	}

	var wg sync.WaitGroup

	errChan := make(chan error, len(accounts))

	for _, acc := range accounts {
		wg.Add(1)

		go func(acc *mmodel.Account) {
			defer wg.Done()

			var updates []string

			var args []any

			updates = append(updates, "available_balance = $"+strconv.Itoa(len(args)+1))
			args = append(args, acc.Balance.Available)

			updates = append(updates, "on_hold_balance = $"+strconv.Itoa(len(args)+1))
			args = append(args, acc.Balance.OnHold)

			updates = append(updates, "balance_scale = $"+strconv.Itoa(len(args)+1))
			args = append(args, acc.Balance.Scale)

			updates = append(updates, "version = $"+strconv.Itoa(len(args)+1))
			version := acc.Version + 1
			args = append(args, version)

			updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
			args = append(args, time.Now(), organizationID, ledgerID, acc.ID, acc.Version)

			query := `UPDATE account SET ` + strings.Join(updates, ", ") +
				` WHERE organization_id = $` + strconv.Itoa(len(args)-3) +
				` AND ledger_id = $` + strconv.Itoa(len(args)-2) +
				` AND id = $` + strconv.Itoa(len(args)-1) +
				` AND version = $` + strconv.Itoa(len(args)) +
				` AND deleted_at IS NULL`

			result, err := tx.ExecContext(ctx, query, args...)
			if err != nil {
				errChan <- err
				return
			}

			rowsAffected, err := result.RowsAffected()
			if err != nil || rowsAffected == 0 {
				if err == nil {
					err = sql.ErrNoRows
				}
				errChan <- err
			}
		}(acc)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				return rollbackErr
			}

			return err
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Account{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to commit accounts", err)

		return commitErr
	}

	return nil
}
