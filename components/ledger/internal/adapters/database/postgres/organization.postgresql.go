package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	o "github.com/LerianStudio/midaz/components/ledger/internal/adapters/interface/onboarding/organization"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// OrganizationPostgreSQLRepository is a Postgresql-specific implementation of the OrganizationRepository.
type OrganizationPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewOrganizationPostgreSQLRepository returns a new instance of OrganizationPostgresRepository using the given Postgres connection.
func NewOrganizationPostgreSQLRepository(pc *mpostgres.PostgresConnection) *OrganizationPostgreSQLRepository {
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
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &o.OrganizationPostgreSQLModel{}
	record.FromEntity(organization)

	address, err := json.Marshal(record.Address)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to marshal address", err)

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "organization_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert organization record from entity to JSON string", err)

		return nil, err
	}

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
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Organization{}).Name())
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
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create organization. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Update an Organization entity into Postgresql and returns the Organization updated.
func (r *OrganizationPostgreSQLRepository) Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &o.OrganizationPostgreSQLModel{}
	record.FromEntity(organization)

	var updates []string

	var args []any

	if !common.IsNilOrEmpty(organization.ParentOrganizationID) {
		updates = append(updates, "parent_organization_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.ParentOrganizationID)
	}

	if !common.IsNilOrEmpty(&organization.LegalName) {
		updates = append(updates, "legal_name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.LegalName)
	}

	if !common.IsNilOrEmpty(organization.DoingBusinessAs) {
		updates = append(updates, "doing_business_as = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.DoingBusinessAs)
	}

	if !organization.Address.IsEmpty() {
		address, err := json.Marshal(record.Address)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to marshal address", err)

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

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "organization_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert organization record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Organization{}).Name())
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
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update organization. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Find retrieves an Organization entity from the database using the provided ID.
func (r *OrganizationPostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	organization := &o.OrganizationPostgreSQLModel{}

	var address string

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, `SELECT * FROM organization WHERE id = $1`, id)

	spanQuery.End()

	if err := row.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
		&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
		&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return nil, err
	}

	err = json.Unmarshal([]byte(address), &organization.Address)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

		return nil, err
	}

	return organization.ToEntity(), nil
}

// FindAll retrieves Organizations entities from the database.
func (r *OrganizationPostgreSQLRepository) FindAll(ctx context.Context, limit, page int) ([]*mmodel.Organization, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_organizations")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var organizations []*mmodel.Organization

	findAll := sqrl.Select("*").
		From(r.tableName).
		Where(sqrl.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		Limit(common.SafeIntToUint64(limit)).
		Offset(common.SafeIntToUint64((page - 1) * limit)).
		PlaceholderFormat(sqrl.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}

	spanQuery.End()

	defer rows.Close()

	for rows.Next() {
		var organization o.OrganizationPostgreSQLModel

		var address string

		if err := rows.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
			&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
			&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

			return nil, err
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return organizations, nil
}

// ListByIDs retrieves Organizations entities from the database using the provided IDs.
func (r *OrganizationPostgreSQLRepository) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*mmodel.Organization, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_organizations_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var organizations []*mmodel.Organization

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_organizations_by_ids.query")

	rows, err := db.QueryContext(ctx, `SELECT * FROM organization WHERE id = ANY($1) AND deleted_at IS NULL ORDER BY created_at DESC`, pq.Array(ids))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var organization o.OrganizationPostgreSQLModel

		var address string

		if err := rows.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
			&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
			&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to unmarshal address", err)

			return nil, err
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return organizations, nil
}

// Delete removes an Organization entity from the database using the provided ID.
func (r *OrganizationPostgreSQLRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_organization")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE organization SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to delete organization. Rows affected is 0", err)

		return err
	}

	return nil
}
