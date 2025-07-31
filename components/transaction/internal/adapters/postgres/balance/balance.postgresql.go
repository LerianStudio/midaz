package balance

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
)

// Repository provides an interface for operations related to balance template entities.
// It defines methods for creating, finding, listing, updating, and deleting balance templates.
type Repository interface {
	Create(ctx context.Context, balance *mmodel.Balance) error
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Balance, error)
	ListAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error)
	ListAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error)
	ListByAccountIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Balance, error)
	ListByAliases(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error)
	SelectForUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string, fromTo map[string]libTransaction.Amount) error
	BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) error
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}

// BalancePostgreSQLRepository is a Postgresql-specific implementation of the BalanceRepository.
type BalancePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewBalancePostgreSQLRepository returns a new instance of BalancePostgreSQLRepository using the given Postgres connection.
func NewBalancePostgreSQLRepository(pc *libPostgres.PostgresConnection) *BalancePostgreSQLRepository {
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

func (r *BalancePostgreSQLRepository) Create(ctx context.Context, balance *mmodel.Balance) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_balances")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", balance.OrganizationID),
		attribute.String("app.request.ledger_id", balance.LedgerID),
		attribute.String("app.request.account_id", balance.AccountID),
		attribute.String("app.request.asset_code", balance.AssetCode),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", balance)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert balance from entity to JSON string", err)
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
	}

	record := &BalancePostgreSQLModel{}
	record.FromEntity(balance)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	spanExec.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&spanExec, "app.request.repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert balance record from entity to JSON string", err)
	}

	result, err := db.ExecContext(ctx, `INSERT INTO balance VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING *`,
		record.ID,
		record.OrganizationID,
		record.LedgerID,
		record.AccountID,
		record.Alias,
		record.AssetCode,
		record.Available,
		record.OnHold,
		record.Version,
		record.AccountType,
		record.AllowSending,
		record.AllowReceiving,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to create balance. Rows affected is 0", err)

		return err
	}

	return nil
}

// ListByAccountIDs list Balances entity from the database using the provided accountIDs.
func (r *BalancePostgreSQLRepository) ListByAccountIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIds []uuid.UUID) ([]*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_ids")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	spanQuery.SetAttributes(attributes...)

	rows, err := db.QueryContext(
		ctx,
		"SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND account_id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID,
		ledgerID,
		pq.Array(accountIds),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err := rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	return balances, nil
}

// ListAll list Balances entity from the database.
func (r *BalancePostgreSQLRepository) ListAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_balances")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert pagination filter from entity to JSON string", err)
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	balances := make([]*mmodel.Balance, 0)

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
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all.query")

	spanQuery.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&spanQuery, "app.request.repository_filter", filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to convert pagination filter from entity to JSON string", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err = rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(balances) > filter.Limit

	balances = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, balances, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(balances) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, balances[0].ID, balances[len(balances)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return balances, cur, nil
}

// ListAllByAccountID list Balances entity from the database using the provided accountID.
func (r *BalancePostgreSQLRepository) ListAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_all_balances_by_account_id")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	}

	span.SetAttributes(attributes...)

	if err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", filter); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert pagination filter from entity to JSON string", err)
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	balances := make([]*mmodel.Balance, 0)

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
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_all_by_account_id.query")

	spanQuery.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&spanQuery, "app.request.repository_filter", filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to convert pagination filter from entity to JSON string", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to get operations on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err = rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(balances) > filter.Limit

	balances = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, balances, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(balances) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, balances[0].ID, balances[len(balances)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return balances, cur, nil
}

