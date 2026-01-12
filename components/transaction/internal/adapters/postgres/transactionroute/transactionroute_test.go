package transactionroute

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionRoutePostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()
		deletedAt := time.Now().Add(-24 * time.Hour)

		model := &TransactionRoutePostgreSQLModel{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Charge Settlement",
			Description:    "Settlement route for service charges",
			CreatedAt:      time.Now().Add(-48 * time.Hour),
			UpdatedAt:      time.Now().Add(-1 * time.Hour),
			DeletedAt:      sql.NullTime{Time: deletedAt, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.Title, entity.Title)
		assert.Equal(t, model.Description, entity.Description)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
		require.NotNil(t, entity.DeletedAt)
		assert.Equal(t, deletedAt, *entity.DeletedAt)
	})

	t.Run("with_deleted_at_nil", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		model := &TransactionRoutePostgreSQLModel{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Simple Route",
			Description:    "Route without deletion",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      sql.NullTime{Valid: false},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.DeletedAt, "DeletedAt should be nil when Valid is false")
	})

	t.Run("with_deleted_at_valid_but_zero_time", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		model := &TransactionRoutePostgreSQLModel{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Edge Case Route",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      sql.NullTime{Time: time.Time{}, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		require.NotNil(t, entity.DeletedAt, "DeletedAt should be set when Valid is true")
		assert.True(t, entity.DeletedAt.IsZero(), "DeletedAt should preserve zero time value")
	})

	t.Run("with_empty_description", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		model := &TransactionRoutePostgreSQLModel{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Minimal Route",
			Description:    "",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Empty(t, entity.Description)
	})
}

func TestTransactionRoutePostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()
		deletedAt := time.Now().Add(-24 * time.Hour)

		entity := &mmodel.TransactionRoute{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Charge Settlement",
			Description:    "Settlement route for service charges",
			CreatedAt:      time.Now().Add(-48 * time.Hour),
			UpdatedAt:      time.Now().Add(-1 * time.Hour),
			DeletedAt:      &deletedAt,
		}

		var model TransactionRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, entity.ID, model.ID)
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.Title, model.Title)
		assert.Equal(t, entity.Description, model.Description)
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.True(t, model.DeletedAt.Valid)
		assert.Equal(t, deletedAt, model.DeletedAt.Time)
	})

	t.Run("with_deleted_at_nil", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		entity := &mmodel.TransactionRoute{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Active Route",
			Description:    "Route without deletion",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      nil,
		}

		var model TransactionRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.False(t, model.DeletedAt.Valid, "DeletedAt.Valid should be false when entity.DeletedAt is nil")
		assert.True(t, model.DeletedAt.Time.IsZero(), "DeletedAt.Time should be zero value")
	})

	t.Run("with_optional_fields_empty", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		entity := &mmodel.TransactionRoute{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Minimal Route",
			Description:    "",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model TransactionRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, entity.ID, model.ID)
		assert.Empty(t, model.Description)
		assert.False(t, model.DeletedAt.Valid)
	})
}
