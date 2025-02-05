package balance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/google/uuid"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Repository provides an interface for operations related to balance template entities.
//
//go:generate mockgen --destination=balance.mock.go --package=balance . Repository
type Repository interface {
	Create(ctx context.Context, balance *Balance) error
	Update(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses) error
}

// BalancePostgreSQLRepository is a Postgresql-specific implementation of the BalanceRepository.
type BalancePostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewBalancePostgreSQLRepository returns a new instance of BalancePostgreSQLRepository using the given Postgres connection.
func NewBalancePostgreSQLRepository(pc *mpostgres.PostgresConnection) *BalancePostgreSQLRepository {
	c := &BalancePostgreSQLRepository{
		connection: pc,
		tableName:  "balance",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

func (r *BalancePostgreSQLRepository) Create(ctx context.Context, balance *Balance) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balances")
	defer span.End()
	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
	}

	record := &BalancePostgreSQLModel{}
	record.FromEntity(balance)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "balance_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert balance record from entity to JSON string", err)

		return err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO balance VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *`,
		record.ID,
		record.OrganizationID,
		record.LedgerID,
		record.AccountID,
		record.Alias,
		record.AssetCode,
		record.Available,
		record.OnHold,
		record.Scale,
		record.Version,
		record.AccountType,
		record.AllowSending,
		record.AllowReceiving,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Balance{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create balance. Rows affected is 0", err)

		return err
	}

	return nil
}

func (r *BalancePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_balances")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	tx, err := db.Begin()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to init balances", err)

		return err
	}

	var wg sync.WaitGroup

	errChan := make(chan error, len(validate.Aliases))

	for _, alias := range validate.Aliases {
		wg.Add(1)

		go func() {
			defer wg.Done()

			query := "SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND alias = $3 AND deleted_at IS NULL FOR UPDATE"

			row := tx.QueryRowContext(ctx, query, organizationID, ledgerID, alias)

			var model BalancePostgreSQLModel
			err = row.Scan(
				&model.ID,
				&model.Alias,
				&model.LedgerID,
				&model.OrganizationID,
				&model.AssetCode,
				&model.Available,
				&model.OnHold,
				&model.Scale,
				&model.Version,
				&model.CreatedAt,
				&model.UpdatedAt,
				&model.DeletedAt,
			)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					errChan <- fmt.Errorf("registro não encontrado para alias %s", alias)
					return
				}
				errChan <- fmt.Errorf("erro no select for update: %w", err)
				return
			}

			var newModel Balance

			var updates []string

			var args []any

			updates = append(updates, "available = $"+strconv.Itoa(len(args)+1))
			args = append(args, newModel.Available)

			updates = append(updates, "on_hold = $"+strconv.Itoa(len(args)+1))
			args = append(args, newModel.OnHold)

			updates = append(updates, "scale = $"+strconv.Itoa(len(args)+1))
			args = append(args, newModel.Scale)

			updates = append(updates, "version = $"+strconv.Itoa(len(args)+1))
			version := newModel.Version + 1
			args = append(args, version)

			updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
			args = append(args, time.Now(), organizationID, ledgerID, alias, newModel.Version)

			queryUpdate := `UPDATE account SET ` + strings.Join(updates, ", ") +
				` WHERE organization_id = $` + strconv.Itoa(len(args)-3) +
				` AND ledger_id = $` + strconv.Itoa(len(args)-2) +
				` AND alias = $` + strconv.Itoa(len(args)-1) +
				` AND version = $` + strconv.Itoa(len(args)) +
				` AND deleted_at IS NULL`

			result, err := tx.ExecContext(ctx, queryUpdate, args...)
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

		}()
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

		mopentelemetry.HandleSpanError(&span, "Failed to commit balances", err)

		return commitErr
	}

	return nil
}
