package assetrate

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

var assetRateColumnList = []string{
	"id",
	"organization_id",
	"ledger_id",
	"external_id",
	`"from"`,
	`"to"`,
	"rate",
	"rate_scale",
	"source",
	"ttl",
	"created_at",
	"updated_at",
}

const (
	updateFieldOffset     = 1
	rateFieldOffset       = 2
	rateScaleFieldOffset  = 3
	ttlFieldOffset        = 4
	externalIDFieldOffset = 5
	whereClauseOffset     = 2
)

// Repository provides an interface for asset_rate template entities.
// It defines methods for creating, finding, and updating asset rates.
type Repository interface {
	Create(ctx context.Context, assetRate *mmodel.AssetRate) (*mmodel.AssetRate, error)
	FindByCurrencyPair(ctx context.Context, organizationID, ledgerID uuid.UUID, from, to string) (*mmodel.AssetRate, error)
	FindByExternalID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AssetRate, error)
	FindAllByAssetCodes(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, toAssetCodes []string, filter http.Pagination) ([]*mmodel.AssetRate, libHTTP.CursorPagination, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, assetRate *mmodel.AssetRate) (*mmodel.AssetRate, error)
}

// AssetRatePostgreSQLRepository is a Postgresql-specific implementation of the AssetRateRepository.
type AssetRatePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	wrapper    *mmigration.MigrationWrapper // For future health checks
	tableName  string
}

// NewAssetRatePostgreSQLRepository returns a new instance of AssetRatePostgreSQLRepository using the given MigrationWrapper.
func NewAssetRatePostgreSQLRepository(mw *mmigration.MigrationWrapper) *AssetRatePostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "AssetRatePostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "AssetRatePostgreSQLRepository")

	return &AssetRatePostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "asset_rate",
	}
}

// Create a new AssetRate entity into Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) Create(ctx context.Context, assetRate *mmodel.AssetRate) (*mmodel.AssetRate, error) {
	assert.NotNil(assetRate, "assetRate entity must not be nil for Create",
		"repository", "AssetRatePostgreSQLRepository")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	record := &AssetRatePostgreSQLModel{}
	record.FromEntity(assetRate)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	result, err := db.ExecContext(ctx, `INSERT INTO asset_rate VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *`,
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.ExternalID,
		&record.From,
		&record.To,
		&record.Rate,
		&record.RateScale,
		&record.Source,
		&record.TTL,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		logger.Errorf("Failed to execute insert query: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.AssetRate{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create asset rate. Rows affected is 0", err)

		logger.Errorf("Failed to create asset rate. Rows affected is 0: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	return record.ToEntity(), nil
}

// FindByExternalID an AssetRate entity by its external ID in Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) FindByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*mmodel.AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_rate_by_external_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	record := &AssetRatePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	findQuery := squirrel.Select(assetRateColumnList...).
		From("asset_rate").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"external_id": externalID}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.ExternalID,
		&record.From,
		&record.To,
		&record.Rate,
		&record.RateScale,
		&record.Source,
		&record.TTL,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.AssetRate{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find asset rate. Row not found", err)

			logger.Errorf("Failed to find asset rate. Row not found: %v", err)

			return nil, pkg.ValidateInternalError(err, "AssetRate")
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		logger.Errorf("Failed to scan asset rate record: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	return record.ToEntity(), nil
}

// FindByCurrencyPair an AssetRate entity by its currency pair in Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) FindByCurrencyPair(ctx context.Context, organizationID, ledgerID uuid.UUID, from, to string) (*mmodel.AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_rate_by_currency_pair")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	record := &AssetRatePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	findQuery := squirrel.Select(assetRateColumnList...).
		From("asset_rate").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr(`"from" = ?`, from)).
		Where(squirrel.Expr(`"to" = ?`, to)).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		spanQuery.End()

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.ExternalID,
		&record.From,
		&record.To,
		&record.Rate,
		&record.RateScale,
		&record.Source,
		&record.TTL,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Errorf("Asset rate not found: %v", err)

			return nil, nil
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		logger.Errorf("Failed to scan asset rate record: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	return record.ToEntity(), nil
}

