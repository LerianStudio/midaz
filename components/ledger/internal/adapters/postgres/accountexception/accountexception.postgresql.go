// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountexception

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// accountExceptionColumns is the canonical column order shared by every read
// path and by the RETURNING clause on Create/Update, so scanAccountException
// can be reused without duplicating the twelve-field Scan call site.
var accountExceptionColumns = []string{
	"id",
	"organization_id",
	"ledger_id",
	"account_id",
	"operational_type_codes",
	"balance_key",
	"context",
	"effective_at",
	"expires_at",
	"created_at",
	"updated_at",
	"deleted_at",
}

// accountExceptionInsertColumns is the subset of columns written by Create.
// deleted_at is server-defaulted to NULL, so leaving it out of the INSERT keeps
// the squirrel Values() call aligned with the fields Create actually populates.
var accountExceptionInsertColumns = []string{
	"id",
	"organization_id",
	"ledger_id",
	"account_id",
	"operational_type_codes",
	"balance_key",
	"context",
	"effective_at",
	"expires_at",
	"created_at",
	"updated_at",
}

// accountExceptionReturningColumns is the RETURNING expression used by Create
// and Update. It is derived from accountExceptionColumns rather than written
// out again, so the RETURNING clause and the SELECT projection cannot drift
// apart from the order scanAccountException expects.
var accountExceptionReturningColumns = strings.Join(accountExceptionColumns, ", ")

// scanAccountException reads one row into the given model using the canonical
// column order from accountExceptionColumns.
func scanAccountException(row interface{ Scan(...any) error }, m *AccountExceptionPostgreSQLModel) error {
	return row.Scan(
		&m.ID,
		&m.OrganizationID,
		&m.LedgerID,
		&m.AccountID,
		&m.OperationalTypeCodes,
		&m.BalanceKey,
		&m.Context,
		&m.EffectiveAt,
		&m.ExpiresAt,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.DeletedAt,
	)
}

// Repository provides the persistence contract for account exceptions.
//
// Every method is scoped by organization, ledger AND account: an exception is a
// rule that lives under one account, so the account is part of the identity of
// the row, not a filter. Soft-deleted rows are invisible to every read.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=accountexception.postgresql_mock.go --package=accountexception . Repository
type Repository interface {
	// Create persists a new account exception in the given organization and ledger.
	// The owning account is taken from the entity, which the service layer has
	// already validated against the scope.
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, exception *mmodel.AccountException) (*mmodel.AccountException, error)
	// FindByID returns one live account exception by scoped ID, or
	// ErrAccountExceptionNotFound (0503) when no such row exists.
	FindByID(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*mmodel.AccountException, error)
	// FindAllByAccountID returns a page of live account exceptions for one account,
	// newest first, using page-based (limit/offset) pagination.
	FindAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.AccountException, error)
	// ListByAccountID returns every live account exception for one account, oldest
	// first and unpaginated. This is the enrichment loader: the ascending order is
	// the matching order, so the first rule that matches wins deterministically.
	ListByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.AccountException, error)
	// Update applies partial changes to a live account exception by scoped ID.
	//
	// A field left at its zero value is left untouched, with one deliberate
	// exception: a non-nil BalanceKey pointing at the empty string CLEARS the
	// column to NULL, widening the rule back to every balance. That is the only
	// encoding of a *string that can express all three intents — unchanged (nil),
	// set (non-empty) and cleared ("") — and it matches the clear sentinel
	// documented on mmodel.UpdateAccountExceptionInput.
	Update(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID, exception *mmodel.AccountException) (*mmodel.AccountException, error)
	// Delete soft-deletes a live account exception by scoped ID.
	Delete(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) error
}

// Nothing wires this repository into the service layer yet, so the compiler has
// no other reason to check the concrete type against the port. The assertion
// keeps a signature drift between the two a build error instead of a surprise
// at the moment the use cases are added.
var _ Repository = (*AccountExceptionPostgreSQLRepository)(nil)

