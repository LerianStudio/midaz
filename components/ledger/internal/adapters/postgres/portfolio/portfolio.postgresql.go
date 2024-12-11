package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"github.com/LerianStudio/midaz/pkg/mpointers"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

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

// Repository provides an interface for operations related to portfolio entities.
//
//go:generate mockgen --destination=portfolio.mock.go --package=portfolio . Repository
type Repository interface {
	Create(ctx context.Context, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error)
	FindByIDEntity(ctx context.Context, organizationID, ledgerID, entityID uuid.UUID) (*mmodel.Portfolio, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Portfolio, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Portfolio, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}

// PortfolioPostgreSQLRepository is a Postgresql-specific implementation of the PortfolioRepository.
type PortfolioPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewPortfolioPostgreSQLRepository returns a new instance of PortfolioPostgreSQLRepository using the given Postgres connection.
func NewPortfolioPostgreSQLRepository(pc *mpostgres.PostgresConnection) *PortfolioPostgreSQLRepository {
	c := &PortfolioPostgreSQLRepository{
		connection: pc,
		tableName:  "portfolio",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new portfolio entity into Postgresql and returns it.
func (r *PortfolioPostgreSQLRepository) Create(ctx context.Context, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_portfolio")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &PortfolioPostgreSQLModel{}
	record.FromEntity(portfolio)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "portfolio_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert portfolio record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO portfolio VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *`,
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
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Portfolio{}).Name())
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create Portfolio. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByIDEntity find portfolio from the database using the Entity id.
func (r *PortfolioPostgreSQLRepository) FindByIDEntity(ctx context.Context, organizationID, ledgerID, entityID uuid.UUID) (*mmodel.Portfolio, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_portfolio_by_id_entity")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	portfolio := &PortfolioPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_id_entity.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM portfolio WHERE organization_id = $1 AND ledger_id = $2 AND entity_id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, entityID)

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
		mopentelemetry.HandleSpanError(&span, "Failed to execute query", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		return nil, err
	}

	return portfolio.ToEntity(), nil
}

// FindAll retrieves Portfolio entities from the database.
func (r *PortfolioPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Portfolio, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_portfolios")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var portfolios []*mmodel.Portfolio

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": pkg.NormalizeDate(filter.StartDate, mpointers.Int(-1))}).
		Where(squirrel.LtOrEq{"created_at": pkg.NormalizeDate(filter.EndDate, mpointers.Int(1))}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(pkg.SafeIntToUint64(filter.Limit)).
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

		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
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
			mopentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

			return nil, err
		}

		portfolios = append(portfolios, portfolio.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return portfolios, nil
}

// Find retrieves a Portfolio entity from the database using the provided ID.
func (r *PortfolioPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_portfolio")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	portfolio := &PortfolioPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM portfolio WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, id)

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
		mopentelemetry.HandleSpanError(&span, "Failed to execute query", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())
		}

		return nil, err
	}

	return portfolio.ToEntity(), nil
}

// ListByIDs retrieves Portfolios entities from the database using the provided IDs.
func (r *PortfolioPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Portfolio, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_portfolios_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var portfolios []*mmodel.Portfolio

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_portfolios_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM portfolio WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

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
			mopentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

			return nil, err
		}

		portfolios = append(portfolios, portfolio.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return portfolios, nil
}

// Update a Portfolio entity into Postgresql and returns the Portfolio updated.
func (r *PortfolioPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, portfolio *mmodel.Portfolio) (*mmodel.Portfolio, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_portfolio")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &PortfolioPostgreSQLModel{}
	record.FromEntity(portfolio)

	var updates []string

	var args []any

	if portfolio.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if !portfolio.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE portfolio SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "portfolio_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert portfolio record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Portfolio{}).Name())
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update Portfolio. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Portfolio entity from the database using the provided IDs.
func (r *PortfolioPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_portfolio")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE portfolio SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute delete query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to delete Portfolio. Rows affected is 0", err)

		return err
	}

	return nil
}
