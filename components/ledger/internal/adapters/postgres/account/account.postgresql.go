// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libPointers "github.com/LerianStudio/lib-commons/v6/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// holderColumns are the two columns migrations 000017 and 000019 add. They are
// named apart because the /v1 account contract withholds both keys, so a /v1
// request must never reference them: the schema is applied out of band, and a
// database that has not reached those migrations still has to serve /v1.
const (
	holderIDColumn           = "holder_id"
	holderCheckSkippedColumn = "holder_check_skipped"
)

// holderIDPlaceholder and holderCheckSkippedPlaceholder stand in for the holder
// columns on a projection that withholds them. They are typed constants rather
// than omissions so the projection keeps its arity, order and column types, and
// every positional Scan below stays valid for both shapes.
const (
	holderIDPlaceholder           = "NULL::uuid AS " + holderIDColumn
	holderCheckSkippedPlaceholder = "FALSE AS " + holderCheckSkippedColumn
)

// accountColumns returns the account projection. When withHolder is false the
// two holder columns are replaced by constants of the same type, so the
// statement never names them and parses against a pre-000017 schema.
func accountColumns(withHolder bool) []string {
	holderID, holderCheckSkipped := holderIDPlaceholder, holderCheckSkippedPlaceholder
	if withHolder {
		holderID, holderCheckSkipped = holderIDColumn, holderCheckSkippedColumn
	}

	return []string{
		"id",
		"name",
		"parent_account_id",
		"entity_id",
		holderID,
		"asset_code",
		"organization_id",
		"ledger_id",
		"portfolio_id",
		"segment_id",
		"status",
		"status_description",
		"alias",
		"type",
		"created_at",
		"updated_at",
		"deleted_at",
		"blocked",
		holderCheckSkipped,
	}
}

// Repository provides an interface for operations related to account entities.
// It defines methods for creating, retrieving, updating, and deleting accounts in the database.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=account.postgresql_mock.go --package=account . Repository
type Repository interface {
	Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error)
	// The reads below take a holder policy because their result can be serialized
	// to an account response: only /v2 can observe the holder keys, so a /v1 read
	// projects constants and never names the columns. The three ListAccounts*
	// reads take none — the transaction and asset paths they serve read no holder,
	// so they always project constants and never depend on those columns.
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID, segmentID *uuid.UUID, filter http.QueryHeader, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error)
	// FindAllByHolder lists the live accounts owned by holderID within the organization,
	// across every ledger; a non-nil ledgerID narrows to one ledger. Ordered by
	// created_at then id in filter.SortOrder, paged by filter.Limit/Page.
	FindAllByHolder(ctx context.Context, organizationID, holderID uuid.UUID, ledgerID *uuid.UUID, filter http.QueryHeader, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error)
	Find(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, holderPolicy mmodel.HolderPolicy) (*mmodel.Account, error)
	FindWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, holderPolicy mmodel.HolderPolicy) (*mmodel.Account, error)
	FindAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string, holderPolicy mmodel.HolderPolicy) (*mmodel.Account, error)
	FindByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (bool, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID, segmentID *uuid.UUID, ids []uuid.UUID, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error)
	ListByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias []string, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error)
	Update(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error)
	Delete(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error
	ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error)
	ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error)
	// ListExternalAccountsByAssetCode returns the live (not soft-deleted) accounts of
	// type external for the given asset code within the organization and ledger.
	ListExternalAccountsByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, assetCode string) ([]*mmodel.Account, error)
	Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error)
	// CountByHolderID returns the number of non-deleted accounts owned by the
	// holder within the organization, across all ledgers. It backs the CRM
	// holder-delete ownership guard, so it counts only active (deleted_at IS NULL)
	// accounts; soft-deleted accounts no longer pin the holder.
	CountByHolderID(ctx context.Context, organizationID, holderID uuid.UUID) (int64, error)
}

