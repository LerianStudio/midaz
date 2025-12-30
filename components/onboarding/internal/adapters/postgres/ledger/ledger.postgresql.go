package ledger

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

var ledgerColumnList = []string{
	"id",
	"name",
	"organization_id",
	"status",
	"status_description",
	"created_at",
	"updated_at",
	"deleted_at",
}

// Repository provides an interface for operations related to ledger entities.
// It defines methods for creating, finding, updating, and deleting ledgers in the database.
type Repository interface {
	Create(ctx context.Context, ledger *mmodel.Ledger) (*mmodel.Ledger, error)
	Find(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error)
	FindAll(ctx context.Context, organizationID uuid.UUID, filter http.Pagination) ([]*mmodel.Ledger, error)
	FindByName(ctx context.Context, organizationID uuid.UUID, name string) (bool, error)
	ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Ledger, error)
	Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error)
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	Count(ctx context.Context, organizationID uuid.UUID) (int64, error)
}

// LedgerPostgreSQLRepository is a Postgresql-specific implementation of the LedgerRepository.
type LedgerPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	wrapper    *mmigration.MigrationWrapper // For future health checks
	tableName  string
}

// NewLedgerPostgreSQLRepository returns a new instance of LedgerPostgresRepository using the given MigrationWrapper.
func NewLedgerPostgreSQLRepository(mw *mmigration.MigrationWrapper) *LedgerPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "LedgerPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "LedgerPostgreSQLRepository")

	return &LedgerPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "ledger",
	}
}

// Create a new Ledger entity into Postgresql and returns it.
func (r *LedgerPostgreSQLRepository) Create(ctx context.Context, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	assert.NotNil(ledger, "ledger entity must not be nil for Create",
		"repository", "LedgerPostgreSQLRepository")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	record := &LedgerPostgreSQLModel{}
	record.FromEntity(ledger)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			validatedErr := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", validatedErr)

			logger.Warnf("Failed to execute update query: %v", validatedErr)

			return nil, validatedErr
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	if rowsAffected == 0 {
		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger. Rows affected is 0", notFoundErr)

		return nil, notFoundErr
	}

	return record.ToEntity(), nil
}

// Find retrieves a Ledger entity from the database using the provided ID.
func (r *LedgerPostgreSQLRepository) Find(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	ledger := &LedgerPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"id": id}).
		Where("deleted_at IS NULL").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
		&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	return ledger.ToEntity(), nil
}

// FindAll retrieves Ledgers entities from the database.
func (r *LedgerPostgreSQLRepository) FindAll(ctx context.Context, organizationID uuid.UUID, filter http.Pagination) ([]*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_ledgers")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	var ledgers []*mmodel.Ledger

	findAll := squirrel.Select(ledgerColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where("deleted_at IS NULL").
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	return ledgers, nil
}

// FindByName returns error and a boolean indicating if Ledger entities exists by name
func (r *LedgerPostgreSQLRepository) FindByName(ctx context.Context, organizationID uuid.UUID, name string) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_ledger_by_name")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return false, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_name.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Expr("LOWER(name) LIKE LOWER(?)", name)).
		Where("deleted_at IS NULL").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		spanQuery.End()

		return false, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return false, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, reflect.TypeOf(mmodel.Ledger{}).Name(), name)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Ledger name conflict", err)

		return true, err
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Row iteration error", err)

		return false, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	return false, nil
}

// ListByIDs retrieves Ledgers entities from the database using the provided IDs.
func (r *LedgerPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_ledgers_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	var ledgers []*mmodel.Ledger

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_ledgers_by_ids.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where("deleted_at IS NULL").
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	return ledgers, nil
}

// Update a Ledger entity into Postgresql and returns the Ledger updated.
func (r *LedgerPostgreSQLRepository) Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	assert.NotNil(ledger, "ledger entity must not be nil for Update",
		"organization_id", organizationID,
		"ledger_id", id)

	// Ensure FromEntity preserves the correct ID for Update operations
	ledger.ID = id.String()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
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
		` AND deleted_at IS NULL RETURNING ` + strings.Join(ledgerColumnList, ", ")

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	var updated LedgerPostgreSQLModel

	err = db.QueryRowContext(ctx, query, args...).Scan(
		&updated.ID,
		&updated.Name,
		&updated.OrganizationID,
		&updated.Status,
		&updated.StatusDescription,
		&updated.CreatedAt,
		&updated.UpdatedAt,
		&updated.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger. Rows affected is 0", notFoundErr)

			return nil, notFoundErr
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			validatedErr := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", validatedErr)

			logger.Warnf("Failed to execute update query: %v", validatedErr)

			return nil, validatedErr
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	spanExec.End()

	return updated.ToEntity(), nil
}

// Delete removes a Ledger entity from the database using the provided ID.
func (r *LedgerPostgreSQLRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE ledger SET deleted_at = now() WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`, organizationID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute database query", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete ledger. Rows affected is 0", err)

		return err
	}

	return nil
}

// Count retrieves the number of Ledger entities in the database for the given organization ID.
func (r *LedgerPostgreSQLRepository) Count(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_ledgers")
	defer span.End()

	count := int64(0)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return count, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ledger WHERE organization_id = $1 AND deleted_at IS NULL", organizationID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return count, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	return count, nil
}
