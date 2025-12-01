package organization

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Repository provides an interface for organization persistence operations.
//
// This interface defines the contract for organization CRUD operations, following
// the repository pattern from Domain-Driven Design. It abstracts PostgreSQL-specific
// implementation details from the application layer.
//
// Design Decisions:
//
//   - No multi-tenant scoping: Organizations ARE the tenant root
//   - Hierarchical support: Parent-child organization relationships
//   - Soft delete: Delete marks records, preserving audit trail
//   - Batch operations: ListByIDs for efficient bulk lookups
//   - Count operations: For pagination and dashboard metrics
//
// Usage:
//
//	repo := organization.NewOrganizationPostgreSQLRepository(connection)
//	org, err := repo.Create(ctx, &organization)
//	found, err := repo.Find(ctx, orgID)
//
// Thread Safety:
//
// All methods are thread-safe. The underlying database driver handles connection
// pooling and concurrent access.
//
// Observability:
//
// All methods create OpenTelemetry spans for distributed tracing.
// Span names follow the pattern: postgres.<operation>_organization
type Repository interface {
	Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error)
	Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error)
	Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error)
	FindAll(ctx context.Context, filter http.Pagination) ([]*mmodel.Organization, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*mmodel.Organization, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context) (int64, error)
}

// OrganizationPostgreSQLRepository is the PostgreSQL implementation of the Repository interface.
//
// This repository provides organization persistence using PostgreSQL as the backing store.
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
// Address Handling:
//
// Organization addresses are stored as JSONB, requiring:
//   - JSON marshaling on write
//   - JSON unmarshaling on read
//   - No schema changes for address format updates
//
// Lifecycle:
//
//	conn := libPostgres.NewPostgresConnection(cfg)
//	repo := organization.NewOrganizationPostgreSQLRepository(conn)
//	// Use repository...
//	// Connection cleanup handled by PostgresConnection
//
// Thread Safety:
//
// OrganizationPostgreSQLRepository is thread-safe after initialization.
//
// Fields:
//   - connection: Shared PostgreSQL connection (manages pool and lifecycle)
//   - tableName: Database table name ("organization")
type OrganizationPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewOrganizationPostgreSQLRepository creates a new OrganizationPostgreSQLRepository instance.
//
// This constructor initializes the repository with a PostgreSQL connection and
// validates connectivity before returning. It panics on connection failure
// to fail fast during application startup.
//
// Initialization Process:
//  1. Store connection reference
//  2. Set table name to "organization"
//  3. Verify connectivity by calling GetDB
//  4. Panic if connection fails (fail-fast startup)
//
// Parameters:
//   - pc: Configured PostgreSQL connection from lib-commons
//
// Returns:
//   - *OrganizationPostgreSQLRepository: Initialized repository ready for use
//
// Panics:
//   - "Failed to connect database": Connection verification failed
func NewOrganizationPostgreSQLRepository(pc *libPostgres.PostgresConnection) *OrganizationPostgreSQLRepository {
	c := &OrganizationPostgreSQLRepository{
		connection: pc,
		tableName:  "organization",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create inserts a new Organization entity into Postgresql and returns the created Organization.
func (r *OrganizationPostgreSQLRepository) Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &OrganizationPostgreSQLModel{}
	record.FromEntity(organization)

	address, err := json.Marshal(record.Address)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal address", err)

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	result, err := db.ExecContext(ctx, `INSERT INTO organization VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *`,
		record.ID,
		record.ParentOrganizationID,
		record.LegalName,
		record.DoingBusinessAs,
		record.LegalDocument,
		address,
		record.Status,
		record.StatusDescription,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Organization{}).Name())

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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create organization. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Update an Organization entity into Postgresql and returns the Organization updated.
func (r *OrganizationPostgreSQLRepository) Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &OrganizationPostgreSQLModel{}
	record.FromEntity(organization)

	var updates []string

	var args []any

	if !libCommons.IsNilOrEmpty(organization.ParentOrganizationID) {
		updates = append(updates, "parent_organization_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.ParentOrganizationID)
	}

	if !libCommons.IsNilOrEmpty(&organization.LegalName) {
		updates = append(updates, "legal_name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.LegalName)
	}

	if !libCommons.IsNilOrEmpty(organization.DoingBusinessAs) {
		updates = append(updates, "doing_business_as = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.DoingBusinessAs)
	}

	if !organization.Address.IsEmpty() {
		address, err := json.Marshal(record.Address)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to marshal address", err)

			return nil, err
		}

		updates = append(updates, "address = $"+strconv.Itoa(len(args)+1))
		args = append(args, address)
	}

	if !organization.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, id)
	query := `UPDATE organization SET ` + strings.Join(updates, ", ") +
		` WHERE id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Organization{}).Name())

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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update organization. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Find retrieves an Organization entity from the database using the provided ID.
func (r *OrganizationPostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	organization := &OrganizationPostgreSQLModel{}

	var address string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, `SELECT * FROM organization WHERE id = $1 AND deleted_at IS NULL`, id)

	spanQuery.End()

	if err := row.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
		&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
		&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
	}

	err = json.Unmarshal([]byte(address), &organization.Address)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

		return nil, err
	}

	return organization.ToEntity(), nil
}

// FindAll retrieves Organizations entities from the database.
func (r *OrganizationPostgreSQLRepository) FindAll(ctx context.Context, filter http.Pagination) ([]*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_organizations")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var organizations []*mmodel.Organization

	findAll := squirrel.Select("*").
		From(r.tableName).
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
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}

	spanQuery.End()

	defer rows.Close()

	for rows.Next() {
		var organization OrganizationPostgreSQLModel

		var address string

		if err := rows.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
			&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
			&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

			return nil, err
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return organizations, nil
}

// ListByIDs retrieves Organizations entities from the database using the provided IDs.
func (r *OrganizationPostgreSQLRepository) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_organizations_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var organizations []*mmodel.Organization

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_organizations_by_ids.query")

	rows, err := db.QueryContext(ctx, `SELECT * FROM organization WHERE id = ANY($1) AND deleted_at IS NULL ORDER BY created_at DESC`, pq.Array(ids))
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var organization OrganizationPostgreSQLModel

		var address string

		if err := rows.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
			&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
			&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

			return nil, err
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return organizations, nil
}

// Delete removes an Organization entity from the database using the provided ID.
func (r *OrganizationPostgreSQLRepository) Delete(ctx context.Context, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE organization SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete organization. Rows affected is 0", err)

		return err
	}

	return nil
}

// Count retrieves the total count of organizations.
func (r *OrganizationPostgreSQLRepository) Count(ctx context.Context) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_organizations")
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

	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return count, err
	}

	return count, nil
}
