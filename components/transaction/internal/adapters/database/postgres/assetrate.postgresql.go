package postgres

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	a "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
)

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
func (r *AssetRatePostgreSQLRepository) Create(ctx context.Context, assetRate *a.AssetRate) (*a.AssetRate, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &a.AssetRatePostgreSQLModel{}
	record.FromEntity(assetRate)

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
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(a.AssetRate{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}