// AccountPostgreSQLRepository is a Postgresql-specific implementation of the AccountRepository.
type AccountPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewAccountPostgreSQLRepository returns a new instance of AccountPostgreSQLRepository using the given Postgres connection.
func NewAccountPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *AccountPostgreSQLRepository {
	c := &AccountPostgreSQLRepository{
		connection: pc,
		tableName:  "account",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *AccountPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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
		return nil, fmt.Errorf("postgres connection not available")
	}

	return r.connection.Resolver(ctx)
}

// Create a new account entity into Postgresql and returns it.
func (r *AccountPostgreSQLRepository) Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountPostgreSQLModel{}
	record.FromEntity(acc)

	columns := []string{
		"id",
		"name",
		"parent_account_id",
		"entity_id",
		"asset_code",
		"organization_id",
		"ledger_id",
		"portfolio_id",
		"segment_id",
		"status",
		"status_description",
		"alias",
		"type",
		"created_at",
		"updated_at",
		"deleted_at",
		"blocked",
	}

	values := []any{
		record.ID,
		record.Name,
		record.ParentAccountID,
		record.EntityID,
		record.AssetCode,
		record.OrganizationID,
		record.LedgerID,
		record.PortfolioID,
		record.SegmentID,
		record.Status,
		record.StatusDescription,
		record.Alias,
		record.Type,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
		record.Blocked,
	}

	// The holder columns are named only when they carry a value. An account
	// without one — every /v1 create, an external account, the account asset
	// creation opens — writes what the columns default to (NULL and FALSE), so
	// omitting them persists the identical row while keeping the statement
	// parseable against a schema that predates migrations 000017 and 000019.
	if record.HolderID != nil || record.HolderCheckSkipped {
		columns = append(columns, holderIDColumn, holderCheckSkippedColumn)
		values = append(values, record.HolderID, record.HolderCheckSkipped)
	}

	builder := squirrel.Insert(r.tableName).
		Columns(columns...).
		Values(values...).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.create.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			schemaDrift := services.IsSchemaDrift(err)

			err := services.ValidatePGError(pgErr, constant.EntityAccount)

			// Schema drift is an infrastructure failure, not a caller mistake, so
			// it has to flip the span red; a constraint violation stays business.
			if schemaDrift {
				libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)
			} else {
				libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute query", err)
			}

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// applyAccountListFilters applies the account list query header — the optional
// column filters and the created_at range — to a select builder. Shared by the
// ledger-scoped and holder-scoped listings so both honour the same filter set.
func applyAccountListFilters(b squirrel.SelectBuilder, filter http.QueryHeader) squirrel.SelectBuilder {
	// Filter by entity IDs when provided (metadata composition)
	if len(filter.EntityIDs) > 0 {
		b = b.Where(squirrel.Expr("id = ANY(?)", pq.Array(filter.EntityIDs)))
	}

	if !libCommons.IsNilOrEmpty(filter.Status) {
		b = b.Where(squirrel.Expr("status = ?", *filter.Status))
	}

	if !libCommons.IsNilOrEmpty(filter.Type) {
		b = b.Where(squirrel.Expr("type = ?", *filter.Type))
	}

	if !libCommons.IsNilOrEmpty(filter.AssetCode) {
		b = b.Where(squirrel.Expr("asset_code = ?", *filter.AssetCode))
	}

	if !libCommons.IsNilOrEmpty(filter.EntityID) {
		b = b.Where(squirrel.Expr("entity_id = ?", *filter.EntityID))
	}

	if !libCommons.IsNilOrEmpty(filter.HolderID) {
		b = b.Where(squirrel.Expr("holder_id = ?", *filter.HolderID))
	}

	if filter.Blocked != nil {
		b = b.Where(squirrel.Expr("blocked = ?", *filter.Blocked))
	}

	if !libCommons.IsNilOrEmpty(filter.ParentAccountID) {
		b = b.Where(squirrel.Expr("parent_account_id = ?", *filter.ParentAccountID))
	}

	if filter.Name != nil && *filter.Name != "" {
		sanitized := http.EscapeSearchMetacharacters(*filter.Name)
		b = b.Where(
			squirrel.Expr("lower(name) LIKE lower(?) || '%' ESCAPE '\\'", sanitized),
		)
	}

	if filter.Alias != nil && *filter.Alias != "" {
		sanitized := http.EscapeSearchMetacharacters(*filter.Alias)
		b = b.Where(
			squirrel.Expr("lower(alias) LIKE lower(?) || '%' ESCAPE '\\'", sanitized),
		)
	}

	if !filter.StartDate.IsZero() {
		b = b.
			Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
			Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)})
	}

	return b
}

