package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"
	"strconv"
	"strings"
	"time"

	cn "github.com/LerianStudio/midaz/common/constant"

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
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_product")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &r.ProductPostgreSQLModel{}
	record.FromEntity(product)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "product_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert product record from entity to JSON string", err)

		return nil, err
	}

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
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(r.Product{}).Name())
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
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(r.Product{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create product. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByName find product from the database using Organization and Ledger id and Name.
func (p *ProductPostgreSQLRepository) FindByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_product_by_name")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_product_by_name.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM product WHERE organization_id = $1 AND ledger_id = $2 AND name LIKE $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, name)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := common.ValidateBusinessError(cn.ErrDuplicateProductName, reflect.TypeOf(r.Product{}).Name(), name, ledgerID)

		mopentelemetry.HandleSpanError(&span, "Failed to find product by name", err)

		return true, err
	}

	return false, nil
}

// FindAll retrieves Product entities from the database.
func (p *ProductPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*r.Product, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_products")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var products []*r.Product

	findAll := sqrl.Select("*").
		From(p.tableName).
		Where(sqrl.Expr("organization_id = ?", organizationID)).
		Where(sqrl.Expr("ledger_id = ?", ledgerID)).
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

		return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(r.Product{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var product r.ProductPostgreSQLModel
		if err := rows.Scan(&product.ID, &product.Name, &product.LedgerID, &product.OrganizationID,
			&product.Status, &product.StatusDescription, &product.CreatedAt, &product.UpdatedAt, &product.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		products = append(products, product.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return products, nil
}

// FindByIDs retrieves Products entities from the database using the provided IDs.
func (p *ProductPostgreSQLRepository) FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*r.Product, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_products_by_ids")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var products []*r.Product

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_products_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM product WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var product r.ProductPostgreSQLModel
		if err := rows.Scan(&product.ID, &product.Name, &product.LedgerID, &product.OrganizationID,
			&product.Status, &product.StatusDescription, &product.CreatedAt, &product.UpdatedAt, &product.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		products = append(products, product.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return products, nil
}

// Find retrieves a Product entity from the database using the provided ID.
func (p *ProductPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*r.Product, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_product")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	product := &r.ProductPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM product WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, id)

	spanQuery.End()

	if err := row.Scan(&product.ID, &product.Name, &product.LedgerID, &product.OrganizationID,
		&product.Status, &product.StatusDescription, &product.CreatedAt, &product.UpdatedAt, &product.DeletedAt); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(r.Product{}).Name())
		}

		return nil, err
	}

	return product.ToEntity(), nil
}

// Update a Product entity into Postgresql and returns the Product updated.
func (p *ProductPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, prd *r.Product) (*r.Product, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_product")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

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

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "product_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert product record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(r.Product{}).Name())
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
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(r.Product{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update product. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Product entity from the database using the provided IDs.
func (p *ProductPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_product")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE product SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute delete query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(r.Product{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to delete product. Rows affected is 0", err)

		return err
	}

	return nil
}
