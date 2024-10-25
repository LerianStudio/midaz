package postgres

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	ar "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/google/uuid"
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
func (r *AssetRatePostgreSQLRepository) Create(ctx context.Context, assetRate *ar.AssetRate) (*ar.AssetRate, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &ar.AssetRatePostgreSQLModel{}
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
			EntityType: reflect.TypeOf(ar.AssetRate{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// Find an AssetRate entity by its ID in Postgresql and returns it.
func (r *AssetRatePostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, assetRateID uuid.UUID) (*ar.AssetRate, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &ar.AssetRatePostgreSQLModel{}

	row := db.QueryRowContext(ctx, `SELECT * FROM asset_rate WHERE id = $1 AND organization_id = $2 AND ledger_id = $3`, assetRateID, organizationID, ledgerID)
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(o.Operation{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
		}

		return nil, err
	}

	return record.ToEntity(), nil
}
