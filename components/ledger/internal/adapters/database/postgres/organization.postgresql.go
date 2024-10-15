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

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
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
func (r *OrganizationPostgreSQLRepository) Create(ctx context.Context, organization *o.Organization) (*o.Organization, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &o.OrganizationPostgreSQLModel{}
	record.FromEntity(organization)

	address, err := json.Marshal(record.Address)
	if err != nil {
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(o.Organization{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(o.Organization{}).Name())
	}

	return record.ToEntity(), nil
}

// Update an Organization entity into Postgresql and returns the Organization updated.
func (r *OrganizationPostgreSQLRepository) Update(ctx context.Context, id uuid.UUID, organization *o.Organization) (*o.Organization, error) {
	db, err := r.connection.GetDB()
	if err != nil {
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

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(o.Organization{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(o.Organization{}).Name())
	}

	return record.ToEntity(), nil
}

// Find retrieves an Organization entity from the database using the provided ID.
func (r *OrganizationPostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*o.Organization, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	organization := &o.OrganizationPostgreSQLModel{}

	var address string

	row := db.QueryRowContext(ctx, `SELECT * FROM organization WHERE id = $1 AND deleted_at IS NULL`, id)
	if err := row.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
		&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
		&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(o.Organization{}).Name())
		}

		return nil, err
	}

	err = json.Unmarshal([]byte(address), &organization.Address)
	if err != nil {
		return nil, err
	}

	return organization.ToEntity(), nil
}

// FindAll retrieves Organizations entities from the database.
func (r *OrganizationPostgreSQLRepository) FindAll(ctx context.Context, limit, page int) ([]*o.Organization, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var organizations []*o.Organization

	findAll := sqrl.Select("*").
		From(r.tableName).
		Where(sqrl.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		Limit(common.SafeIntToUint64(limit)).
		Offset(common.SafeIntToUint64((page - 1) * limit)).
		PlaceholderFormat(sqrl.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var organization o.OrganizationPostgreSQLModel

		var address string

		if err := rows.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
			&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
			&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			return nil, err
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return organizations, nil
}

// ListByIDs retrieves Organizations entities from the database using the provided IDs.
func (r *OrganizationPostgreSQLRepository) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*o.Organization, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var organizations []*o.Organization

	rows, err := db.QueryContext(ctx, `SELECT * FROM organization WHERE id = ANY($1) AND deleted_at IS NULL ORDER BY created_at DESC`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var organization o.OrganizationPostgreSQLModel

		var address string

		if err := rows.Scan(&organization.ID, &organization.ParentOrganizationID, &organization.LegalName,
			&organization.DoingBusinessAs, &organization.LegalDocument, &address, &organization.Status, &organization.StatusDescription,
			&organization.CreatedAt, &organization.UpdatedAt, &organization.DeletedAt); err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(address), &organization.Address)
		if err != nil {
			return nil, err
		}

		organizations = append(organizations, organization.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return organizations, nil
}

// Delete removes an Organization entity from the database using the provided ID.
func (r *OrganizationPostgreSQLRepository) Delete(ctx context.Context, id uuid.UUID) error {
	db, err := r.connection.GetDB()
	if err != nil {
		return err
	}

	result, err := db.ExecContext(ctx, `UPDATE organization SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(o.Organization{}).Name())
	}

	return nil
}