// ListByAliases list Balances entity from the database using the provided aliases.
func (r *BalancePostgreSQLRepository) ListByAliases(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balances_by_aliases")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.StringSlice("app.request.aliases", aliases),
	}

	span.SetAttributes(attributes...)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var balances []*mmodel.Balance

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_aliases.query")

	spanQuery.SetAttributes(attributes...)

	rows, err := db.QueryContext(
		ctx,
		"SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND alias = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID,
		ledgerID,
		pq.Array(aliases),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err := rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		balances = append(balances, balance.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	return balances, nil
}

// SelectForUpdate a Balance entity into Postgresql.
func (r *BalancePostgreSQLRepository) SelectForUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string, fromTo map[string]libTransaction.Amount) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	_, span := tracer.Start(ctx, "postgres.update_balances")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", fromTo)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert fromTo from entity to JSON string", err)
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to init balances", err)

		return err
	}

	defer func() {
		if err != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to init balances", rollbackErr)

				logger.Errorf("err on rollback: %v", rollbackErr)
			}
		} else {
			commitErr := tx.Commit()
			if commitErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to init balances", commitErr)

				logger.Errorf("err on commit: %v", commitErr)
			}
		}
	}()

	var balances []BalancePostgreSQLModel

	query := "SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND alias = ANY($3) AND deleted_at IS NULL FOR UPDATE"

	ctx, spanQuery := tracer.Start(ctx, "postgres.select_for_update.query")

	spanQuery.SetAttributes(attributes...)

	rows, err := tx.QueryContext(ctx, query, organizationID, ledgerID, aliases)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v - err: %v", query, err)

		return err
	}

	spanQuery.End()

	defer rows.Close()

	for rows.Next() {
		var balance BalancePostgreSQLModel
		if err := rows.Scan(
			&balance.ID,
			&balance.OrganizationID,
			&balance.LedgerID,
			&balance.AccountID,
			&balance.Alias,
			&balance.AssetCode,
			&balance.Available,
			&balance.OnHold,
			&balance.Version,
			&balance.AccountType,
			&balance.AllowSending,
			&balance.AllowReceiving,
			&balance.CreatedAt,
			&balance.UpdatedAt,
			&balance.DeletedAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				logger.Errorf("register not found")

				return err
			}

			logger.Errorf("erro no select for update: %v", err)

			return err
		}

		balances = append(balances, balance)
	}

	for _, balance := range balances {
		calculateBalances, err := libTransaction.OperateBalances(fromTo[balance.Alias],
			libTransaction.Balance{
				Available: balance.Available,
				OnHold:    balance.OnHold,
			})

		if err != nil {
			return err
		}

		var updates []string

		var args []any

		updates = append(updates, "available = $"+strconv.Itoa(len(args)+1))
		args = append(args, calculateBalances.Available)

		updates = append(updates, "on_hold = $"+strconv.Itoa(len(args)+1))
		args = append(args, calculateBalances.OnHold)

		updates = append(updates, "version = $"+strconv.Itoa(len(args)+1))
		version := balance.Version + 1
		args = append(args, version)

		updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
		args = append(args, time.Now(), organizationID, ledgerID, balance.ID)

		queryUpdate := `UPDATE balance SET ` + strings.Join(updates, ", ") +
			` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
			` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
			` AND id = $` + strconv.Itoa(len(args)) +
			` AND deleted_at IS NULL`

		result, err := tx.ExecContext(ctx, queryUpdate, args...)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Err on result exec content", err)

			logger.Errorf("Err on result exec content: %v", err)

			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil || rowsAffected == 0 {
			libOpentelemetry.HandleSpanError(&span, "Err or zero rows affected", err)

			if err == nil {
				err = sql.ErrNoRows
			}

			logger.Errorf("Err or zero rows affected: %v", err)

			return err
		}
	}

	return nil
}

