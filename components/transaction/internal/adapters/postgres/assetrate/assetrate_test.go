package assetrate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetRatePostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		source := "Central Bank"

		model := &AssetRatePostgreSQLModel{
			ID:             "assetrate-123",
			OrganizationID: "org-456",
			LedgerID:       "ledger-789",
			ExternalID:     "ext-012",
			From:           "USD",
			To:             "BRL",
			Rate:           5.25,
			RateScale:      2.0,
			Source:         &source,
			TTL:            3600,
			CreatedAt:      time.Now().Add(-48 * time.Hour),
			UpdatedAt:      time.Now().Add(-1 * time.Hour),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.ExternalID, entity.ExternalID)
		assert.Equal(t, model.From, entity.From)
		assert.Equal(t, model.To, entity.To)
		assert.Equal(t, model.Rate, entity.Rate)
		require.NotNil(t, entity.Scale, "Scale should be converted to pointer")
		assert.Equal(t, model.RateScale, *entity.Scale)
		assert.Equal(t, model.Source, entity.Source)
		assert.Equal(t, model.TTL, entity.TTL)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		model := &AssetRatePostgreSQLModel{
			ID:             "assetrate-456",
			OrganizationID: "org-123",
			LedgerID:       "ledger-456",
			From:           "EUR",
			To:             "USD",
			Rate:           1.08,
			RateScale:      0.0,
			TTL:            0,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Empty(t, entity.ExternalID)
		assert.Equal(t, model.From, entity.From)
		assert.Equal(t, model.To, entity.To)
		assert.Equal(t, model.Rate, entity.Rate)
		require.NotNil(t, entity.Scale)
		assert.Equal(t, model.RateScale, *entity.Scale)
		assert.Nil(t, entity.Source)
		assert.Equal(t, 0, entity.TTL)
	})

	t.Run("with_zero_rate_scale", func(t *testing.T) {
		// Verifies that zero RateScale is correctly converted to Scale pointer
		model := &AssetRatePostgreSQLModel{
			ID:             "assetrate-zero",
			OrganizationID: "org-zero",
			LedgerID:       "ledger-zero",
			From:           "BTC",
			To:             "USD",
			Rate:           50000.0,
			RateScale:      0.0,
			TTL:            7200,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		require.NotNil(t, entity.Scale, "Scale should not be nil even when RateScale is 0")
		assert.Equal(t, 0.0, *entity.Scale)
	})
}

func TestAssetRatePostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		source := "External API"
		scale := 2.0

		entity := &AssetRate{
			ID:             "should-be-overwritten",
			OrganizationID: "org-456",
			LedgerID:       "ledger-789",
			ExternalID:     "ext-012",
			From:           "USD",
			To:             "BRL",
			Rate:           5.25,
			Scale:          &scale,
			Source:         &source,
			TTL:            3600,
			CreatedAt:      time.Now().Add(-48 * time.Hour),
			UpdatedAt:      time.Now().Add(-1 * time.Hour),
		}

		var model AssetRatePostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated")
		assert.NotEqual(t, entity.ID, model.ID, "ID should be newly generated, not copied from entity")
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.ExternalID, model.ExternalID)
		assert.Equal(t, entity.From, model.From)
		assert.Equal(t, entity.To, model.To)
		assert.Equal(t, entity.Rate, model.Rate)
		assert.Equal(t, *entity.Scale, model.RateScale)
		assert.Equal(t, entity.Source, model.Source)
		assert.Equal(t, entity.TTL, model.TTL)
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.Nil(t, model.Metadata, "Metadata is not mapped by FromEntity")
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		scale := 0.0

		entity := &AssetRate{
			OrganizationID: "org-123",
			LedgerID:       "ledger-456",
			From:           "EUR",
			To:             "USD",
			Rate:           1.08,
			Scale:          &scale,
			TTL:            0,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model AssetRatePostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated")
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Empty(t, model.ExternalID)
		assert.Equal(t, entity.From, model.From)
		assert.Equal(t, entity.To, model.To)
		assert.Equal(t, entity.Rate, model.Rate)
		assert.Equal(t, *entity.Scale, model.RateScale)
		assert.Nil(t, model.Source)
		assert.Equal(t, 0, model.TTL)
	})

	t.Run("generates_uuid_v7", func(t *testing.T) {
		scale := 2.0

		entity := &AssetRate{
			OrganizationID: "org-uuid",
			LedgerID:       "ledger-uuid",
			From:           "GBP",
			To:             "EUR",
			Rate:           1.17,
			Scale:          &scale,
			TTL:            1800,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model1 AssetRatePostgreSQLModel
		var model2 AssetRatePostgreSQLModel
		model1.FromEntity(entity)
		model2.FromEntity(entity)

		assert.NotEmpty(t, model1.ID)
		assert.NotEmpty(t, model2.ID)
		assert.NotEqual(t, model1.ID, model2.ID, "Each call should generate a unique ID")
		assert.Len(t, model1.ID, 36, "ID should be a valid UUID string (36 chars with hyphens)")
	})
}