// AccountExceptionPostgreSQLRepository is a PostgreSQL implementation of the Repository.
type AccountExceptionPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewAccountExceptionPostgreSQLRepository creates a new instance of AccountExceptionPostgreSQLRepository.
func NewAccountExceptionPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *AccountExceptionPostgreSQLRepository {
	c := &AccountExceptionPostgreSQLRepository{
		connection: pc,
		tableName:  "account_exception",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
//
// account_exception lives in the ONBOARDING database next to account and
// account_type, so the module-specific lookup asks for ModuleOnboarding.
func (r *AccountExceptionPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	// Module-specific connection (from middleware WithModule)
	if db := tmcore.GetPGContext(ctx, constant.ModuleOnboarding); db != nil {
		return db, nil
	}

	// Generic connection fallback (single-module services)
	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant {
		return nil, fmt.Errorf("tenant postgres connection missing from context")
	}

	if r.connection == nil {
		return nil, fmt.Errorf("postgres connection not configured")
	}

	return r.connection.Resolver(ctx)
}

// Create persists a new account exception and returns the stored row.
func (r *AccountExceptionPostgreSQLRepository) Create(ctx context.Context, organizationID, ledgerID uuid.UUID, exception *mmodel.AccountException) (*mmodel.AccountException, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	accountExceptionID := ""
	accountID := ""

	if exception != nil {
		accountExceptionID = exception.ID
		accountID = exception.AccountID
	}

	ctx, span := tracer.Start(ctx, "postgres.create_account_exception")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID),
		attribute.String("app.request.account_exception_id", accountExceptionID),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before creating account exception", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountExceptionPostgreSQLModel{}
	record.FromEntity(exception)

	query, args, err := squirrel.Insert(r.tableName).
		Columns(accountExceptionInsertColumns...).
		Values(
			record.ID,
			record.OrganizationID,
			record.LedgerID,
			record.AccountID,
			record.OperationalTypeCodes,
			record.BalanceKey,
			record.Context,
			record.EffectiveAt,
			record.ExpiresAt,
			record.CreatedAt,
			record.UpdatedAt,
		).
		Suffix("RETURNING " + accountExceptionReturningColumns).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build create query", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built create account exception query", libLog.String("query", query))

	_, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	inserted := &AccountExceptionPostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := scanAccountException(row, inserted); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityAccountException)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute create query", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute create query", err)

		return nil, err
	}

	return inserted.ToEntity(), nil
}

// FindByID returns one live account exception scoped by organization, ledger and account.
func (r *AccountExceptionPostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*mmodel.AccountException, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_exception")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
		attribute.String("app.request.account_exception_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before finding account exception", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	query, args, err := squirrel.Select(accountExceptionColumns...).
		From(r.tableName).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"account_id":      accountID,
			"id":              id,
			"deleted_at":      nil,
		}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build find query", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built find account exception query", libLog.String("query", query))

	_, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	found := &AccountExceptionPostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := scanAccountException(row, found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrAccountExceptionNotFound, constant.EntityAccountException)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanQuery, "Account exception not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan account exception", err)

		return nil, err
	}

	return found.ToEntity(), nil
}

// FindAllByAccountID returns one page of live account exceptions for an account,
// newest first. Pagination is page-based (limit/offset), which is the frozen
// contract for this listing — deliberately NOT the cursor pagination used by the
// operation-route listing.
func (r *AccountExceptionPostgreSQLRepository) FindAllByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.AccountException, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_account_exceptions")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
		attribute.Int("app.request.query.limit", filter.Limit),
		attribute.Int("app.request.query.page", filter.Page),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before finding account exceptions", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	// A caller that sends no (or a nonsensical) page must not produce a negative
	// OFFSET: page 1 is the first page, so anything below it clamps to it.
	page := filter.Page
	if page < 1 {
		page = 1
	}

	limit := filter.Limit
	if limit < 0 {
		limit = 0
	}

	query, args, err := squirrel.Select(accountExceptionColumns...).
		From(r.tableName).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"account_id":      accountID,
			"deleted_at":      nil,
		}).
		OrderBy("created_at DESC").
		Limit(libCommons.SafeIntToUint64(limit)).
		Offset(libCommons.SafeIntToUint64((page - 1) * limit)).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build find all query", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built find all account exceptions query", libLog.String("query", query))

	return r.queryAccountExceptions(ctx, db, query, args, "postgres.find_all.query")
}