// accountListOrderBy orders a listing by created_at with id as tiebreaker, both
// in the requested direction. The tiebreaker makes paging total: accounts written
// in the same transaction share a created_at, and without it a row can repeat on
// one page and be missing from another.
func accountListOrderBy(filter http.QueryHeader) []string {
	dir := strings.ToUpper(filter.SortOrder)

	return []string{"created_at " + dir, "id " + dir}
}

// scanAccountRows drains an account projection into domain entities. The scan
// order mirrors accountColumns.
func scanAccountRows(rows *sql.Rows) ([]*mmodel.Account, error) {
	var accounts []*mmodel.Account

	for rows.Next() {
		var acc AccountPostgreSQLModel
		if err := rows.Scan(
			&acc.ID,
			&acc.Name,
			&acc.ParentAccountID,
			&acc.EntityID,
			&acc.HolderID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
			&acc.HolderCheckSkipped,
		); err != nil {
			return nil, mapReadError(err)
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

// FindAll retrieves the live Account entities of a ledger with pagination.
func (r *AccountPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID, segmentID *uuid.UUID, filter http.QueryHeader, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_accounts")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	findAll := squirrel.Select(accountColumns(holderPolicy.ProjectsHolder())...).
		From(r.tableName).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID))

	if portfolioID != nil && *portfolioID != uuid.Nil {
		findAll = findAll.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	if segmentID != nil && *segmentID != uuid.Nil {
		findAll = findAll.Where(squirrel.Expr("segment_id = ?", *segmentID))
	}

	findAll = applyAccountListFilters(findAll, filter).
		OrderBy(accountListOrderBy(filter)...).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", mapped)

		return nil, mapped
	}
	defer rows.Close()

	spanQuery.End()

	accounts, err := scanAccountRows(rows)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read rows", err)

		return nil, err
	}

	return accounts, nil
}

// FindAllByHolder retrieves the live accounts owned by a holder across every
// ledger of the organization, narrowed to one ledger when ledgerID is non-nil.
func (r *AccountPostgreSQLRepository) FindAllByHolder(ctx context.Context, organizationID, holderID uuid.UUID, ledgerID *uuid.UUID, filter http.QueryHeader, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_accounts_by_holder")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.Bool("app.request.has_ledger_id", ledgerID != nil && *ledgerID != uuid.Nil),
	)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	findAll := squirrel.Select(accountColumns(holderPolicy.ProjectsHolder())...).
		From(r.tableName).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("holder_id = ?", holderID))

	if ledgerID != nil && *ledgerID != uuid.Nil {
		findAll = findAll.Where(squirrel.Expr("ledger_id = ?", *ledgerID))
	}

	// The path holder is authoritative; a holder_id query filter cannot widen or
	// contradict it.
	filter.HolderID = nil

	findAll = applyAccountListFilters(findAll, filter).
		OrderBy(accountListOrderBy(filter)...).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_all_accounts_by_holder.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", mapped)

		return nil, mapped
	}
	defer rows.Close()

	spanQuery.End()

	accounts, err := scanAccountRows(rows)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read rows", err)

		return nil, err
	}

	return accounts, nil
}

