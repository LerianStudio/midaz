package asset

import (
	"context"
	"database/sql"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Repository provides an interface for operations related to asset entities.
// It defines methods for creating, finding, updating, and deleting assets in the database.
type Repository interface {
	Create(ctx context.Context, asset *mmodel.Asset) (*mmodel.Asset, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Asset, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Asset, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error)
	FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, asset *mmodel.Asset) (*mmodel.Asset, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error)
}

// AssetPostgreSQLRepository is a Postgresql-specific implementation of the AssetRepository.
type AssetPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewAssetPostgreSQLRepository returns a new instance of AssetPostgreSQLRepository using the given Postgres connection.
func NewAssetPostgreSQLRepository(pc *libPostgres.PostgresConnection) *AssetPostgreSQLRepository {
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
func (r *AssetPostgreSQLRepository) Create(ctx context.Context, asset *mmodel.Asset) (*mmodel.Asset, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetPostgreSQLModel{}
	record.FromEntity(asset)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "asset_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert asset record from entity to JSON string", err)

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
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to create asset. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByNameOrCode retrieves Asset entities by name or code from the database.
func (r *AssetPostgreSQLRepository) FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_by_name_or_code")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_name_or_code.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND (name LIKE $3 OR code = $4) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, name, code)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrAssetNameOrCodeDuplicate, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Asset name or code already exists", err)

		return true, err
	}

	return false, nil
}

// FindAll retrieves Asset entities from the database with soft-deleted records.
func (r *AssetPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Asset, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_assets")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var assets []*mmodel.Asset

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDate(filter.StartDate, libPointers.Int(0))}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDate(filter.EndDate, libPointers.Int(0))}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var asset AssetPostgreSQLModel
		if err := rows.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
			&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		assets = append(assets, asset.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return assets, nil
}

// ListByIDs retrieves Assets entities from the database using the provided IDs.
func (r *AssetPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Asset, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_assets_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var assets []*mmodel.Asset

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_assets_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var asset AssetPostgreSQLModel
		if err := rows.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
			&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		assets = append(assets, asset.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return assets, nil
}

// Find retrieves an Asset entity from the database using the provided ID.
func (r *AssetPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	asset := &AssetPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)

	spanQuery.End()

	if err := row.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
		&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		return nil, err
	}

	return asset.ToEntity(), nil
}

// Update an Asset entity into Postgresql and returns the Asset updated.
func (r *AssetPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, asset *mmodel.Asset) (*mmodel.Asset, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetPostgreSQLModel{}
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

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "asset_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert asset record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to update asset. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes an Asset entity from the database using the provided IDs.
func (r *AssetPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE asset SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute delete query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to delete asset. Rows affected is 0", err)

		return err
	}

	return nil
}

// Count retrieves the total count of Asset entities from the database.
func (r *AssetPostgreSQLRepository) Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_assets")
	defer span.End()

	var count = int64(0)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return count, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND deleted_at IS NULL",
		organizationID, ledgerID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return count, err
	}

	return count, nil
}
