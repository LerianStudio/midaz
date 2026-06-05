// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounttype

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libPointers "github.com/LerianStudio/lib-commons/v5/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

var accountTypeColumnList = []string{
	"id",
	"organization_id",
	"ledger_id",
	"name",
	"description",
	"key_value",
	"created_at",
	"updated_at",
	"deleted_at",
}

// Repository provides an interface for operations related to account type entities.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=accounttype.postgresql_mock.go --package=accounttype . Repository
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error)
	FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error)
	FindByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.AccountType, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.AccountType, libHTTP.CursorPagination, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.AccountType, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}

// AccountTypePostgreSQLRepository is a PostgreSQL implementation of the AccountTypeRepository.
type AccountTypePostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewAccountTypePostgreSQLRepository creates a new instance of AccountTypePostgreSQLRepository.
func NewAccountTypePostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *AccountTypePostgreSQLRepository {
	c := &AccountTypePostgreSQLRepository{
		connection: pc,
		tableName:  "account_type",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *AccountTypePostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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

// Create creates a new account type.
// It returns the created account type and an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) Create(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_account_type")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &AccountTypePostgreSQLModel{}
	record.FromEntity(accountType)

	query, args, err := squirrel.Insert(r.tableName).
		Columns(accountTypeColumnList...).
		Values(
			record.ID,
			record.OrganizationID,
			record.LedgerID,
			record.Name,
			record.Description,
			record.KeyValue,
			record.CreatedAt,
			record.UpdatedAt,
			record.DeletedAt,
		).
		Suffix("RETURNING " + strings.Join(accountTypeColumnList, ", ")).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	inserted := &AccountTypePostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&inserted.ID,
		&inserted.OrganizationID,
		&inserted.LedgerID,
		&inserted.Name,
		&inserted.Description,
		&inserted.KeyValue,
		&inserted.CreatedAt,
		&inserted.UpdatedAt,
		&inserted.DeletedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityAccountType)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute insert account type query", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to execute insert account type query", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute insert account type query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute insert account type query", libLog.Err(err))

		return nil, err
	}

	return inserted.ToEntity(), nil
}

// FindByID retrieves an account type by its ID.
// It returns the account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_type_by_id")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	var record AccountTypePostgreSQLModel

	_, spanQuery := tracer.Start(ctx, "postgres.find_by_id.query")

	row := db.QueryRowContext(ctx, `
		SELECT 
			id, 
			organization_id, 
			ledger_id, 
			name, 
			description, 
			key_value, 
			created_at, 
			updated_at, 
			deleted_at 
		FROM account_type 
		WHERE id = $1 
			AND organization_id = $2 
			AND ledger_id = $3 
			AND deleted_at IS NULL`,
		id, organizationID, ledgerID)

	err = row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Name,
		&record.Description,
		&record.KeyValue,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan account type record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, services.ErrDatabaseItemNotFound
		}

		return nil, err
	}

	spanQuery.End()

	return record.ToEntity(), nil
}

// FindByKey retrieves an account type by its key within an organization and ledger.
// It returns the account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) FindByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_type_by_key")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	var record AccountTypePostgreSQLModel

	_, spanQuery := tracer.Start(ctx, "postgres.find_by_key.query")

	row := db.QueryRowContext(ctx, `
		SELECT 
			id, 
			organization_id, 
			ledger_id, 
			name, 
			description, 
			key_value, 
			created_at, 
			updated_at, 
			deleted_at 
		FROM account_type 
		WHERE key_value = $1 
			AND organization_id = $2 
			AND ledger_id = $3 
			AND deleted_at IS NULL`,
		strings.ToLower(key), organizationID, ledgerID)

	err = row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Name,
		&record.Description,
		&record.KeyValue,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan account type record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, services.ErrDatabaseItemNotFound
		}

		return nil, err
	}

	spanQuery.End()

	return record.ToEntity(), nil
}