// Find retrieves an Account entity from the database using the provided ID.
func (r *AccountPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, holderPolicy mmodel.HolderPolicy) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	findOne := squirrel.Select(accountColumns(holderPolicy.ProjectsHolder())...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ?", id)).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		findOne = findOne.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := findOne.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	acc := &AccountPostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&acc.ID,
		&acc.Name,
		&acc.ParentAccountID,
		&acc.EntityID,
		&acc.HolderID,
		&acc.AssetCode,
		&acc.OrganizationID,
		&acc.LedgerID,
		&acc.PortfolioID,
		&acc.SegmentID,
		&acc.Status,
		&acc.StatusDescription,
		&acc.Alias,
		&acc.Type,
		&acc.CreatedAt,
		&acc.UpdatedAt,
		&acc.DeletedAt,
		&acc.Blocked,
		&acc.HolderCheckSkipped,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccount)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to scan row", err)

			return nil, err
		}

		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", mapped)

		return nil, mapped
	}

	return acc.ToEntity(), nil
}

// FindWithDeleted retrieves an Account entity from the database using the provided ID (including soft-deleted ones).
func (r *AccountPostgreSQLRepository) FindWithDeleted(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, holderPolicy mmodel.HolderPolicy) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_with_deleted_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	findOne := squirrel.Select(accountColumns(holderPolicy.ProjectsHolder())...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ?", id)).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		findOne = findOne.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := findOne.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	acc := &AccountPostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find_with_deleted.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&acc.ID,
		&acc.Name,
		&acc.ParentAccountID,
		&acc.EntityID,
		&acc.HolderID,
		&acc.AssetCode,
		&acc.OrganizationID,
		&acc.LedgerID,
		&acc.PortfolioID,
		&acc.SegmentID,
		&acc.Status,
		&acc.StatusDescription,
		&acc.Alias,
		&acc.Type,
		&acc.CreatedAt,
		&acc.UpdatedAt,
		&acc.DeletedAt,
		&acc.Blocked,
		&acc.HolderCheckSkipped,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccount)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to scan row", err)

			return nil, err
		}

		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", mapped)

		return nil, mapped
	}

	return acc.ToEntity(), nil
}

// FindAlias retrieves an Account entity from the database using the provided Alias.
func (r *AccountPostgreSQLRepository) FindAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string, holderPolicy mmodel.HolderPolicy) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_alias")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	findOne := squirrel.Select(accountColumns(holderPolicy.ProjectsHolder())...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("alias = ?", alias)).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		findOne = findOne.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := findOne.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	acc := &AccountPostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find_alias.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&acc.ID,
		&acc.Name,
		&acc.ParentAccountID,
		&acc.EntityID,
		&acc.HolderID,
		&acc.AssetCode,
		&acc.OrganizationID,
		&acc.LedgerID,
		&acc.PortfolioID,
		&acc.SegmentID,
		&acc.Status,
		&acc.StatusDescription,
		&acc.Alias,
		&acc.Type,
		&acc.CreatedAt,
		&acc.UpdatedAt,
		&acc.DeletedAt,
		&acc.Blocked,
		&acc.HolderCheckSkipped,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrAccountAliasNotFound, constant.EntityAccount)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to scan row", err)

			return nil, err
		}

		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", mapped)

		return nil, mapped
	}

	return acc.ToEntity(), nil
}

// FindByAlias find account from the database using Organization and Ledger id and Alias. Returns true and ErrAliasUnavailability error if the alias is already taken.
func (r *AccountPostgreSQLRepository) FindByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (bool, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_by_alias")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return false, err
	}

	builder := squirrel.Select("1").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("alias LIKE ?", alias)).
		Where(squirrel.Expr("deleted_at IS NULL")).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return false, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_by_alias.query")

	var exists int

	err = db.QueryRowContext(ctx, query, args...).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			spanQuery.End()
			return false, nil
		}

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		return false, err
	}

	spanQuery.End()

	err = pkg.ValidateBusinessError(constant.ErrAliasUnavailability, constant.EntityAccount, alias)

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Alias is already taken", err)

	return true, err
}

