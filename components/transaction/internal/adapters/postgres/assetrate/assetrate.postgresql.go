package assetrate

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

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

	result, err := db.ExecContext(ctx, `INSERT INTO asset_rate VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *`,
		record.ID,
		record.BaseAssetCode,
		record.CounterAssetCode,
		record.Amount,
		record.Scale,
		record.Source,
		record.OrganizationID,
		record.LedgerID,
		record.CreatedAt,
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

	row := db.QueryRowContext(ctx, `SELECT * FROM asset_rate WHERE id = $1 AND organization_id = $2 AND ledger_id = $3`, assetRateID, organizationID, ledgerID)

	spanQuery.End()

	if err := row.Scan(
		&record.ID,
		&record.BaseAssetCode,
		&record.CounterAssetCode,
		&record.Amount,
		&record.Scale,
		&record.Source,
		&record.OrganizationID,
		&record.LedgerID,
		&record.CreatedAt,
	); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan asset rate record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(AssetRate{}).Name())
		}

		return nil, err
	}

	return record.ToEntity(), nil
}
