package assetrate

import (
	"context"
	"database/sql"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Repository provides an interface for asset_rate template entities.
// It defines methods for creating, finding, and updating asset rates.
type Repository interface {
	Create(ctx context.Context, assetRate *AssetRate) (*AssetRate, error)
	FindByCurrencyPair(ctx context.Context, organizationID, ledgerID uuid.UUID, from, to string) (*AssetRate, error)
	FindByExternalID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*AssetRate, error)
	FindAllByAssetCodes(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, toAssetCodes []string, filter http.Pagination) ([]*AssetRate, libHTTP.CursorPagination, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, assetRate *AssetRate) (*AssetRate, error)
}

// AssetRatePostgreSQLRepository is a Postgresql-specific implementation of the AssetRateRepository.
type AssetRatePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewAssetRatePostgreSQLRepository returns a new instance of AssetRatePostgreSQLRepository using the given Postgres connection.
func NewAssetRatePostgreSQLRepository(pc *libPostgres.PostgresConnection) *AssetRatePostgreSQLRepository {
	c := &AssetRatePostgreSQLRepository{
		connection: pc,
		tableName:  "asset_rate",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new AssetRate entity into Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) Create(ctx context.Context, assetRate *AssetRate) (*AssetRate, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}
	record.FromEntity(assetRate)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "asset_rate_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert asset_rate record from entity to JSON string", err)

		return nil, err
	}

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

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to create asset rate. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByExternalID an AssetRate entity by its external ID in Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) FindByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*AssetRate, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_rate_by_external_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, `SELECT * FROM asset_rate WHERE organization_id = $1 AND ledger_id = $2 AND external_id = $3 ORDER BY created_at DESC`, organizationID, ledgerID, externalID)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())
		}

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByCurrencyPair an AssetRate entity by its currency pair in Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) FindByCurrencyPair(ctx context.Context, organizationID, ledgerID uuid.UUID, from, to string) (*AssetRate, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_rate_by_currency_pair")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, `SELECT * FROM asset_rate WHERE organization_id = $1 AND ledger_id = $2 AND "from" = $3 AND "to" = $4 ORDER BY created_at DESC`, organizationID, ledgerID, from, to)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindAllByAssetCodes returns all asset rates by asset codes.
func (r *AssetRatePostgreSQLRepository) FindAllByAssetCodes(ctx context.Context, organizationID, ledgerID uuid.UUID, fromAssetCode string, toAssetCodes []string, filter http.Pagination) ([]*AssetRate, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_asset_rates_by_asset_codes")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	assetRates := make([]*AssetRate, 0)

	decodedCursor := libHTTP.Cursor{}
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !isFirstPage {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr(`"from" = ?`, fromAssetCode)).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	if toAssetCodes != nil {
		findAll.Where(squirrel.Eq{`"to"`: toAssetCodes})
	}

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

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
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		assetRates = append(assetRates, assetRate.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(assetRates) > filter.Limit

	assetRates = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, assetRates, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(assetRates) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, assetRates[0].ID, assetRates[len(assetRates)-1].ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return assetRates, cur, nil
}

// Update an AssetRate entity into Postgresql and returns the AssetRate updated.
func (r *AssetRatePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, assetRate *AssetRate) (*AssetRate, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
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
		"updated_at = $"+strconv.Itoa(len(args)+1),
		"rate = $"+strconv.Itoa(len(args)+2),
		"rate_scale = $"+strconv.Itoa(len(args)+3),
		"ttl = $"+strconv.Itoa(len(args)+4),
		"external_id = $"+strconv.Itoa(len(args)+5),
	)

	args = append(args, record.UpdatedAt, record.Rate, record.RateScale, record.TTL, record.ExternalID, organizationID, ledgerID, id)

	query := `UPDATE asset_rate SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args))

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "asset_rate_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert asset rate record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to update asset rate. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}