// ListByIDs retrieves Accounts entities from the database using the provided IDs.
func (r *AccountPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID, segmentID *uuid.UUID, ids []uuid.UUID, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	findAll := squirrel.Select(accountColumns(holderPolicy.ProjectsHolder())...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		findAll = findAll.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	if segmentID != nil && *segmentID != uuid.Nil {
		findAll = findAll.Where(squirrel.Expr("segment_id = ?", *segmentID))
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", mapped)

		return nil, mapped
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
			&acc.HolderID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
			&acc.HolderCheckSkipped,
		); err != nil {
			mapped := mapReadError(err)

			libOpentelemetry.HandleSpanError(span, "Failed to scan row", mapped)

			return nil, mapped
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

// ListByAlias retrieves Accounts entities from the database using the provided alias.
func (r *AccountPostgreSQLRepository) ListByAlias(ctx context.Context, organizationID, ledgerID, portfolioID uuid.UUID, alias []string, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_alias")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	findAll := squirrel.Select(accountColumns(holderPolicy.ProjectsHolder())...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("portfolio_id = ?", portfolioID)).
		Where(squirrel.Expr("alias = ANY(?)", pq.Array(alias))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.list_by_alias.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", mapped)

		return nil, mapped
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
			&acc.HolderID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
			&acc.HolderCheckSkipped,
		); err != nil {
			mapped := mapReadError(err)

			libOpentelemetry.HandleSpanError(span, "Failed to scan row", mapped)

			return nil, mapped
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

// applyNullableFields applies nullable field updates (segmentId, entityId, portfolioId)
// supporting RFC 7396 JSON Merge Patch null semantics.
func applyNullableFields(builder squirrel.UpdateBuilder, acc *mmodel.Account, record *AccountPostgreSQLModel) squirrel.UpdateBuilder {
	if !libCommons.IsNilOrEmpty(acc.SegmentID) {
		builder = builder.Set("segment_id", record.SegmentID)
	} else if slices.Contains(acc.NullFields, "segmentId") {
		builder = builder.Set("segment_id", nil)
	}

	if !libCommons.IsNilOrEmpty(acc.EntityID) {
		builder = builder.Set("entity_id", record.EntityID)
	} else if slices.Contains(acc.NullFields, "entityId") {
		builder = builder.Set("entity_id", nil)
	}

	if !libCommons.IsNilOrEmpty(acc.PortfolioID) {
		builder = builder.Set("portfolio_id", record.PortfolioID)
	} else if slices.Contains(acc.NullFields, "portfolioId") {
		builder = builder.Set("portfolio_id", nil)
	}

	return builder
}

// Update an Account entity into Postgresql and returns the Account updated.
func (r *AccountPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountPostgreSQLModel{}
	record.FromEntity(acc)

	builder := squirrel.Update(r.tableName)

	if acc.Name != "" {
		builder = builder.Set("name", record.Name)
	}

	if !acc.Status.IsEmpty() {
		builder = builder.Set("status", record.Status)
		builder = builder.Set("status_description", record.StatusDescription)
	}

	if !libCommons.IsNilOrEmpty(acc.Alias) {
		builder = builder.Set("alias", record.Alias)
	}

	if acc.Blocked != nil {
		builder = builder.Set("blocked", *acc.Blocked)
	}

	builder = applyNullableFields(builder, acc, record)

	record.UpdatedAt = time.Now()
	builder = builder.Set("updated_at", record.UpdatedAt)

	builder = builder.Where(squirrel.Eq{"organization_id": organizationID})
	builder = builder.Where(squirrel.Eq{"ledger_id": ledgerID})
	builder = builder.Where(squirrel.Eq{"id": id})
	builder = builder.Where(squirrel.Expr("deleted_at IS NULL"))

	if portfolioID != nil && *portfolioID != uuid.Nil {
		builder = builder.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	builder = builder.PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			schemaDrift := services.IsSchemaDrift(err)

			err := services.ValidatePGError(pgErr, constant.EntityAccount)

			// Schema drift is an infrastructure failure, not a caller mistake, so
			// it has to flip the span red; a constraint violation stays business.
			if schemaDrift {
				libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)
			} else {
				libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)
			}

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete an Account entity from the database (soft delete) using the provided ID.
func (r *AccountPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return err
	}

	builder := squirrel.Update(r.tableName).
		Set("deleted_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		PlaceholderFormat(squirrel.Dollar)

	if portfolioID != nil && *portfolioID != uuid.Nil {
		builder = builder.Where(squirrel.Expr("portfolio_id = ?", *portfolioID))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return err
	}

	_, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccount)
	}

	spanExec.End()

	return nil
}

// ListAccountsByIDs list Accounts entity from the database using the provided IDs.
func (r *AccountPostgreSQLRepository) ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	findAll := squirrel.Select(accountColumns(false)...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", mapped)

		return nil, mapped
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
			&acc.HolderID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
			&acc.HolderCheckSkipped,
		); err != nil {
			mapped := mapReadError(err)

			libOpentelemetry.HandleSpanError(span, "Failed to scan row", mapped)

			return nil, mapped
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

// ListAccountsByAlias list Accounts entity from the database using the provided alias.
func (r *AccountPostgreSQLRepository) ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_accounts_by_alias")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	var accounts []*mmodel.Account

	findAll := squirrel.Select(accountColumns(false)...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("alias = ANY(?)", pq.Array(aliases))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.list_by_alias.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", mapped)

		return nil, mapped
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
			&acc.HolderID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
			&acc.HolderCheckSkipped,
		); err != nil {
			mapped := mapReadError(err)

			libOpentelemetry.HandleSpanError(span, "Failed to scan row", mapped)

			return nil, mapped
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		return nil, err
	}

	return accounts, nil
}

func (r *AccountPostgreSQLRepository) ListExternalAccountsByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, assetCode string) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_external_accounts_by_asset_code")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.asset_code", assetCode),
	)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	var accounts []*mmodel.Account

	findAll := squirrel.Select(accountColumns(false)...).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"asset_code": assetCode}).
		Where(squirrel.Eq{"type": constant.ExternalAccountType}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Executing query", libLog.String("query", query))

	_, spanQuery := tracer.Start(ctx, "postgres.list_external_accounts_by_asset_code.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		spanQuery.End()

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
			&acc.HolderID,
			&acc.AssetCode,
			&acc.OrganizationID,
			&acc.LedgerID,
			&acc.PortfolioID,
			&acc.SegmentID,
			&acc.Status,
			&acc.StatusDescription,
			&acc.Alias,
			&acc.Type,
			&acc.CreatedAt,
			&acc.UpdatedAt,
			&acc.DeletedAt,
			&acc.Blocked,
			&acc.HolderCheckSkipped,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, "Failed to scan row", libLog.Err(err))

			return nil, err
		}

		accounts = append(accounts, acc.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		logger.Log(ctx, libLog.LevelError, "Failed to iterate rows", libLog.Err(err))

		return nil, err
	}

	span.SetAttributes(attribute.Int("db.rows_returned", len(accounts)))

	return accounts, nil
}

// Count retrieves the count of accounts from the database.
func (r *AccountPostgreSQLRepository) Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_accounts")
	defer span.End()

	count := int64(0)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return count, err
	}

	builder := squirrel.Select("COUNT(*)").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return count, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.count.query")

	err = db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", mapped)

		return count, mapped
	}

	spanQuery.End()

	return count, nil
}

func (r *AccountPostgreSQLRepository) CountByHolderID(ctx context.Context, organizationID, holderID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_accounts_by_holder")
	defer span.End()

	count := int64(0)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return count, err
	}

	builder := squirrel.Select("COUNT(*)").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"holder_id": holderID}).
		Where(squirrel.Expr("deleted_at IS NULL")).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return count, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.count_by_holder.query")

	err = db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", mapped)

		return count, mapped
	}

	spanQuery.End()

	return count, nil
}

// mapReadError classifies a read failure. A projection that named a column the
// applied migrations have not created becomes the retryable schema-migration
// sentinel, so the caller learns the cause instead of an opaque 500; every other
// error propagates unchanged.
func mapReadError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && services.IsSchemaDrift(err) {
		return services.ValidatePGError(pgErr, constant.EntityAccount)
	}

	return err
}
