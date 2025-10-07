// Package organization provides PostgreSQL repository implementation for Organization entities.
// This file contains the repository implementation for CRUD operations on organizations.
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

// Repository provides an interface for organization entity persistence operations.
//
// This interface defines the contract for organization data access, following the Repository
// pattern from Domain-Driven Design. It abstracts the underlying data store (PostgreSQL)
// from the business logic layer.
//
// All methods:
//   - Accept context for tracing, logging, and cancellation
//   - Return business errors (not database errors)
//   - Exclude soft-deleted entities from queries (except where noted)
//   - Use UUIDs for entity identification
type Repository interface {
	// Create inserts a new organization into the database.
	// Returns the created organization with generated ID and timestamps.
	Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error)

	// Update modifies an existing organization by ID.
	// Only updates provided fields (partial updates supported).
	// Returns error if organization not found or already deleted.
	Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error)

	// Find retrieves a single organization by ID.
	// Excludes soft-deleted organizations.
	// Returns ErrEntityNotFound if not found.
	Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error)

	// FindAll retrieves a paginated list of organizations.
	// Supports pagination, sorting, and date range filtering.
	// Excludes soft-deleted organizations.
	FindAll(ctx context.Context, filter http.Pagination) ([]*mmodel.Organization, error)

	// ListByIDs retrieves multiple organizations by their IDs.
	// Returns only organizations that exist and are not deleted.
	// Used for batch operations and validation.
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*mmodel.Organization, error)

	// Delete performs a soft delete by setting deleted_at timestamp.
	// Returns error if organization not found or already deleted.
	Delete(ctx context.Context, id uuid.UUID) error

	// Count returns the total number of active organizations.
	// Excludes soft-deleted organizations.
	// Used for pagination metadata.
	Count(ctx context.Context) (int64, error)
}

// OrganizationPostgreSQLRepository is a PostgreSQL implementation of the Repository interface.
//
// This struct provides concrete PostgreSQL-based persistence for organization entities.
// It uses raw SQL queries for performance and control, with squirrel for query building
// where appropriate.
//
// Features:
//   - Soft deletes (deleted_at timestamp)
//   - JSON serialization for address field
//   - OpenTelemetry tracing for all operations
//   - Structured logging with context
//   - Business error conversion from PostgreSQL errors
type OrganizationPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection // PostgreSQL connection pool
	tableName  string                          // Table name ("organization")
}

// NewOrganizationPostgreSQLRepository creates a new PostgreSQL repository instance.
//
// This constructor initializes the repository with a database connection and verifies
// connectivity by attempting to get the database handle. If the connection fails,
// it panics to prevent the application from starting with a broken database connection.
//
// Parameters:
//   - pc: PostgreSQL connection pool from lib-commons
//
// Returns:
//   - *OrganizationPostgreSQLRepository: Initialized repository
//
// Panics:
//   - If database connection cannot be established
//
// Note: Panicking in constructors is acceptable for critical dependencies that
// must be available for the application to function.
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

// Create inserts a new organization into PostgreSQL.
//
// This method:
// 1. Converts domain model to database model
// 2. Marshals address to JSON
// 3. Executes INSERT statement
// 4. Handles PostgreSQL-specific errors (unique violations, foreign key violations)
// 5. Returns the created organization
//
// The method uses ExecContext instead of QueryRowContext because PostgreSQL doesn't
// return the inserted row with RETURNING clause in ExecContext (this appears to be
// a bug in the implementation - the RETURNING clause is present but unused).
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organization: Domain model to insert
//
// Returns:
//   - *mmodel.Organization: Created organization (from input, not from database)
//   - error: Business error if creation fails
//
// Possible Errors:
//   - ErrDuplicateLedger: Legal document already exists
//   - ErrEntityNotFound: Parent organization not found (foreign key violation)
//   - Database connection errors
//
// OpenTelemetry: Creates span "postgres.create_organization"
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

