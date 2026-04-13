// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package organization

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v4/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
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

// Repository defines the persistence operations for organization entities.
type Repository interface {
	// Create inserts a new organization, generates its ID, and returns the persisted entity.
	Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error)

	// Update applies non-zero fields from organization to the record identified by id.
	// Returns the updated entity or ErrEntityNotFound if the id does not exist (or is soft-deleted).
	Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error)

	// Find retrieves a single organization by id, excluding soft-deleted records.
	// Returns ErrEntityNotFound when no matching record exists.
	Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error)

	// FindAll returns a paginated list of organizations matching the optional name filters.
	// Both legalName and doingBusinessAs perform case-insensitive prefix matching.
	FindAll(ctx context.Context, filter http.Pagination, legalName, doingBusinessAs *string) ([]*mmodel.Organization, error)

	// ListByIDs returns organizations whose IDs are in the provided slice, excluding soft-deleted records.
	// Returns an empty slice (not an error) when ids is empty.
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*mmodel.Organization, error)

	// Delete performs a soft-delete by setting deleted_at on the record identified by id.
	// Returns ErrEntityNotFound if the id does not exist or is already deleted.
	Delete(ctx context.Context, id uuid.UUID) error

	// Count returns the total number of non-deleted organizations.
	Count(ctx context.Context) (int64, error)
}

// OrganizationPostgreSQLRepository is a Postgresql-specific implementation of the OrganizationRepository.
type OrganizationPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewOrganizationPostgreSQLRepository returns a new instance of OrganizationPostgresRepository using the given Postgres connection.
func NewOrganizationPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *OrganizationPostgreSQLRepository {
	c := &OrganizationPostgreSQLRepository{
		connection: pc,
		tableName:  "organization",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *OrganizationPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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

func (r *OrganizationPostgreSQLRepository) Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_organization")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &OrganizationPostgreSQLModel{}
	record.FromEntity(organization)
	record.ID = uuid.Must(libCommons.GenerateUUIDv7()).String()

	address, err := json.Marshal(record.Address)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal address", err)

		return nil, err
	}

	builder := squirrel.Insert(r.tableName).
		Columns(organizationColumnList...).
		Values(
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
			record.DeletedAt,
		).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build insert query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build insert query", libLog.Err(err))

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.create_organization.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityOrganization)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute insert query", err)
			logger.Log(ctx, libLog.LevelError, "Failed to execute insert query", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute insert query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute insert query", libLog.Err(err))

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected", libLog.Err(err))

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityOrganization)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create organization: no rows affected", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

func (r *OrganizationPostgreSQLRepository) Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_organization")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &OrganizationPostgreSQLModel{}
	record.FromEntity(organization)

	record.UpdatedAt = time.Now()

	builder := squirrel.Update(r.tableName).
		Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if !libCommons.IsNilOrEmpty(organization.ParentOrganizationID) {
		builder = builder.Set("parent_organization_id", record.ParentOrganizationID)
	}

	if !libCommons.IsNilOrEmpty(&organization.LegalName) {
		builder = builder.Set("legal_name", record.LegalName)
	}

	if organization.DoingBusinessAs != nil {
		builder = builder.Set("doing_business_as", record.DoingBusinessAs)
	}

	if !organization.Address.IsEmpty() {
		address, err := json.Marshal(record.Address)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to marshal address", err)

			return nil, err
		}

		builder = builder.Set("address", address)
	}

	if !organization.Status.IsEmpty() {
		builder = builder.Set("status", record.Status).
			Set("status_description", record.StatusDescription)
	}

	builder = builder.Suffix("RETURNING " + strings.Join(organizationColumnList, ", "))

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build update query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build update query", libLog.Err(err))

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.update_organization.exec")
	defer spanExec.End()

	var address string

	updated := &OrganizationPostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&updated.ID,
		&updated.ParentOrganizationID,
		&updated.LegalName,
		&updated.DoingBusinessAs,
		&updated.LegalDocument,
		&address,
		&updated.Status,
		&updated.StatusDescription,
		&updated.CreatedAt,
		&updated.UpdatedAt,
		&updated.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityOrganization)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to update organization: no rows affected", err)

			return nil, err
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityOrganization)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)
			logger.Log(ctx, libLog.LevelError, "Failed to execute update query", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute update query", libLog.Err(err))

		return nil, err
	}

	if err := json.Unmarshal([]byte(address), &updated.Address); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal address", err)

		return nil, err
	}

	return updated.ToEntity(), nil
}

func (r *OrganizationPostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_organization")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	organization := &OrganizationPostgreSQLModel{}

	var address string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	findQuery := squirrel.Select(organizationColumnList...).
		From("organization").
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		spanQuery.End()

		return nil, err
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
		&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
		&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to scan row", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

		return nil, err
	}

	err = json.Unmarshal([]byte(address), &organization.Address)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal address", err)

		return nil, err
	}

	return organization.ToEntity(), nil
}

func (r *OrganizationPostgreSQLRepository) FindAll(ctx context.Context, filter http.Pagination, legalName, doingBusinessAs *string) ([]*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_organizations")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	var organizations []*mmodel.Organization

	findAll := squirrel.Select(organizationColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	if legalName != nil && *legalName != "" {
		sanitized := http.EscapeSearchMetacharacters(*legalName)
		findAll = findAll.Where(squirrel.ILike{"legal_name": sanitized + "%"})
	}

	if doingBusinessAs != nil && *doingBusinessAs != "" {
		sanitized := http.EscapeSearchMetacharacters(*doingBusinessAs)
		findAll = findAll.Where(squirrel.ILike{"doing_business_as": sanitized + "%"})
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal address", err)

			return nil, err
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		return nil, err
	}

	return organizations, nil
}

func (r *OrganizationPostgreSQLRepository) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*mmodel.Organization, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_organizations_by_ids")
	defer span.End()

	if len(ids) == 0 {
		return []*mmodel.Organization{}, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	var organizations []*mmodel.Organization

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_organizations_by_ids.query")

	listQuery := squirrel.Select(organizationColumnList...).
		From("organization").
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := listQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to build query: %v", err))

		spanQuery.End()

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

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
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to scan row: %v", err))

			return nil, err
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal address", err)

			return nil, err
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		return nil, err
	}

	return organizations, nil
}

func (r *OrganizationPostgreSQLRepository) Delete(ctx context.Context, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_organization")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE organization SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get rows affected: %v", err))

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete organization. Rows affected is 0", err)

		return err
	}

	return nil
}

func (r *OrganizationPostgreSQLRepository) Count(ctx context.Context) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_organizations")
	defer span.End()

	count := int64(0)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return count, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))

		return count, err
	}

	return count, nil
}
