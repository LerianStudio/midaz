package segment

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSegmentPostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		statusDesc := "Active segment"
		deletedAt := time.Now().Add(-24 * time.Hour)

		model := &SegmentPostgreSQLModel{
			ID:                "segment-123",
			Name:              "Main Segment",
			LedgerID:          "ledger-456",
			OrganizationID:    "org-789",
			Status:            "ACTIVE",
			StatusDescription: &statusDesc,
			CreatedAt:         time.Now().Add(-48 * time.Hour),
			UpdatedAt:         time.Now().Add(-1 * time.Hour),
			DeletedAt:         sql.NullTime{Time: deletedAt, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.Name, entity.Name)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.Status, entity.Status.Code)
		assert.Equal(t, model.StatusDescription, entity.Status.Description)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
		require.NotNil(t, entity.DeletedAt)
		assert.Equal(t, deletedAt, *entity.DeletedAt)
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		model := &SegmentPostgreSQLModel{
			ID:             "segment-456",
			Name:           "Simple Segment",
			LedgerID:       "ledger-123",
			OrganizationID: "org-456",
			Status:         "PENDING",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.Name, entity.Name)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.Status, entity.Status.Code)
		assert.Nil(t, entity.Status.Description)
		assert.Nil(t, entity.DeletedAt)
	})

	t.Run("with_deleted_at_valid_but_zero_time", func(t *testing.T) {
		// Tests the actual branch: source checks .Time.IsZero(), not .Valid
		model := &SegmentPostgreSQLModel{
			ID:             "segment-edge",
			Name:           "Edge Case Segment",
			LedgerID:       "ledger-edge",
			OrganizationID: "org-edge",
			Status:         "ACTIVE",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      sql.NullTime{Time: time.Time{}, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.DeletedAt, "DeletedAt should be nil when Time is zero, regardless of Valid flag")
	})
}

func TestSegmentPostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		statusDesc := "Active segment"
		deletedAt := time.Now().Add(-24 * time.Hour)

		entity := &mmodel.Segment{
			ID:             "should-be-overwritten",
			Name:           "Main Segment",
			LedgerID:       "ledger-456",
			OrganizationID: "org-789",
			Status: mmodel.Status{
				Code:        "ACTIVE",
				Description: &statusDesc,
			},
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
			DeletedAt: &deletedAt,
		}

		var model SegmentPostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated")
		assert.NotEqual(t, entity.ID, model.ID, "ID should be newly generated, not copied from entity")
		assert.Equal(t, entity.Name, model.Name)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.Status.Code, model.Status)
		assert.Equal(t, entity.Status.Description, model.StatusDescription)
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.True(t, model.DeletedAt.Valid, "DeletedAt should be valid")
		assert.Equal(t, deletedAt, model.DeletedAt.Time)
		assert.Nil(t, model.Metadata, "Metadata is not mapped by FromEntity")
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		entity := &mmodel.Segment{
			Name:           "Simple Segment",
			LedgerID:       "ledger-123",
			OrganizationID: "org-456",
			Status: mmodel.Status{
				Code: "PENDING",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		var model SegmentPostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated")
		assert.Equal(t, entity.Name, model.Name)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.Status.Code, model.Status)
		assert.Nil(t, model.StatusDescription)
		assert.False(t, model.DeletedAt.Valid, "DeletedAt should not be valid when entity.DeletedAt is nil")
	})

	t.Run("generates_uuid_v7", func(t *testing.T) {
		entity := &mmodel.Segment{
			Name:           "UUID Test Segment",
			LedgerID:       "ledger-uuid",
			OrganizationID: "org-uuid",
			Status:         mmodel.Status{Code: "ACTIVE"},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model1 SegmentPostgreSQLModel
		var model2 SegmentPostgreSQLModel
		model1.FromEntity(entity)
		model2.FromEntity(entity)

		assert.NotEmpty(t, model1.ID)
		assert.NotEmpty(t, model2.ID)
		assert.NotEqual(t, model1.ID, model2.ID, "Each call should generate a unique ID")
		assert.Len(t, model1.ID, 36, "ID should be a valid UUID string (36 chars with hyphens)")
	})
}
