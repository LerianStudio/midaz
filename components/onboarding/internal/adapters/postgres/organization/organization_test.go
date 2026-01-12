package organization

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationPostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		parentOrgID := "parent-org-123"
		dba := "Acme Corp"
		statusDesc := "Active and operational"
		deletedAt := time.Now().Add(-24 * time.Hour)

		model := &OrganizationPostgreSQLModel{
			ID:                   "org-123",
			ParentOrganizationID: &parentOrgID,
			LegalName:            "Acme Corporation LLC",
			DoingBusinessAs:      &dba,
			LegalDocument:        "12345678901234",
			Address: mmodel.Address{
				Line1:   "123 Main St",
				Line2:   stringPtr("Suite 100"),
				ZipCode: "10001",
				City:    "New York",
				State:   "NY",
				Country: "US",
			},
			Status:            "ACTIVE",
			StatusDescription: &statusDesc,
			CreatedAt:         time.Now().Add(-48 * time.Hour),
			UpdatedAt:         time.Now().Add(-1 * time.Hour),
			DeletedAt:         sql.NullTime{Time: deletedAt, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.ParentOrganizationID, entity.ParentOrganizationID)
		assert.Equal(t, model.LegalName, entity.LegalName)
		assert.Equal(t, model.DoingBusinessAs, entity.DoingBusinessAs)
		assert.Equal(t, model.LegalDocument, entity.LegalDocument)
		assert.Equal(t, model.Address, entity.Address)
		assert.Equal(t, model.Status, entity.Status.Code)
		assert.Equal(t, model.StatusDescription, entity.Status.Description)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
		require.NotNil(t, entity.DeletedAt)
		assert.Equal(t, deletedAt, *entity.DeletedAt)
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		model := &OrganizationPostgreSQLModel{
			ID:            "org-456",
			LegalName:     "Simple Corp",
			LegalDocument: "98765432109876",
			Status:        "PENDING",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Nil(t, entity.ParentOrganizationID)
		assert.Equal(t, model.LegalName, entity.LegalName)
		assert.Nil(t, entity.DoingBusinessAs)
		assert.Equal(t, model.LegalDocument, entity.LegalDocument)
		assert.Equal(t, model.Status, entity.Status.Code)
		assert.Nil(t, entity.Status.Description)
		assert.Nil(t, entity.DeletedAt)
	})

	t.Run("with_deleted_at_valid_but_zero_time", func(t *testing.T) {
		// Tests the actual branch: source checks .Time.IsZero(), not .Valid
		model := &OrganizationPostgreSQLModel{
			ID:            "org-789",
			LegalName:     "Another Corp",
			LegalDocument: "11111111111111",
			Status:        "ACTIVE",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			DeletedAt:     sql.NullTime{Time: time.Time{}, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.DeletedAt, "DeletedAt should be nil when Time is zero, regardless of Valid flag")
	})
}

func TestOrganizationPostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		parentOrgID := "parent-org-123"
		dba := "Acme Corp"
		statusDesc := "Active and operational"
		deletedAt := time.Now().Add(-24 * time.Hour)

		entity := &mmodel.Organization{
			ID:                   "should-be-overwritten",
			ParentOrganizationID: &parentOrgID,
			LegalName:            "Acme Corporation LLC",
			DoingBusinessAs:      &dba,
			LegalDocument:        "12345678901234",
			Address: mmodel.Address{
				Line1:   "123 Main St",
				Line2:   stringPtr("Suite 100"),
				ZipCode: "10001",
				City:    "New York",
				State:   "NY",
				Country: "US",
			},
			Status: mmodel.Status{
				Code:        "ACTIVE",
				Description: &statusDesc,
			},
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
			DeletedAt: &deletedAt,
		}

		var model OrganizationPostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated")
		assert.NotEqual(t, entity.ID, model.ID, "ID should be newly generated, not copied from entity")
		assert.Equal(t, entity.ParentOrganizationID, model.ParentOrganizationID)
		assert.Equal(t, entity.LegalName, model.LegalName)
		assert.Equal(t, entity.DoingBusinessAs, model.DoingBusinessAs)
		assert.Equal(t, entity.LegalDocument, model.LegalDocument)
		assert.Equal(t, entity.Address, model.Address)
		assert.Equal(t, entity.Status.Code, model.Status)
		assert.Equal(t, entity.Status.Description, model.StatusDescription)
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.True(t, model.DeletedAt.Valid, "DeletedAt should be valid")
		assert.Equal(t, deletedAt, model.DeletedAt.Time)
		assert.Nil(t, model.Metadata, "Metadata is not mapped by FromEntity")
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		entity := &mmodel.Organization{
			LegalName:     "Simple Corp",
			LegalDocument: "98765432109876",
			Status: mmodel.Status{
				Code: "PENDING",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		var model OrganizationPostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated")
		assert.Nil(t, model.ParentOrganizationID)
		assert.Equal(t, entity.LegalName, model.LegalName)
		assert.Nil(t, model.DoingBusinessAs)
		assert.Equal(t, entity.LegalDocument, model.LegalDocument)
		assert.Equal(t, entity.Status.Code, model.Status)
		assert.Nil(t, model.StatusDescription)
		assert.False(t, model.DeletedAt.Valid, "DeletedAt should not be valid when entity.DeletedAt is nil")
	})

	t.Run("generates_uuid_v7", func(t *testing.T) {
		entity := &mmodel.Organization{
			LegalName:     "UUID Test Corp",
			LegalDocument: "33333333333333",
			Status:        mmodel.Status{Code: "ACTIVE"},
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		var model1 OrganizationPostgreSQLModel
		var model2 OrganizationPostgreSQLModel
		model1.FromEntity(entity)
		model2.FromEntity(entity)

		assert.NotEmpty(t, model1.ID)
		assert.NotEmpty(t, model2.ID)
		assert.NotEqual(t, model1.ID, model2.ID, "Each call should generate a unique ID")
		assert.Len(t, model1.ID, 36, "ID should be a valid UUID string (36 chars with hyphens)")
	})
}

func stringPtr(s string) *string {
	return &s
}
