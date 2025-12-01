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
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// Repository provides an interface for ledger persistence operations.
//
// This interface defines the contract for ledger CRUD operations, following
// the repository pattern from Domain-Driven Design. It abstracts PostgreSQL-specific
// implementation details from the application layer.
//
// Design Decisions:
//
//   - Organization scoping: All operations require organizationID for multi-tenant isolation
//   - Name uniqueness: FindByName validates names within organization scope
//   - Soft delete: Delete marks records, preserving audit trail
//   - Batch operations: ListByIDs for efficient bulk lookups
//   - Count operations: For pagination and dashboard metrics
//
// Usage:
//
//	repo := ledger.NewLedgerPostgreSQLRepository(connection)
//	led, err := repo.Create(ctx, &ledger)
//	found, err := repo.Find(ctx, orgID, ledgerID)
//
// Thread Safety:
//
// All methods are thread-safe. The underlying database driver handles connection
// pooling and concurrent access.
//
// Observability:
//
// All methods create OpenTelemetry spans for distributed tracing.
// Span names follow the pattern: postgres.<operation>_ledger
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

// LedgerPostgreSQLRepository is the PostgreSQL implementation of the Repository interface.
//
// This repository provides ledger persistence using PostgreSQL as the backing store.
// It implements the hexagonal architecture pattern by adapting the domain Repository
// interface to PostgreSQL-specific operations.
//
// Connection Management:
//
// The repository uses a shared PostgresConnection from lib-commons which provides:
//   - Connection pooling
//   - Automatic reconnection
//   - Health checks
//
// Lifecycle:
//
//	conn := libPostgres.NewPostgresConnection(cfg)
//	repo := ledger.NewLedgerPostgreSQLRepository(conn)
//	// Use repository...
//	// Connection cleanup handled by PostgresConnection
//
// Thread Safety:
//
// LedgerPostgreSQLRepository is thread-safe after initialization.
//
// Fields:
//   - connection: Shared PostgreSQL connection (manages pool and lifecycle)
//   - tableName: Database table name ("ledger")
type LedgerPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewLedgerPostgreSQLRepository creates a new LedgerPostgreSQLRepository instance.
//
// This constructor initializes the repository with a PostgreSQL connection and
// validates connectivity before returning. It panics on connection failure
// to fail fast during application startup.
//
// Initialization Process:
//  1. Store connection reference
//  2. Set table name to "ledger"
//  3. Verify connectivity by calling GetDB
//  4. Panic if connection fails (fail-fast startup)
//
// Parameters:
//   - pc: Configured PostgreSQL connection from lib-commons
//
// Returns:
//   - *LedgerPostgreSQLRepository: Initialized repository ready for use
//
// Panics:
//   - "Failed to connect database": Connection verification failed
func NewLedgerPostgreSQLRepository(pc *libPostgres.PostgresConnection) *LedgerPostgreSQLRepository {
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
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
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", err)

			logger.Warnf("Failed to execute update query: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger. Rows affected is 0", err)

		return nil, err
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

		return nil, err
	}

	ledger := &LedgerPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM ledger WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL", organizationID, id)

	spanQuery.End()

	if err := row.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
		&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
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

		return nil, err
	}

	var ledgers []*mmodel.Ledger

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Eq{"deleted_at": nil}).
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

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
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

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_name.query")

	rows, err := db.QueryContext(ctx,
		"SELECT * FROM ledger WHERE organization_id = $1 AND LOWER(name) LIKE LOWER($2) AND deleted_at IS NULL",
		organizationID,
		name)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, reflect.TypeOf(mmodel.Ledger{}).Name(), name)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Ledger name conflict", err)

		return true, err
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

		return nil, err
	}

	var ledgers []*mmodel.Ledger

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_ledgers_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM ledger WHERE organization_id = $1 AND id = ANY($2) AND deleted_at IS NULL ORDER BY created_at DESC", organizationID, pq.Array(ids))
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return ledgers, nil
}

// Update a Ledger entity into Postgresql and returns the Ledger updated.
func (r *LedgerPostgreSQLRepository) Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

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

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", err)

			logger.Warnf("Failed to execute update query: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
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

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE ledger SET deleted_at = now() WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`, organizationID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute database query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return err
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

	var count = int64(0)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return count, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ledger WHERE organization_id = $1 AND deleted_at IS NULL", organizationID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return count, err
	}

	return count, nil
}