// Update modifies an existing organization in PostgreSQL.
//
// This method implements partial updates by dynamically building the UPDATE statement
// based on which fields are provided. Only non-empty fields are updated, allowing
// clients to update specific fields without affecting others.
//
// Update Logic:
// 1. Converts domain model to database model
// 2. Builds dynamic UPDATE statement with only provided fields
// 3. Executes update with WHERE id = $n AND deleted_at IS NULL
// 4. Returns error if no rows affected (not found or already deleted)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - id: UUID of organization to update
//   - organization: Domain model with fields to update
//
// Returns:
//   - *mmodel.Organization: Updated organization (from input, not from database)
//   - error: Business error if update fails
//
// Possible Errors:
//   - ErrEntityNotFound: Organization not found or already deleted
//   - ErrDuplicateLedger: Legal document conflicts with existing organization
//   - Database connection errors
//
// OpenTelemetry: Creates span "postgres.update_organization"
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

// Find retrieves a single organization by ID from PostgreSQL.
//
// This method fetches an organization and unmarshals its JSON address field.
// It excludes soft-deleted organizations via WHERE deleted_at IS NULL.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - id: UUID of organization to retrieve
//
// Returns:
//   - *mmodel.Organization: Found organization
//   - error: Business error if not found
//
// Possible Errors:
//   - ErrEntityNotFound: Organization not found or soft-deleted
//   - JSON unmarshal errors for address field
//   - Database connection errors
//
// OpenTelemetry: Creates span "postgres.find_organization"
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

// FindAll retrieves a paginated list of organizations from PostgreSQL.
//
// This method uses squirrel query builder for complex filtering and pagination.
// It supports:
//   - Pagination (limit, offset)
//   - Sorting (asc/desc by ID)
//   - Date range filtering (created_at between start and end)
//   - Soft delete exclusion
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - filter: Pagination parameters (limit, page, sort order, date range)
//
// Returns:
//   - []*mmodel.Organization: Array of organizations (empty if none found)
//   - error: Database error if query fails
//
// OpenTelemetry: Creates span "postgres.find_all_organizations"
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

// ListByIDs retrieves multiple organizations by their IDs from PostgreSQL.
//
// This method uses PostgreSQL's ANY operator for efficient batch retrieval.
// It returns only organizations that exist and are not soft-deleted.
// Results are ordered by created_at DESC.
//
// Use Cases:
//   - Batch validation of organization IDs
//   - Fetching multiple organizations for reporting
//   - Validating parent organization references
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - ids: Array of organization UUIDs to retrieve
//
// Returns:
//   - []*mmodel.Organization: Array of found organizations (may be fewer than requested)
//   - error: Database error if query fails
//
// OpenTelemetry: Creates span "postgres.list_organizations_by_ids"
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

// Delete performs a soft delete of an organization in PostgreSQL.
//
// This method sets the deleted_at timestamp to the current time instead of physically
// removing the record. Soft-deleted organizations are excluded from normal queries
// but remain in the database for audit purposes.
//
// The WHERE clause includes "deleted_at IS NULL" to prevent double-deletion and
// ensure idempotency.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - id: UUID of organization to delete
//
// Returns:
//   - error: Business error if deletion fails
//
// Possible Errors:
//   - ErrEntityNotFound: Organization not found or already deleted
//   - Database connection errors
//
// OpenTelemetry: Creates span "postgres.delete_organization"
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

// Count returns the total number of active organizations in PostgreSQL.
//
// This method counts all organizations excluding soft-deleted ones.
// Used for pagination metadata (X-Total-Count header).
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//
// Returns:
//   - int64: Total count of active organizations
//   - error: Database error if query fails
//
// OpenTelemetry: Creates span "postgres.count_organizations"
func (r *OrganizationPostgreSQLRepository) Count(ctx context.Context) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_organizations")
	defer span.End()

	count := int64(0)

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
