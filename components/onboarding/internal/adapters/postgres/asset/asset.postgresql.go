package asset

import (
	"context"
	"database/sql"
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
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

var assetColumnList = []string{
	"id",
	"name",
	"type",
	"code",
	"status",
	"status_description",
	"ledger_id",
	"organization_id",
	"created_at",
	"updated_at",
	"deleted_at",
}

const (
	// argsOffsetForWhereClause represents the offset for WHERE clause arguments in SQL queries
	argsOffsetForWhereClause = 2
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
	assert.NotNil(pc, "PostgreSQL connection must not be nil", "repository", "AssetPostgreSQLRepository")

	db, err := pc.GetDB()
	assert.NoError(err, "database connection required for AssetPostgreSQLRepository",
		"repository", "AssetPostgreSQLRepository")
	assert.NotNil(db, "database handle must not be nil", "repository", "AssetPostgreSQLRepository")

	return &AssetPostgreSQLRepository{
		connection: pc,
		tableName:  "asset",
	}
}

// Create a new asset entity into Postgresql and returns it.
func (r *AssetPostgreSQLRepository) Create(ctx context.Context, asset *mmodel.Asset) (*mmodel.Asset, error) {
	assert.NotNil(asset, "asset entity must not be nil for Create",
		"repository", "AssetPostgreSQLRepository")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	record := &AssetPostgreSQLModel{}
	record.FromEntity(asset)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			validatedErr := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute insert query", validatedErr)

			return nil, validatedErr
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	if rowsAffected == 0 {
		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset. Rows affected is 0", notFoundErr)

		return nil, notFoundErr
	}

	return record.ToEntity(), nil
}

// FindByNameOrCode retrieves Asset entities by name or code from the database.
func (r *AssetPostgreSQLRepository) FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_by_name_or_code")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return false, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_name_or_code.query")

	query, args, err := squirrel.Select(assetColumnList...).
		From("asset").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Or{squirrel.Expr("name LIKE ?", name), squirrel.Eq{"code": code}}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		spanQuery.End()

		return false, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return false, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		// Scan the actual duplicate asset's name and code for better error reporting
		var foundID, foundName, foundType, foundCode string
		var foundStatus, foundStatusDesc sql.NullString
		var foundLedgerID, foundOrgID string
		var foundCreatedAt, foundUpdatedAt time.Time
		var foundDeletedAt sql.NullTime

		scanErr := rows.Scan(
			&foundID, &foundName, &foundType, &foundCode,
			&foundStatus, &foundStatusDesc,
			&foundLedgerID, &foundOrgID,
			&foundCreatedAt, &foundUpdatedAt, &foundDeletedAt,
		)
		if scanErr != nil {
			// If scan fails, log with search parameters as fallback
			logger.Warnf("Asset name or code duplicate found (search params: name=%s, code=%s)", name, code)
		} else {
			logger.Warnf("Asset name or code duplicate found: existing_asset_name=%s, existing_asset_code=%s (search params: name=%s, code=%s)",
				foundName, foundCode, name, code)
		}

		err := pkg.ValidateBusinessError(constant.ErrAssetNameOrCodeDuplicate, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Asset name or code already exists", err)

		return true, err
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Row iteration error", err)

		logger.Errorf("Row iteration error: %v", err)

		return false, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	return false, nil
}

// FindAll retrieves Asset entities from the database with soft-deleted records.
func (r *AssetPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_assets")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	var assets []*mmodel.Asset

	findAll := squirrel.Select(assetColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
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

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var asset AssetPostgreSQLModel
		if err := rows.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
			&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		assets = append(assets, asset.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		logger.Errorf("Failed to scan rows: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	return assets, nil
}

// ListByIDs retrieves Assets entities from the database using the provided IDs.
func (r *AssetPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_assets_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	var assets []*mmodel.Asset

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_assets_by_ids.query")

	query, args, err := squirrel.Select(assetColumnList...).
		From("asset").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var asset AssetPostgreSQLModel
		if err := rows.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
			&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		assets = append(assets, asset.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		logger.Errorf("Failed to scan rows: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	return assets, nil
}

// Find retrieves an Asset entity from the database using the provided ID.
func (r *AssetPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	asset := &AssetPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	query, args, err := squirrel.Select(assetColumnList...).
		From("asset").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
		&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	return asset.ToEntity(), nil
}

// Update an Asset entity into Postgresql and returns the Asset updated.
func (r *AssetPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, asset *mmodel.Asset) (*mmodel.Asset, error) {
	assert.NotNil(asset, "asset entity must not be nil for Update",
		"organization_id", organizationID,
		"ledger_id", ledgerID,
		"asset_id", id)

	// Ensure FromEntity preserves the correct ID for Update operations
	asset.ID = id.String()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
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
		` WHERE organization_id = $` + strconv.Itoa(len(args)-argsOffsetForWhereClause) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			validatedErr := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", validatedErr)

			logger.Warnf("Failed to execute update query: %v", validatedErr)

			return nil, validatedErr
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	if rowsAffected == 0 {
		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update asset. Rows affected is 0", notFoundErr)

		return nil, notFoundErr
	}

	return record.ToEntity(), nil
}

// Delete removes an Asset entity from the database using the provided IDs.
func (r *AssetPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_asset")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE asset SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute delete query", err)

		logger.Errorf("Failed to execute delete query: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Asset{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete asset. Rows affected is 0", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	return nil
}

// Count retrieves the total count of Asset entities from the database.
func (r *AssetPostgreSQLRepository) Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_assets")
	defer span.End()

	count := int64(0)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return count, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND deleted_at IS NULL",
		organizationID, ledgerID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return count, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
	}

	return count, nil
}
