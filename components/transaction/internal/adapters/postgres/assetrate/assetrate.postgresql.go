package assetrate

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"

	"github.com/google/uuid"
)

// Repository provides an interface for asset_rate template entities.
//
//go:generate mockgen --destination=assetrate.mock.go --package=assetrate . Repository
type Repository interface {
	Create(ctx context.Context, assetRate *AssetRate) (*AssetRate, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*AssetRate, error)
	FindByCurrencyPair(ctx context.Context, organizationID, ledgerID uuid.UUID, from, to string) (*AssetRate, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, assetRate *AssetRate) (*AssetRate, error)
}

// AssetRatePostgreSQLRepository is a Postgresql-specific implementation of the AssetRateRepository.
type AssetRatePostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewAssetRatePostgreSQLRepository returns a new instance of AssetRatePostgreSQLRepository using the given Postgres connection.
func NewAssetRatePostgreSQLRepository(pc *mpostgres.PostgresConnection) *AssetRatePostgreSQLRepository {
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
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}
	record.FromEntity(assetRate)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "asset_rate_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert asset_rate record from entity to JSON string", err)

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
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create asset rate. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Find an AssetRate entity by its ID in Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, assetRateID uuid.UUID) (*AssetRate, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, `SELECT * FROM asset_rate WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 ORDER BY created_at DESC`, organizationID, ledgerID, assetRateID)

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
		mopentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())
		}

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByCurrencyPair an AssetRate entity by its currency pair in Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) FindByCurrencyPair(ctx context.Context, organizationID, ledgerID uuid.UUID, from, to string) (*AssetRate, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

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
		mopentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return record.ToEntity(), nil
}

// Update an AssetRate entity into Postgresql and returns the AssetRate updated.
func (r *AssetRatePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, assetRate *AssetRate) (*AssetRate, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_asset_rate")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetRatePostgreSQLModel{}
	record.FromEntity(assetRate)

	var updates []string

	var args []any

	if !pkg.IsNilOrEmpty(assetRate.Source) {
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

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "asset_rate_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert asset rate record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update asset rate. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}
