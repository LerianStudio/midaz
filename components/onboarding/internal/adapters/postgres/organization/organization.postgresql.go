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
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/trace"
)

var organizationColumnList = []string{
	"id",
	"parent_organization_id",
	"legal_name",
	"doing_business_as",
	"legal_document",
	"address",
	"status",
	"status_description",
	"created_at",
	"updated_at",
	"deleted_at",
}

// Repository provides an interface for operations related to organization entities.
// It defines methods for creating, updating, finding, and deleting organizations.
type Repository interface {
	Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error)
	Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error)
	Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error)
	FindAll(ctx context.Context, filter http.Pagination) ([]*mmodel.Organization, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*mmodel.Organization, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context) (int64, error)
}

// OrganizationPostgreSQLRepository is a Postgresql-specific implementation of the OrganizationRepository.
type OrganizationPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	wrapper    *mmigration.MigrationWrapper // For future health checks
	tableName  string
}

// NewOrganizationPostgreSQLRepository returns a new instance of OrganizationPostgresRepository using the given MigrationWrapper.
func NewOrganizationPostgreSQLRepository(mw *mmigration.MigrationWrapper) *OrganizationPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "OrganizationPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "OrganizationPostgreSQLRepository")

	return &OrganizationPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "organization",
	}
}

// Create inserts a new Organization entity into Postgresql and returns the created Organization.
func (r *OrganizationPostgreSQLRepository) Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error) {
	assert.NotNil(organization, "organization entity must not be nil for Create",
		"repository", "OrganizationPostgreSQLRepository")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	record := &OrganizationPostgreSQLModel{}
	record.FromEntity(organization)

	address, err := json.Marshal(record.Address)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal address", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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
			validatedErr := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", validatedErr)

			logger.Warnf("Failed to execute update query: %v", validatedErr)

			return nil, validatedErr
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	if rowsAffected == 0 {
		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create organization. Rows affected is 0", notFoundErr)

		return nil, notFoundErr
	}

	return record.ToEntity(), nil
}

// buildOrganizationUpdateQuery constructs the SQL update query for an organization
func (r *OrganizationPostgreSQLRepository) buildOrganizationUpdateQuery(organization *mmodel.Organization, record *OrganizationPostgreSQLModel, id uuid.UUID, span *trace.Span) (string, []any, error) {
	var (
		updates []string
		args    []any
	)

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
			libOpentelemetry.HandleSpanError(span, "Failed to marshal address", err)
			return "", nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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

	return query, args, nil
}

// Update an Organization entity into Postgresql and returns the Organization updated.
func (r *OrganizationPostgreSQLRepository) Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error) {
	assert.NotNil(organization, "organization entity must not be nil for Update",
		"organization_id", id)

	// Ensure FromEntity preserves the correct ID for Update operations
	organization.ID = id.String()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	record := &OrganizationPostgreSQLModel{}
	record.FromEntity(organization)

	query, args, err := r.buildOrganizationUpdateQuery(organization, record, id, &span)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			validatedErr := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", validatedErr)

			logger.Warnf("Failed to execute update query: %v", validatedErr)

			return nil, validatedErr
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	if rowsAffected == 0 {
		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update organization. Rows affected is 0", notFoundErr)

		return nil, notFoundErr
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

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	organization := &OrganizationPostgreSQLModel{}

	var address string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	findQuery := squirrel.Select(organizationColumnList...).
		From("organization").
		Where(squirrel.Eq{"id": id}).
		Where("deleted_at IS NULL").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
		&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
		&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	err = json.Unmarshal([]byte(address), &organization.Address)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	var organizations []*mmodel.Organization

	findAll := squirrel.Select(organizationColumnList...).
		From(r.tableName).
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

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	var organizations []*mmodel.Organization

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_organizations_by_ids.query")

	listQuery := squirrel.Select(organizationColumnList...).
		From("organization").
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where("deleted_at IS NULL").
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := listQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE organization SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
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

	count := int64(0)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return count, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return count, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	return count, nil
}