// ListByAccountID returns every live account exception for an account, oldest
// first. This feeds the transaction-time enrichment: the ascending created_at
// order IS the matching order, so the first rule that matches an operation wins
// deterministically across replays.
func (r *AccountExceptionPostgreSQLRepository) ListByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.AccountException, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_account_exceptions_by_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before listing account exceptions", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	query, args, err := squirrel.Select(accountExceptionColumns...).
		From(r.tableName).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"account_id":      accountID,
			"deleted_at":      nil,
		}).
		OrderBy("created_at ASC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build list query", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built list account exceptions query", libLog.String("query", query))

	return r.queryAccountExceptions(ctx, db, query, args, "postgres.list_by_account.query")
}

// queryAccountExceptions runs a projection built from accountExceptionColumns
// and maps every row to its domain entity. Shared by the paginated listing and
// the unpaginated enrichment loader, which differ only in ORDER BY and LIMIT.
func (r *AccountExceptionPostgreSQLRepository) queryAccountExceptions(ctx context.Context, db dbresolver.DB, query string, args []any, spanName string) ([]*mmodel.AccountException, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, spanQuery := tracer.Start(ctx, spanName)
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	exceptions := make([]*mmodel.AccountException, 0)

	for rows.Next() {
		record := &AccountExceptionPostgreSQLModel{}

		if err := scanAccountException(rows, record); err != nil {
			libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan account exception", err)

			return nil, err
		}

		exceptions = append(exceptions, record.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to iterate rows", err)

		return nil, err
	}

	spanQuery.SetAttributes(attribute.Int("db.rows_returned", len(exceptions)))

	return exceptions, nil
}

// Update applies a partial change to a live account exception and returns the stored row.
func (r *AccountExceptionPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID, exception *mmodel.AccountException) (*mmodel.AccountException, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_account_exception")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
		attribute.String("app.request.account_exception_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before updating account exception", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountExceptionPostgreSQLModel{}
	record.FromEntity(exception)

	qb := squirrel.Update(r.tableName)

	if exception != nil {
		if len(exception.OperationalTypeCodes) > 0 {
			qb = qb.Set("operational_type_codes", record.OperationalTypeCodes)
		}

		if exception.BalanceKey != nil {
			// The empty string is the documented clear sentinel: it widens the
			// rule back to every balance, which on the column is a NULL.
			if *exception.BalanceKey == "" {
				qb = qb.Set("balance_key", nil)
			} else {
				qb = qb.Set("balance_key", record.BalanceKey)
			}
		}

		if exception.Context != "" {
			qb = qb.Set("context", record.Context)
		}

		if exception.EffectiveAt != nil {
			qb = qb.Set("effective_at", record.EffectiveAt)
		}

		if exception.ExpiresAt != nil {
			qb = qb.Set("expires_at", record.ExpiresAt)
		}
	}

	record.UpdatedAt = time.Now().UTC()

	query, args, err := qb.
		Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"account_id":      accountID,
			"id":              id,
			"deleted_at":      nil,
		}).
		Suffix("RETURNING " + accountExceptionReturningColumns).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build update query", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built update account exception query", libLog.String("query", query))

	_, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	updated := &AccountExceptionPostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := scanAccountException(row, updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to update account exception. Rows affected is 0", services.ErrDatabaseItemNotFound)

			return nil, services.ErrDatabaseItemNotFound
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityAccountException)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)

		return nil, err
	}

	return updated.ToEntity(), nil
}

// Delete soft-deletes a live account exception scoped by organization, ledger and account.
func (r *AccountExceptionPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_account_exception")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
		attribute.String("app.request.account_exception_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before deleting account exception", err)

		return err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return err
	}

	query, args, err := squirrel.Update(r.tableName).
		Set("deleted_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"account_id":      accountID,
			"id":              id,
			"deleted_at":      nil,
		}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build delete query", err)

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built delete account exception query", libLog.String("query", query))

	_, spanExec := tracer.Start(ctx, "postgres.delete.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute delete query", err)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to get rows affected", err)

		return err
	}

	spanExec.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))

	if rowsAffected == 0 {
		err := services.ErrDatabaseItemNotFound

		libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to delete account exception. Rows affected is 0", err)

		return err
	}

	return nil
}
