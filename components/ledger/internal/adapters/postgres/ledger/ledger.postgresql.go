package ledger

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

// Repository provides an interface for operations related to ledger entities.
//
//go:generate mockgen --destination=ledger.mock.go --package=ledger . Repository
type Repository interface {
	Create(ctx context.Context, ledger *mmodel.Ledger) (*mmodel.Ledger, error)
	Find(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error)
	FindAll(ctx context.Context, organizationID uuid.UUID, filter http.Pagination) ([]*mmodel.Ledger, error)
	FindByName(ctx context.Context, organizationID uuid.UUID, name string) (bool, error)
	ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Ledger, error)
	Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error)
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
}

// LedgerPostgreSQLRepository is a Postgresql-specific implementation of the LedgerRepository.
type LedgerPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewLedgerPostgreSQLRepository returns a new instance of LedgerPostgresRepository using the given Postgres connection.
func NewLedgerPostgreSQLRepository(pc *mpostgres.PostgresConnection) *LedgerPostgreSQLRepository {
	c := &LedgerPostgreSQLRepository{
		connection: pc,
		tableName:  "ledger",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new Ledger entity into Postgresql and returns it.
func (r *LedgerPostgreSQLRepository) Create(ctx context.Context, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &LedgerPostgreSQLModel{}
	record.FromEntity(ledger)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "ledger_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert ledger record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO ledger VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *`,
		record.ID,
		record.Name,
		record.OrganizationID,
		record.Status,
		record.StatusDescription,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Ledger{}).Name())
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create ledger. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Find retrieves a Ledger entity from the database using the provided ID.
func (r *LedgerPostgreSQLRepository) Find(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	ledger := &LedgerPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM ledger WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL", organizationID, id)

	spanQuery.End()

	if err := row.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
		&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		return nil, err
	}

	return ledger.ToEntity(), nil
}

// FindAll retrieves Ledgers entities from the database.
func (r *LedgerPostgreSQLRepository) FindAll(ctx context.Context, organizationID uuid.UUID, filter http.Pagination) ([]*mmodel.Ledger, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_ledgers")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var ledgers []*mmodel.Ledger

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
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
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return ledgers, nil
}

// FindByName returns error and a boolean indicating if Ledger entities exists by name
func (r *LedgerPostgreSQLRepository) FindByName(ctx context.Context, organizationID uuid.UUID, name string) (bool, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_ledger_by_name")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_name.query")

	rows, err := db.QueryContext(ctx,
		"SELECT * FROM ledger WHERE organization_id = $1 AND LOWER(name) LIKE LOWER($2) AND deleted_at IS NULL",
		organizationID,
		name)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, reflect.TypeOf(mmodel.Ledger{}).Name(), name)

		mopentelemetry.HandleSpanError(&span, "Ledger name conflict", err)

		return true, err
	}

	return false, nil
}

// ListByIDs retrieves Ledgers entities from the database using the provided IDs.
func (r *LedgerPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Ledger, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_ledgers_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var ledgers []*mmodel.Ledger

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_ledgers_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM ledger WHERE organization_id = $1 AND id = ANY($2) AND deleted_at IS NULL ORDER BY created_at DESC", organizationID, pq.Array(ids))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return ledgers, nil
}

// Update a Ledger entity into Postgresql and returns the Ledger updated.
func (r *LedgerPostgreSQLRepository) Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &LedgerPostgreSQLModel{}
	record.FromEntity(ledger)

	var updates []string

	var args []any

	if ledger.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if ledger.OrganizationID != "" {
		updates = append(updates, "organization_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.OrganizationID)
	}

	if !ledger.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, id)

	query := `UPDATE ledger SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "ledger_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert ledger record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Ledger{}).Name())
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update ledger. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Ledger entity from the database using the provided ID.
func (r *LedgerPostgreSQLRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE ledger SET deleted_at = now() WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`, organizationID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute database query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to delete ledger. Rows affected is 0", err)

		return err
	}

	return nil
}
