// Package assetrate provides PostgreSQL adapter implementations for asset rate management.
// It contains database models, input/output types, and repository methods for
// storing and retrieving currency conversion rates between assets.
package assetrate

import (
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// Type aliases for backward compatibility with code that imports from this package.
// These aliases point to the canonical types in mmodel package.
type (
	// AssetRate is an alias to mmodel.AssetRate for backward compatibility
	AssetRate = mmodel.AssetRate
	// CreateAssetRateInput is an alias to mmodel.CreateAssetRateInput for backward compatibility
	CreateAssetRateInput = mmodel.CreateAssetRateInput
)

// AssetRatePostgreSQLModel represents the entity AssetRatePostgreSQLModel into SQL context in Database
//
// @Description Database model for storing asset rate information in PostgreSQL
type AssetRatePostgreSQLModel struct {
	ID             string          // Unique identifier (UUID format)
	OrganizationID string          // Organization that owns this asset rate
	LedgerID       string          // Ledger containing this asset rate
	ExternalID     string          // External identifier for integration
	From           string          // Source asset code
	To             string          // Target asset code
	Rate           decimal.Decimal // Conversion rate value
	RateScale      int             // Decimal places for the rate
	Source         *string         // Source of rate information (e.g., "External System")
	TTL            int             // Time-to-live in seconds
	CreatedAt      time.Time       // Timestamp when created
	UpdatedAt      time.Time       // Timestamp when last updated
	Metadata       map[string]any  // Additional custom attributes
}

// ToEntity converts an AssetRatePostgreSQLModel to entity AssetRate
func (a *AssetRatePostgreSQLModel) ToEntity() *mmodel.AssetRate {
	assetRate := &mmodel.AssetRate{
		ID:             a.ID,
		OrganizationID: a.OrganizationID,
		LedgerID:       a.LedgerID,
		ExternalID:     a.ExternalID,
		From:           a.From,
		To:             a.To,
		Rate:           a.Rate,
		Scale:          a.RateScale,
		Source:         a.Source,
		TTL:            a.TTL,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}

	return assetRate
}

// FromEntity converts an entity AssetRate to AssetRatePostgreSQLModel
func (a *AssetRatePostgreSQLModel) FromEntity(assetRate *mmodel.AssetRate) {
	*a = AssetRatePostgreSQLModel{
		ID:             assetRate.ID,
		OrganizationID: assetRate.OrganizationID,
		LedgerID:       assetRate.LedgerID,
		ExternalID:     assetRate.ExternalID,
		From:           assetRate.From,
		To:             assetRate.To,
		Rate:           assetRate.Rate,
		RateScale:      assetRate.Scale,
		Source:         assetRate.Source,
		TTL:            assetRate.TTL,
		CreatedAt:      assetRate.CreatedAt,
		UpdatedAt:      assetRate.UpdatedAt,
	}
}
