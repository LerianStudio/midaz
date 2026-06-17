// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package portfolio

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
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libPointers "github.com/LerianStudio/lib-commons/v5/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

var portfolioColumnList = []string{
	"id",
	"name",
	"entity_id",
	"ledger_id",
	"organization_id",
	"status",
	"status_description",
	"created_at",
	"updated_at",
	"deleted_at",
}

// Repository provides an interface for operations related to portfolio entities.
// It defines methods for creating, finding, updating, and deleting portfolios in the database.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=portfolio.postgresql_mock.go --package=portfolio . Repository
type Repository interface {
	Create(ctx context.Context, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error)
	FindByIDEntity(ctx context.Context, organizationID, ledgerID, entityID uuid.UUID) (*mmodel.Portfolio, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Portfolio, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Portfolio, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error)
}

// PortfolioPostgreSQLRepository is a Postgresql-specific implementation of the PortfolioRepository.
type PortfolioPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewPortfolioPostgreSQLRepository returns a new instance of PortfolioPostgreSQLRepository using the given Postgres connection.
func NewPortfolioPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *PortfolioPostgreSQLRepository {
	c := &PortfolioPostgreSQLRepository{
		connection: pc,
		tableName:  "portfolio",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *PortfolioPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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

// Create a new portfolio entity into Postgresql and returns it.
func (r *PortfolioPostgreSQLRepository) Create(ctx context.Context, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_portfolio")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &PortfolioPostgreSQLModel{}
	record.FromEntity(portfolio)

	_, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	insertQuery := `INSERT INTO portfolio VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING ` + strings.Join(portfolioColumnList, ", ")

	inserted := &PortfolioPostgreSQLModel{}

	row := db.QueryRowContext(ctx, insertQuery,
		record.ID,
		record.Name,
		record.EntityID,
		record.LedgerID,
		record.OrganizationID,
		record.Status,
		record.StatusDescription,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err := row.Scan(
		&inserted.ID,
		&inserted.Name,
		&inserted.EntityID,
		&inserted.LedgerID,
		&inserted.OrganizationID,
		&inserted.Status,
		&inserted.StatusDescription,
		&inserted.CreatedAt,
		&inserted.UpdatedAt,
		&inserted.DeletedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityPortfolio)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute insert query", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to execute insert query", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute insert query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute insert query", libLog.Err(err))

		return nil, err
	}

	return inserted.ToEntity(), nil
}

// FindByIDEntity find portfolio from the database using the Entity id.
func (r *PortfolioPostgreSQLRepository) FindByIDEntity(ctx context.Context, organizationID, ledgerID, entityID uuid.UUID) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_portfolio_by_id_entity")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	portfolio := &PortfolioPostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find_by_id_entity.query")

	query, args, err := squirrel.Select(portfolioColumnList...).
		From("portfolio").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"entity_id": entityID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		spanQuery.End()

		return nil, err
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&portfolio.ID,
		&portfolio.Name,
		&portfolio.EntityID,
		&portfolio.LedgerID,
		&portfolio.OrganizationID,
		&portfolio.Status,
		&portfolio.StatusDescription,
		&portfolio.CreatedAt,
		&portfolio.UpdatedAt,
		&portfolio.DeletedAt); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)
		}

		return nil, err
	}

	return portfolio.ToEntity(), nil
}