// FindAllByAssetCodes returns all asset rates by asset codes.
func (r *AssetRatePostgreSQLRepository) FindAllByAssetCodes(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, toAssetCodes []string, filter http.Pagination) ([]*mmodel.AssetRate, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_asset_rates_by_asset_codes")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "AssetRate")
	}

	decodedCursor := libHTTP.Cursor{}
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !isFirstPage {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "AssetRate")
		}
	}

	query, args, err := r.buildAssetRateFindAllQuery(organizationID, ledgerID, fromAssetCode, toAssetCodes, filter, decodedCursor, orderDirection)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "AssetRate")
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "AssetRate")
	}
	defer rows.Close()

	spanQuery.End()

	assetRates, err := r.scanAssetRateRows(rows)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		logger.Errorf("Failed to scan rows: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "AssetRate")
	}

	hasPagination := len(assetRates) > filter.Limit

	assetRates = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, assetRates, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(assetRates) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, assetRates[0].ID, assetRates[len(assetRates)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, "AssetRate")
		}
	}

	return assetRates, cur, nil
}

// Update an AssetRate entity into Postgresql and returns the AssetRate updated.
func (r *AssetRatePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, assetRate *mmodel.AssetRate) (*mmodel.AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	record := &AssetRatePostgreSQLModel{}
	record.FromEntity(assetRate)

	var updates []string

	var args []any

	if !libCommons.IsNilOrEmpty(assetRate.Source) {
		updates = append(updates, "source = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Source)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates,
		"updated_at = $"+strconv.Itoa(len(args)+updateFieldOffset),
		"rate = $"+strconv.Itoa(len(args)+rateFieldOffset),
		"rate_scale = $"+strconv.Itoa(len(args)+rateScaleFieldOffset),
		"ttl = $"+strconv.Itoa(len(args)+ttlFieldOffset),
		"external_id = $"+strconv.Itoa(len(args)+externalIDFieldOffset),
	)

	args = append(args, record.UpdatedAt, record.Rate, record.RateScale, record.TTL, record.ExternalID, organizationID, ledgerID, id)

	query := `UPDATE asset_rate SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-whereClauseOffset) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args))

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.AssetRate{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update asset rate. Rows affected is 0", err)

		logger.Warnf("Failed to update asset rate. Rows affected is 0: %v", err)

		return nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	return record.ToEntity(), nil
}

// scanAssetRateRows scans rows and returns asset rates
func (r *AssetRatePostgreSQLRepository) scanAssetRateRows(rows *sql.Rows) ([]*mmodel.AssetRate, error) {
	assetRates := make([]*mmodel.AssetRate, 0)

	for rows.Next() {
		var assetRate AssetRatePostgreSQLModel
		if err := rows.Scan(
			&assetRate.ID,
			&assetRate.OrganizationID,
			&assetRate.LedgerID,
			&assetRate.ExternalID,
			&assetRate.From,
			&assetRate.To,
			&assetRate.Rate,
			&assetRate.RateScale,
			&assetRate.Source,
			&assetRate.TTL,
			&assetRate.CreatedAt,
			&assetRate.UpdatedAt,
		); err != nil {
			return nil, pkg.ValidateInternalError(err, "AssetRate")
		}

		assetRates = append(assetRates, assetRate.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return assetRates, pkg.ValidateInternalError(err, "AssetRate")
	}

	return assetRates, nil
}

// buildAssetRateFindAllQuery builds the SQL query for finding asset rates
func (r *AssetRatePostgreSQLRepository) buildAssetRateFindAllQuery(organizationID, ledgerID uuid.UUID, fromAssetCode string, toAssetCodes []string, filter http.Pagination, decodedCursor libHTTP.Cursor, orderDirection string) (string, []any, error) {
	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr(`"from" = ?`, fromAssetCode)).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	if toAssetCodes != nil {
		findAll = findAll.Where(squirrel.Eq{`"to"`: toAssetCodes})
	}

	findAll, _ = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		return "", nil, pkg.ValidateInternalError(err, "AssetRate")
	}

	return query, args, nil
}
