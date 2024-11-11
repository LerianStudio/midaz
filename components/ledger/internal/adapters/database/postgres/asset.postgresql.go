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
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// AssetPostgreSQLRepository is a Postgresql-specific implementation of the AssetRepository.
type AssetPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewAssetPostgreSQLRepository returns a new instance of AssetPostgreSQLRepository using the given Postgres connection.
func NewAssetPostgreSQLRepository(pc *mpostgres.PostgresConnection) *AssetPostgreSQLRepository {
	c := &AssetPostgreSQLRepository{
		connection: pc,
		tableName:  "asset",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new asset entity into Postgresql and returns it.
func (r *AssetPostgreSQLRepository) Create(ctx context.Context, asset *s.Asset) (*s.Asset, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &s.AssetPostgreSQLModel{}
	record.FromEntity(asset)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "asset_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert asset record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO asset VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *`,
		record.ID,
		record.Name,
		record.Type,
		record.Code,
		record.Status,
		record.StatusDescription,
		record.LedgerID,
		record.OrganizationID,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(s.Asset{}).Name())
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
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(s.Asset{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create asset. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByNameOrCode retrieves Asset entities by name or code from the database.
func (r *AssetPostgreSQLRepository) FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_by_name_or_code")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_name_or_code.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND (name LIKE $3 OR code = $4) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, name, code)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := common.ValidateBusinessError(cn.ErrAssetNameOrCodeDuplicate, reflect.TypeOf(s.Asset{}).Name())

		mopentelemetry.HandleSpanError(&span, "Asset name or code already exists", err)

		return true, err
	}

	return false, nil
}

// FindAll retrieves Asset entities from the database.
func (r *AssetPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*s.Asset, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_assets")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var assets []*s.Asset

	findAll := sqrl.Select("*").
		From(r.tableName).
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

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var asset s.AssetPostgreSQLModel
		if err := rows.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
			&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		assets = append(assets, asset.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return assets, nil
}

// ListByIDs retrieves Assets entities from the database using the provided IDs.
func (r *AssetPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*s.Asset, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_assets_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var assets []*s.Asset

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_assets_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var asset s.AssetPostgreSQLModel
		if err := rows.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
			&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		assets = append(assets, asset.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return assets, nil
}

// Find retrieves an Asset entity from the database using the provided ID.
func (r *AssetPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*s.Asset, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	asset := &s.AssetPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)

	spanQuery.End()

	if err := row.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
		&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(s.Asset{}).Name())
		}

		return nil, err
	}

	return asset.ToEntity(), nil
}

// Update an Asset entity into Postgresql and returns the Asset updated.
func (r *AssetPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, asset *s.Asset) (*s.Asset, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &s.AssetPostgreSQLModel{}
	record.FromEntity(asset)

	var updates []string

	var args []any

	if asset.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if !asset.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE asset SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "asset_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert asset record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(s.Asset{}).Name())
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
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(s.Asset{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update asset. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes an Asset entity from the database using the provided IDs.
func (r *AssetPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE asset SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
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
		err := common.ValidateBusinessError(cn.ErrEntityNotFound, reflect.TypeOf(s.Asset{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to delete asset. Rows affected is 0", err)

		return err
	}

	return nil
}