// Update updates an account type by its ID.
// It returns the updated account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_account_type")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &AccountTypePostgreSQLModel{}
	record.FromEntity(accountType)
	record.UpdatedAt = time.Now()

	update := squirrel.Update(r.tableName).
		Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if accountType.Name != "" {
		update = update.Set("name", record.Name)
	}

	if accountType.Description != "" {
		update = update.Set("description", record.Description)
	}

	update = update.Suffix("RETURNING " + strings.Join(accountTypeColumnList, ", "))

	query, args, err := update.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err), libLog.String("account_type_id", id.String()))

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	updated := &AccountTypePostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&updated.ID,
		&updated.OrganizationID,
		&updated.LedgerID,
		&updated.Name,
		&updated.Description,
		&updated.KeyValue,
		&updated.CreatedAt,
		&updated.UpdatedAt,
		&updated.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to update account type. Rows affected is 0", services.ErrDatabaseItemNotFound)
			logger.Log(ctx, libLog.LevelWarn, "Failed to update account type. Rows affected is 0", libLog.String("account_type_id", id.String()))

			return nil, services.ErrDatabaseItemNotFound
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityAccountType)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to execute update query", libLog.Err(err), libLog.String("account_type_id", id.String()))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute update query", libLog.Err(err), libLog.String("account_type_id", id.String()))

		return nil, err
	}

	return updated.ToEntity(), nil
}

// FindAll retrieves all account types with cursor pagination.
// It returns the account types, pagination cursor, and an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.AccountType, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_account_types")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	var accountTypes []*mmodel.AccountType

	pagination := filter.ToCursorPagination()

	decodedCursor := libHTTP.Cursor{}
	isFirstPage := libCommons.IsNilOrEmpty(&pagination.Cursor)
	orderDirection := strings.ToUpper(pagination.SortOrder)
	cursorDirection := libHTTP.CursorDirectionNext

	if !isFirstPage {
		decodedCursor, err = libHTTP.DecodeCursor(pagination.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		cursorDirection = decodedCursor.Direction
	}

	findAll := squirrel.Select(accountTypeColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(pagination.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(pagination.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	// Filter by entity IDs when provided (metadata composition)
	if len(filter.EntityIDs) > 0 {
		findAll = findAll.Where(squirrel.Expr("id = ANY(?)", pq.Array(filter.EntityIDs)))
	}

	if !libCommons.IsNilOrEmpty(filter.KeyValue) {
		findAll = findAll.Where(squirrel.Expr("key_value = ?", strings.ToLower(*filter.KeyValue)))
	}

	findAll, err = applyCursorPagination(findAll, decodedCursor, orderDirection, pagination.Limit)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to apply cursor pagination", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var record AccountTypePostgreSQLModel
		if err := rows.Scan(
			&record.ID,
			&record.OrganizationID,
			&record.LedgerID,
			&record.Name,
			&record.Description,
			&record.KeyValue,
			&record.CreatedAt,
			&record.UpdatedAt,
			&record.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan account type record", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		accountTypes = append(accountTypes, record.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(accountTypes) > pagination.Limit

	accountTypes = libHTTP.PaginateRecords(isFirstPage, hasPagination, cursorDirection, accountTypes, pagination.Limit)

	cur := libHTTP.CursorPagination{}
	if len(accountTypes) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, cursorDirection, accountTypes[0].ID.String(), accountTypes[len(accountTypes)-1].ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return accountTypes, cur, nil
}

// ListByIDs retrieves account types by their IDs.
// It returns the account types matching the provided IDs or an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_account_types_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	var accountTypes []*mmodel.AccountType

	_, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	query := `SELECT 
		id, 
		organization_id, 
		ledger_id, 
		name, 
		description, 
		key_value, 
		created_at, 
		updated_at, 
		deleted_at 
	FROM account_type 
	WHERE organization_id = $1 
		AND ledger_id = $2 
		AND id = ANY($3) 
		AND deleted_at IS NULL 
	ORDER BY created_at DESC`

	rows, err := db.QueryContext(ctx, query, organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var record AccountTypePostgreSQLModel
		if err := rows.Scan(
			&record.ID,
			&record.OrganizationID,
			&record.LedgerID,
			&record.Name,
			&record.Description,
			&record.KeyValue,
			&record.CreatedAt,
			&record.UpdatedAt,
			&record.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan account type record", err)

			return nil, err
		}

		accountTypes = append(accountTypes, record.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		return nil, err
	}

	return accountTypes, nil
}

// Delete performs a soft delete of an account type by its ID.
// It returns an error if the operation fails or if the account type is not found.
func (r *AccountTypePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_account_type")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return err
	}

	query := "UPDATE account_type SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	_, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute delete query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute delete query", libLog.Err(err))

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected", libLog.Err(err))

		return err
	}

	if rowsAffected == 0 {
		return services.ErrDatabaseItemNotFound
	}

	return nil
}
