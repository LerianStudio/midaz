package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LerianStudio/midaz/common"
	"reflect"
	"strconv"
	"strings"
	"time"

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

	_, err := c.connection.GetDB(context.Background())
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create inserts a new Organization entity into Postgresql and returns the created Organization.
func (r *OrganizationPostgreSQLRepository) Create(ctx context.Context, organization *o.Organization) (*o.Organization, error) {
	db, err := r.connection.GetDB(ctx)
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
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(o.Organization{}).Name(),
		}
	}

	return record.ToEntity(), nil
}

// Update an Organization entity into Postgresql and returns the Organization updated.
func (r *OrganizationPostgreSQLRepository) Update(ctx context.Context, id uuid.UUID, organization *o.Organization) (*o.Organization, error) {
	db, err := r.connection.GetDB(ctx)
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
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(o.Organization{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// Find retrieves an Organization entity from the database using the provided ID.
func (r *OrganizationPostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*o.Organization, error) {
	db, err := r.connection.GetDB(ctx)
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
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Organization{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
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
func (r *OrganizationPostgreSQLRepository) FindAll(ctx context.Context, limit int, id uuid.UUID) (*o.Pagination, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	var organizations []*o.Organization

	findAll := sqrl.Select("*").
		From(r.tableName).
		Where(sqrl.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		PlaceholderFormat(sqrl.Dollar)

	var previousPageToken string
	if id != uuid.Nil {
		findAll = findAll.Where(sqrl.Gt{"id": id})
		previousPageToken = id.String()
	}

	query, _, err := findAll.ToSql()

	rows, err := db.QueryContext(ctx, query, id)
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

	totalPages, err := r.countPages(ctx, limit)
	if err != nil {
		return nil, err
	}

	currentPage, err := r.currentPage(ctx, limit, id)
	if err != nil {
		return nil, err
	}

	var pagination = &o.Pagination{
		Organizations:     organizations,
		CurrentPage:       currentPage,
		TotalPages:        totalPages,
		NextPageToken:     &organizations[limit-1].ID,
		PreviousPageToken: &previousPageToken,
	}

	return pagination, nil
}

// ListByIDs retrieves Organizations entities from the database using the provided IDs.
func (r *OrganizationPostgreSQLRepository) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]*o.Organization, error) {
	db, err := r.connection.GetDB(ctx)
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
	db, err := r.connection.GetDB(ctx)
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
		return common.EntityNotFoundError{
			EntityType: reflect.TypeOf(o.Organization{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return nil
}

// countPages response total of count in the table
func (r *OrganizationPostgreSQLRepository) countPages(ctx context.Context, limit int) (int, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return 0, err
	}

	query, _, err := sqrl.Select(fmt.Sprintf("CEIL(COUNT(id) / %d.0) AS total_pages", limit)).
		From(r.tableName).
		Where(sqrl.Eq{"deleted_at": nil}).
		ToSql()
	if err != nil {
		return 0, err
	}

	var totalPages int
	err = db.QueryRowContext(ctx, query).Scan(&totalPages)
	if err != nil {
		return 0, err
	}

	return totalPages, nil
}

// currentPage response current page
func (r *OrganizationPostgreSQLRepository) currentPage(ctx context.Context, limit int, id uuid.UUID) (int, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return 0, err
	}

	recordsBefore := sqrl.Select("COUNT(id) AS records_before").
		From(r.tableName).
		Where(sqrl.Eq{"deleted_at": nil}).
		PlaceholderFormat(sqrl.Dollar)

	if id != uuid.Nil {
		recordsBefore = recordsBefore.Where(sqrl.Gt{"id": sqrl.Expr("CAST(? AS uuid)", id)})
	}

	query, _, err := sqrl.Select(fmt.Sprintf("CEIL(records_before / %d.0) AS current_page", limit)).
		FromSelect(recordsBefore, "records_before").
		Limit(uint64(limit)).
		ToSql()
	if err != nil {
		return 0, err
	}

	var current int
	err = db.QueryRowContext(ctx, query).Scan(&current)
	if err != nil {
		return 0, err
	}

	return current, nil
}
