package postgres

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// ProductPostgreSQLRepository is a Postgresql-specific implementation of the Repository.
type ProductPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewProductPostgreSQLRepository returns a new instance of ProductPostgreSQLRepository using the given Postgres connection.
func NewProductPostgreSQLRepository(pc *mpostgres.PostgresConnection) *ProductPostgreSQLRepository {
	c := &ProductPostgreSQLRepository{
		connection: pc,
		tableName:  "product",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new product entity into Postgresql and returns it.
func (p *ProductPostgreSQLRepository) Create(ctx context.Context, product *r.Product) (*r.Product, error) {
	db, err := p.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &r.ProductPostgreSQLModel{}
	record.FromEntity(product)

	result, err := db.ExecContext(ctx, `INSERT INTO product VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *`,
		record.ID,
		record.Name,
		record.LedgerID,
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
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(r.Product{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(r.Product{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// FindByName find product from the database using Organization and Ledger id and Name.
func (p *ProductPostgreSQLRepository) FindByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error) {
	db, err := p.connection.GetDB()
	if err != nil {
		return false, err
	}

	rows, err := db.QueryContext(ctx, "SELECT * FROM product WHERE organization_id = $1 AND ledger_id = $2 AND name LIKE $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, name)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if rows.Next() {
		return true, common.EntityConflictError{
			EntityType: reflect.TypeOf(r.Product{}).Name(),
			Title:      "Entity found.",
			Code:       "0008",
			Message:    "Entity was found matching by the provided Name. Ensure the another Name is being used for the entity you are attempting to manage.",
		}
	}

	return false, nil
}

// FindAll retrieves Product entities from the database.
func (p *ProductPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*r.Product, error) {
	db, err := p.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var products []*r.Product

	findAll := sqrl.Select("*").
		From(p.tableName).
		Where(sqrl.Expr("organization_id = ?", organizationID)).
		Where(sqrl.Expr("ledger_id = ?", ledgerID)).
		Where(sqrl.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		Offset(uint64((page - 1) * limit)).
		PlaceholderFormat(sqrl.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(r.Product{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}
	defer rows.Close()

	for rows.Next() {
		var product r.ProductPostgreSQLModel
		if err := rows.Scan(&product.ID, &product.Name, &product.LedgerID, &product.OrganizationID,
			&product.Status, &product.StatusDescription, &product.CreatedAt, &product.UpdatedAt, &product.DeletedAt); err != nil {
			return nil, err
		}

		products = append(products, product.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

// FindByIDs retrieves Products entities from the database using the provided IDs.
func (p *ProductPostgreSQLRepository) FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*r.Product, error) {
	db, err := p.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var products []*r.Product

	rows, err := db.QueryContext(ctx, "SELECT * FROM product WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var product r.ProductPostgreSQLModel
		if err := rows.Scan(&product.ID, &product.Name, &product.LedgerID, &product.OrganizationID,
			&product.Status, &product.StatusDescription, &product.CreatedAt, &product.UpdatedAt, &product.DeletedAt); err != nil {
			return nil, err
		}

		products = append(products, product.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

// Find retrieves a Product entity from the database using the provided ID.
func (p *ProductPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*r.Product, error) {
	db, err := p.connection.GetDB()
	if err != nil {
		return nil, err
	}

	product := &r.ProductPostgreSQLModel{}

	row := db.QueryRowContext(ctx, "SELECT * FROM product WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, id)
	if err := row.Scan(&product.ID, &product.Name, &product.LedgerID, &product.OrganizationID,
		&product.Status, &product.StatusDescription, &product.CreatedAt, &product.UpdatedAt, &product.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(r.Product{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
		}

		return nil, err
	}

	return product.ToEntity(), nil
}

// Update a Product entity into Postgresql and returns the Product updated.
func (p *ProductPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, prd *r.Product) (*r.Product, error) {
	db, err := p.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &r.ProductPostgreSQLModel{}
	record.FromEntity(prd)

	var updates []string

	var args []any

	if prd.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if !prd.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE product SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(r.Product{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(r.Product{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// Delete removes a Product entity from the database using the provided IDs.
func (p *ProductPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	db, err := p.connection.GetDB()
	if err != nil {
		return err
	}

	result, err := db.ExecContext(ctx, `UPDATE product SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return common.EntityNotFoundError{
			EntityType: reflect.TypeOf(r.Product{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return nil
}