// FindAll retrieves Portfolio entities from the database.
func (r *PortfolioPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_portfolios")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	var portfolios []*mmodel.Portfolio

	pagination := filter.ToOffsetPagination()

	findAll := squirrel.Select(portfolioColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Eq{"deleted_at": nil})

	if !pagination.StartDate.IsZero() {
		findAll = findAll.
			Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(pagination.StartDate, libPointers.Int(0), false)}).
			Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(pagination.EndDate, libPointers.Int(0), true)})
	}

	findAll = findAll.OrderBy("id " + strings.ToUpper(pagination.SortOrder)).
		Limit(libCommons.SafeIntToUint64(pagination.Limit)).
		Offset(libCommons.SafeIntToUint64((pagination.Page - 1) * pagination.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	// Filter by entity IDs when provided (metadata composition)
	if len(filter.EntityIDs) > 0 {
		findAll = findAll.Where(squirrel.Expr("id = ANY(?)", pq.Array(filter.EntityIDs)))
	}

	if !libCommons.IsNilOrEmpty(filter.EntityID) {
		findAll = findAll.Where(squirrel.Expr("entity_id = ?", *filter.EntityID))
	}

	if !libCommons.IsNilOrEmpty(filter.Status) {
		findAll = findAll.Where(squirrel.Expr("status = ?", *filter.Status))
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var portfolio PortfolioPostgreSQLModel
		if err := rows.Scan(
			&portfolio.ID,
			&portfolio.Name,
			&portfolio.EntityID,
			&portfolio.LedgerID,
			&portfolio.OrganizationID,
			&portfolio.Status,
			&portfolio.StatusDescription,
			&portfolio.CreatedAt,
			&portfolio.UpdatedAt,
			&portfolio.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)

			logger.Log(ctx, libLog.LevelError, "Failed to scan rows", libLog.Err(err))

			return nil, err
		}

		portfolios = append(portfolios, portfolio.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		return nil, err
	}

	return portfolios, nil
}

// Find retrieves a Portfolio entity from the database using the provided ID.
func (r *PortfolioPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_portfolio")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	portfolio := &PortfolioPostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find.query")

	query, args, err := squirrel.Select(portfolioColumnList...).
		From("portfolio").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		spanQuery.End()

		return nil, err
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&portfolio.ID,
		&portfolio.Name,
		&portfolio.EntityID,
		&portfolio.LedgerID,
		&portfolio.OrganizationID,
		&portfolio.Status,
		&portfolio.StatusDescription,
		&portfolio.CreatedAt,
		&portfolio.UpdatedAt,
		&portfolio.DeletedAt); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)
		}

		return nil, err
	}

	return portfolio.ToEntity(), nil
}

// ListByIDs retrieves Portfolios entities from the database using the provided IDs.
func (r *PortfolioPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_portfolios_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	var portfolios []*mmodel.Portfolio

	_, spanQuery := tracer.Start(ctx, "postgres.list_portfolios_by_ids.query")

	query, args, err := squirrel.Select(portfolioColumnList...).
		From("portfolio").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		spanQuery.End()

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var portfolio PortfolioPostgreSQLModel
		if err := rows.Scan(
			&portfolio.ID,
			&portfolio.Name,
			&portfolio.EntityID,
			&portfolio.LedgerID,
			&portfolio.OrganizationID,
			&portfolio.Status,
			&portfolio.StatusDescription,
			&portfolio.CreatedAt,
			&portfolio.UpdatedAt,
			&portfolio.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)

			logger.Log(ctx, libLog.LevelError, "Failed to scan rows", libLog.Err(err))

			return nil, err
		}

		portfolios = append(portfolios, portfolio.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		return nil, err
	}

	return portfolios, nil
}

// Update a Portfolio entity into Postgresql and returns the Portfolio updated.
func (r *PortfolioPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_portfolio")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &PortfolioPostgreSQLModel{}
	record.FromEntity(portfolio)

	record.UpdatedAt = time.Now()

	builder := squirrel.Update(r.tableName).
		Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if portfolio.EntityID != "" {
		builder = builder.Set("entity_id", record.EntityID)
	}

	if portfolio.Name != "" {
		builder = builder.Set("name", record.Name)
	}

	if !portfolio.Status.IsEmpty() {
		builder = builder.Set("status", record.Status).
			Set("status_description", record.StatusDescription)
	}

	builder = builder.Suffix("RETURNING " + strings.Join(portfolioColumnList, ", "))

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build update query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build update query", libLog.Err(err))

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	updated := &PortfolioPostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&updated.ID,
		&updated.Name,
		&updated.EntityID,
		&updated.LedgerID,
		&updated.OrganizationID,
		&updated.Status,
		&updated.StatusDescription,
		&updated.CreatedAt,
		&updated.UpdatedAt,
		&updated.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to update portfolio. Rows affected is 0", err)

			return nil, err
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityPortfolio)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to execute update query", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute update query", libLog.Err(err))

		return nil, err
	}

	return updated.ToEntity(), nil
}

// Delete removes a Portfolio entity from the database using the provided IDs.
func (r *PortfolioPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_portfolio")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return err
	}

	_, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE portfolio SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPortfolio)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete Portfolio. Rows affected is 0", err)

		return err
	}

	return nil
}

// Count retrieves the number of Portfolio entities in the database.
func (r *PortfolioPostgreSQLRepository) Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_portfolios")
	defer span.End()

	count := int64(0)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return count, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM portfolio WHERE organization_id = $1 AND ledger_id = $2 AND deleted_at IS NULL", organizationID, ledgerID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return count, err
	}

	return count, nil
}
