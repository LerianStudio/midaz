package asset

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetPostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		statusDesc := "Active currency"
		deletedAt := time.Now().Add(-24 * time.Hour)

		model := &AssetPostgreSQLModel{
			ID:                "asset-123",
			Name:              "US Dollar",
			Type:              "currency",
			Code:              "USD",
			Status:            "ACTIVE",
			StatusDescription: &statusDesc,
			LedgerID:          "ledger-456",
			OrganizationID:    "org-789",
			CreatedAt:         time.Now().Add(-48 * time.Hour),
			UpdatedAt:         time.Now().Add(-1 * time.Hour),
			DeletedAt:         sql.NullTime{Time: deletedAt, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.Name, entity.Name)
		assert.Equal(t, model.Type, entity.Type)
		assert.Equal(t, model.Code, entity.Code)
		assert.Equal(t, model.Status, entity.Status.Code)
		assert.Equal(t, model.StatusDescription, entity.Status.Description)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
		require.NotNil(t, entity.DeletedAt)
		assert.Equal(t, deletedAt, *entity.DeletedAt)
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		model := &AssetPostgreSQLModel{
			ID:             "asset-456",
			Name:           "Bitcoin",
			Type:           "crypto",
			Code:           "BTC",
			Status:         "PENDING",
			LedgerID:       "ledger-123",
			OrganizationID: "org-456",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.Name, entity.Name)
		assert.Equal(t, model.Type, entity.Type)
		assert.Equal(t, model.Code, entity.Code)
		assert.Equal(t, model.Status, entity.Status.Code)
		assert.Nil(t, entity.Status.Description)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Nil(t, entity.DeletedAt)
	})

	t.Run("with_deleted_at_valid_but_zero_time", func(t *testing.T) {
		// Tests the actual branch: source checks .Time.IsZero(), not .Valid
		model := &AssetPostgreSQLModel{
			ID:             "asset-edge",
			Name:           "Edge Asset",
			Type:           "commodity",
			Code:           "EDGE",
			Status:         "ACTIVE",
			LedgerID:       "ledger-edge",
			OrganizationID: "org-edge",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      sql.NullTime{Time: time.Time{}, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.DeletedAt, "DeletedAt should be nil when Time is zero, regardless of Valid flag")
	})
}

func TestAssetPostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		statusDesc := "Active currency"
		deletedAt := time.Now().Add(-24 * time.Hour)

		entity := &mmodel.Asset{
			ID:   "should-be-overwritten",
			Name: "US Dollar",
			Type: "currency",
			Code: "USD",
			Status: mmodel.Status{
				Code:        "ACTIVE",
				Description: &statusDesc,
			},
			LedgerID:       "ledger-456",
			OrganizationID: "org-789",
			CreatedAt:      time.Now().Add(-48 * time.Hour),
			UpdatedAt:      time.Now().Add(-1 * time.Hour),
			DeletedAt:      &deletedAt,
		}

		var model AssetPostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated")
		assert.NotEqual(t, entity.ID, model.ID, "ID should be newly generated, not copied from entity")
		assert.Equal(t, entity.Name, model.Name)
		assert.Equal(t, entity.Type, model.Type)
		assert.Equal(t, entity.Code, model.Code)
		assert.Equal(t, entity.Status.Code, model.Status)
		assert.Equal(t, entity.Status.Description, model.StatusDescription)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.True(t, model.DeletedAt.Valid, "DeletedAt should be valid")
		assert.Equal(t, deletedAt, model.DeletedAt.Time)
		assert.Nil(t, model.Metadata, "Metadata is not mapped by FromEntity")
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		entity := &mmodel.Asset{
			Name:           "Bitcoin",
			Type:           "crypto",
			Code:           "BTC",
			LedgerID:       "ledger-123",
			OrganizationID: "org-456",
			Status: mmodel.Status{
				Code: "PENDING",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		var model AssetPostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated")
		assert.Equal(t, entity.Name, model.Name)
		assert.Equal(t, entity.Type, model.Type)
		assert.Equal(t, entity.Code, model.Code)
		assert.Equal(t, entity.Status.Code, model.Status)
		assert.Nil(t, model.StatusDescription)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.False(t, model.DeletedAt.Valid, "DeletedAt should not be valid when entity.DeletedAt is nil")
	})

	t.Run("generates_uuid_v7", func(t *testing.T) {
		entity := &mmodel.Asset{
			Name:           "UUID Test Asset",
			Type:           "test",
			Code:           "TST",
			LedgerID:       "ledger-uuid",
			OrganizationID: "org-uuid",
			Status:         mmodel.Status{Code: "ACTIVE"},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model1 AssetPostgreSQLModel
		var model2 AssetPostgreSQLModel
		model1.FromEntity(entity)
		model2.FromEntity(entity)

		assert.NotEmpty(t, model1.ID)
		assert.NotEmpty(t, model2.ID)
		assert.NotEqual(t, model1.ID, model2.ID, "Each call should generate a unique ID")
		assert.Len(t, model1.ID, 36, "ID should be a valid UUID string (36 chars with hyphens)")
	})
}