// BalancesUpdate updates the balances in the database.
func (r *BalancePostgreSQLRepository) BalancesUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	_, span := tracer.Start(ctx, "postgres.update_balances")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to init balances", err)

		return err
	}

	defer func() {
		if err != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to init balances", rollbackErr)

				logger.Errorf("err on rollback: %v", rollbackErr)
			}
		} else {
			commitErr := tx.Commit()
			if commitErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to init balances", commitErr)

				logger.Errorf("err on commit: %v", commitErr)
			}
		}
	}()

	for _, balance := range balances {
		ctxBalance, spanUpdate := tracer.Start(ctx, "postgres.update_balance")

		balanceAttributes := []attribute.KeyValue{
			attribute.String("app.request.balance_id", balance.ID),
			attribute.String("app.request.balance_account_type", balance.AccountType),
			attribute.String("app.request.balance_asset_code", balance.AssetCode),
			attribute.String("app.request.balance_alias", balance.Alias),
			attribute.String("app.request.balance_available", balance.Available.String()),
			attribute.String("app.request.balance_on_hold", balance.OnHold.String()),
			attribute.Int64("app.request.balance_version", balance.Version),
			attribute.String("app.request.balance_account_id", balance.AccountID),
			attribute.String("app.request.balance_organization_id", balance.OrganizationID),
			attribute.String("app.request.balance_ledger_id", balance.LedgerID),
		}

		attributes = append(attributes, balanceAttributes...)

		spanUpdate.SetAttributes(attributes...)

		var updates []string

		var args []any

		updates = append(updates, "available = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.Available)

		updates = append(updates, "on_hold = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.OnHold)

		updates = append(updates, "version = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.Version)

		updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
		args = append(args, time.Now(), organizationID, ledgerID, balance.ID, balance.Version)

		queryUpdate := `UPDATE balance SET ` + strings.Join(updates, ", ") +
			` WHERE organization_id = $` + strconv.Itoa(len(args)-3) +
			` AND ledger_id = $` + strconv.Itoa(len(args)-2) +
			` AND id = $` + strconv.Itoa(len(args)-1) +
			` AND version < $` + strconv.Itoa(len(args)) +
			` AND deleted_at IS NULL`

		result, err := tx.ExecContext(ctxBalance, queryUpdate, args...)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdate, "Err on result exec content", err)

			logger.Errorf("Err on result exec content: %v", err)

			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdate, "Err ", err)

			logger.Errorf("Err: %v", err)

			return err
		}

		if rowsAffected == 0 {
			logger.Infof("Zero rows affected")

			continue
		}

		spanUpdate.End()
	}

	return nil
}

// Find retrieves a balance entity from the database using the provided ID.
func (r *BalancePostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_balance")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.balance_id", id.String()),
	}

	span.SetAttributes(attributes...)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	balance := &BalancePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	spanQuery.SetAttributes(attributes...)

	row := db.QueryRowContext(ctx, "SELECT * FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)

	spanQuery.End()

	if err = row.Scan(
		&balance.ID,
		&balance.OrganizationID,
		&balance.LedgerID,
		&balance.AccountID,
		&balance.Alias,
		&balance.AssetCode,
		&balance.Available,
		&balance.OnHold,
		&balance.Version,
		&balance.AccountType,
		&balance.AllowSending,
		&balance.AllowReceiving,
		&balance.CreatedAt,
		&balance.UpdatedAt,
		&balance.DeletedAt,
	); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())
		}

		return nil, err
	}

	return balance.ToEntity(), nil
}

// Delete marks a balance as deleted in the database using the ID provided
func (r *BalancePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_balance")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.balance_id", id.String()),
	}

	span.SetAttributes(attributes...)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.delete.exec")

	spanQuery.SetAttributes(attributes...)

	result, err := db.ExecContext(ctx, `
		UPDATE balance 
		SET deleted_at = NOW()
		WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to execute delete query", err)

		return err
	}

	spanQuery.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to delete balance. Rows affected is 0", err)

		return err
	}

	return nil
}

// Update updates the allow_sending and allow_receiving fields of a Balance in the database.
func (r *BalancePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_balance")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.balance_id", id.String()),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", balance)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert balance from entity to JSON string", err)
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.update.exec")
	defer spanQuery.End()

	spanQuery.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&spanQuery, "app.request.repository_input", balance)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to convert balance from entity to JSON string", err)
	}

	var updates []string

	var args []any

	if balance.AllowSending != nil {
		updates = append(updates, "allow_sending = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.AllowSending)
	}

	if balance.AllowReceiving != nil {
		updates = append(updates, "allow_receiving = $"+strconv.Itoa(len(args)+1))
		args = append(args, balance.AllowReceiving)
	}

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
	args = append(args, time.Now(), organizationID, ledgerID, id)

	queryUpdate := `UPDATE balance SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, queryUpdate, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Err on result exec content", err)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		if err == nil {
			err = sql.ErrNoRows
		}

		libOpentelemetry.HandleSpanError(&span, "Err on rows affected", err)

		return err
	}

	return nil
}
