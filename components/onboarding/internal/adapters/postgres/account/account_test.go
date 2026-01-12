package account

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountPostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		parentAccountID := "parent-acc-123"
		entityID := "entity-456"
		portfolioID := "portfolio-789"
		segmentID := "segment-012"
		statusDesc := "Active account"
		alias := "main-account"
		deletedAt := time.Now().Add(-24 * time.Hour)

		model := &AccountPostgreSQLModel{
			ID:                "acc-123",
			Name:              "Main Account",
			ParentAccountID:   &parentAccountID,
			EntityID:          &entityID,
			AssetCode:         "USD",
			OrganizationID:    "org-456",
			LedgerID:          "ledger-789",
			PortfolioID:       &portfolioID,
			SegmentID:         &segmentID,
			Status:            "ACTIVE",
			StatusDescription: &statusDesc,
			Alias:             &alias,
			Type:              "deposit",
			Blocked:           true,
			CreatedAt:         time.Now().Add(-48 * time.Hour),
			UpdatedAt:         time.Now().Add(-1 * time.Hour),
			DeletedAt:         sql.NullTime{Time: deletedAt, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.Name, entity.Name)
		assert.Equal(t, model.ParentAccountID, entity.ParentAccountID)
		assert.Equal(t, model.EntityID, entity.EntityID)
		assert.Equal(t, model.AssetCode, entity.AssetCode)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.PortfolioID, entity.PortfolioID)
		assert.Equal(t, model.SegmentID, entity.SegmentID)
		assert.Equal(t, model.Status, entity.Status.Code)
		assert.Equal(t, model.StatusDescription, entity.Status.Description)
		assert.Equal(t, model.Alias, entity.Alias)
		assert.Equal(t, model.Type, entity.Type)
		require.NotNil(t, entity.Blocked, "Blocked should be converted to pointer")
		assert.True(t, *entity.Blocked)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
		require.NotNil(t, entity.DeletedAt)
		assert.Equal(t, deletedAt, *entity.DeletedAt)
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		model := &AccountPostgreSQLModel{
			ID:             "acc-456",
			Name:           "Simple Account",
			AssetCode:      "BRL",
			OrganizationID: "org-123",
			LedgerID:       "ledger-456",
			Status:         "PENDING",
			Type:           "savings",
			Blocked:        false,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Nil(t, entity.ParentAccountID)
		assert.Nil(t, entity.EntityID)
		assert.Nil(t, entity.PortfolioID)
		assert.Nil(t, entity.SegmentID)
		assert.Nil(t, entity.Status.Description)
		assert.Nil(t, entity.Alias)
		require.NotNil(t, entity.Blocked)
		assert.False(t, *entity.Blocked)
		assert.Nil(t, entity.DeletedAt)
	})

	t.Run("with_deleted_at_valid_but_zero_time", func(t *testing.T) {
		// Tests the actual branch: source checks .Time.IsZero(), not .Valid
		model := &AccountPostgreSQLModel{
			ID:             "acc-edge",
			Name:           "Edge Case Account",
			AssetCode:      "EUR",
			OrganizationID: "org-edge",
			LedgerID:       "ledger-edge",
			Status:         "ACTIVE",
			Type:           "checking",
			Blocked:        false,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      sql.NullTime{Time: time.Time{}, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.DeletedAt, "DeletedAt should be nil when Time is zero, regardless of Valid flag")
	})
}

func TestAccountPostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		parentAccountID := "parent-acc-123"
		entityID := "entity-456"
		portfolioID := "portfolio-789"
		segmentID := "segment-012"
		statusDesc := "Active account"
		alias := "main-account"
		blocked := true
		deletedAt := time.Now().Add(-24 * time.Hour)

		entity := &mmodel.Account{
			ID:              "acc-existing-id",
			Name:            "Main Account",
			ParentAccountID: &parentAccountID,
			EntityID:        &entityID,
			AssetCode:       "USD",
			OrganizationID:  "org-456",
			LedgerID:        "ledger-789",
			PortfolioID:     &portfolioID,
			SegmentID:       &segmentID,
			Status: mmodel.Status{
				Code:        "ACTIVE",
				Description: &statusDesc,
			},
			Alias:     &alias,
			Type:      "DEPOSIT",
			Blocked:   &blocked,
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
			DeletedAt: &deletedAt,
		}

		var model AccountPostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, entity.ID, model.ID, "ID should be preserved when provided")
		assert.Equal(t, entity.Name, model.Name)
		assert.Equal(t, entity.ParentAccountID, model.ParentAccountID)
		assert.Equal(t, entity.EntityID, model.EntityID)
		assert.Equal(t, entity.AssetCode, model.AssetCode)
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.PortfolioID, model.PortfolioID)
		assert.Equal(t, entity.SegmentID, model.SegmentID)
		assert.Equal(t, entity.Status.Code, model.Status)
		assert.Equal(t, entity.Status.Description, model.StatusDescription)
		assert.Equal(t, entity.Alias, model.Alias)
		assert.Equal(t, "deposit", model.Type, "Type should be converted to lowercase")
		assert.True(t, model.Blocked)
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.True(t, model.DeletedAt.Valid)
		assert.Equal(t, deletedAt, model.DeletedAt.Time)
		assert.Nil(t, model.Metadata, "Metadata is not mapped by FromEntity")
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		entity := &mmodel.Account{
			Name:           "Simple Account",
			AssetCode:      "BRL",
			OrganizationID: "org-123",
			LedgerID:       "ledger-456",
			Status: mmodel.Status{
				Code: "PENDING",
			},
			Type:      "savings",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		var model AccountPostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated when entity.ID is empty")
		assert.Nil(t, model.ParentAccountID)
		assert.Nil(t, model.EntityID)
		assert.Nil(t, model.PortfolioID, "PortfolioID should be nil when entity.PortfolioID is nil")
		assert.Nil(t, model.StatusDescription)
		assert.Nil(t, model.Alias)
		assert.False(t, model.Blocked, "Blocked defaults to false when entity.Blocked is nil")
		assert.False(t, model.DeletedAt.Valid)
	})

	t.Run("generates_uuid_when_id_empty", func(t *testing.T) {
		entity := &mmodel.Account{
			ID:             "",
			Name:           "Account without ID",
			AssetCode:      "EUR",
			OrganizationID: "org-456",
			LedgerID:       "ledger-789",
			Status:         mmodel.Status{Code: "ACTIVE"},
			Type:           "checking",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model1 AccountPostgreSQLModel
		var model2 AccountPostgreSQLModel
		model1.FromEntity(entity)
		model2.FromEntity(entity)

		assert.NotEmpty(t, model1.ID)
		assert.NotEmpty(t, model2.ID)
		assert.NotEqual(t, model1.ID, model2.ID, "Each call should generate a unique ID when entity.ID is empty")
		assert.Len(t, model1.ID, 36, "ID should be a valid UUID string")
	})

	t.Run("converts_type_to_lowercase", func(t *testing.T) {
		cases := []struct {
			input    string
			expected string
		}{
			{"DEPOSIT", "deposit"},
			{"Savings", "savings"},
			{"CHECKING", "checking"},
			{"MixedCase", "mixedcase"},
		}

		for _, tc := range cases {
			entity := &mmodel.Account{
				Name:           "Type Test",
				AssetCode:      "USD",
				OrganizationID: "org-123",
				LedgerID:       "ledger-456",
				Status:         mmodel.Status{Code: "ACTIVE"},
				Type:           tc.input,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			var model AccountPostgreSQLModel
			model.FromEntity(entity)

			assert.Equal(t, tc.expected, model.Type, "Type '%s' should be converted to '%s'", tc.input, tc.expected)
		}
	})
}
