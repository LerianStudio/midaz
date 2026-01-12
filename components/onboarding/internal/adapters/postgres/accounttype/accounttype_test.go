package accounttype

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountTypePostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		deletedAt := time.Now().Add(-24 * time.Hour)

		model := &AccountTypePostgreSQLModel{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Name:           "Savings Account",
			Description:    "Standard savings account type",
			KeyValue:       "savings",
			CreatedAt:      time.Now().Add(-48 * time.Hour),
			UpdatedAt:      time.Now().Add(-1 * time.Hour),
			DeletedAt:      sql.NullTime{Time: deletedAt, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.Name, entity.Name)
		assert.Equal(t, model.Description, entity.Description)
		assert.Equal(t, model.KeyValue, entity.KeyValue)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
		require.NotNil(t, entity.DeletedAt)
		assert.Equal(t, deletedAt, *entity.DeletedAt)
	})

	t.Run("with_deleted_at_invalid", func(t *testing.T) {
		// AccountType uses .Valid check (different from other models that use .Time.IsZero())
		model := &AccountTypePostgreSQLModel{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Name:           "Edge Case Type",
			Description:    "Testing edge case",
			KeyValue:       "edge",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      sql.NullTime{Time: time.Now(), Valid: false},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.DeletedAt, "DeletedAt should be nil when Valid is false, even with non-zero Time")
	})
}

func TestAccountTypePostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		deletedAt := time.Now().Add(-24 * time.Hour)

		entity := &mmodel.AccountType{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Name:           "Savings Account",
			Description:    "Standard savings account type",
			KeyValue:       "SAVINGS",
			CreatedAt:      time.Now().Add(-48 * time.Hour),
			UpdatedAt:      time.Now().Add(-1 * time.Hour),
			DeletedAt:      &deletedAt,
		}

		var model AccountTypePostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, entity.ID, model.ID, "ID should be copied from entity")
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.Name, model.Name)
		assert.Equal(t, entity.Description, model.Description)
		assert.Equal(t, "savings", model.KeyValue, "KeyValue should be converted to lowercase")
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.True(t, model.DeletedAt.Valid)
		assert.Equal(t, deletedAt, model.DeletedAt.Time)
	})

	t.Run("with_deleted_at_nil_resets_to_zero", func(t *testing.T) {
		entity := &mmodel.AccountType{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Name:           "Checking Account",
			Description:    "Standard checking account type",
			KeyValue:       "checking",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      nil,
		}

		var model AccountTypePostgreSQLModel
		model.FromEntity(entity)

		assert.False(t, model.DeletedAt.Valid, "DeletedAt.Valid should be false when entity.DeletedAt is nil")
		assert.True(t, model.DeletedAt.Time.IsZero(), "DeletedAt.Time should be zero when entity.DeletedAt is nil")
	})

	t.Run("converts_keyvalue_to_lowercase", func(t *testing.T) {
		cases := []struct {
			input    string
			expected string
		}{
			{"SAVINGS", "savings"},
			{"Checking", "checking"},
			{"DEPOSIT", "deposit"},
			{"MixedCase", "mixedcase"},
		}

		for _, tc := range cases {
			entity := &mmodel.AccountType{
				ID:             uuid.New(),
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				Name:           "Test Type",
				Description:    "Test",
				KeyValue:       tc.input,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			var model AccountTypePostgreSQLModel
			model.FromEntity(entity)

			assert.Equal(t, tc.expected, model.KeyValue, "KeyValue '%s' should be converted to '%s'", tc.input, tc.expected)
		}
	})

}
