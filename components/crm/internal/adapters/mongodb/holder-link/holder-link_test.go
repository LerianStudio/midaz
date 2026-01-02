package holderlink

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T {
	return &v
}

func TestMongoDBModel_FromEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	id := uuid.New()
	holderID := uuid.New()
	aliasID := uuid.New()

	tests := []struct {
		name       string
		holderLink *mmodel.HolderLink
	}{
		{
			name: "complete_holder_link",
			holderLink: &mmodel.HolderLink{
				ID:       &id,
				HolderID: &holderID,
				AliasID:  &aliasID,
				LinkType: ptr("PRIMARY_HOLDER"),
				Metadata: map[string]any{
					"key1": "value1",
					"key2": 123,
				},
				CreatedAt: now,
				UpdatedAt: now,
				DeletedAt: &now,
			},
		},
		{
			name: "minimal_holder_link",
			holderLink: &mmodel.HolderLink{
				ID:        &id,
				HolderID:  &holderID,
				AliasID:   &aliasID,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		{
			name: "nil_metadata_initializes_empty_map",
			holderLink: &mmodel.HolderLink{
				ID:        &id,
				HolderID:  &holderID,
				AliasID:   &aliasID,
				LinkType:  ptr("LEGAL_REPRESENTATIVE"),
				Metadata:  nil,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		{
			name: "with_responsible_party_link_type",
			holderLink: &mmodel.HolderLink{
				ID:        &id,
				HolderID:  &holderID,
				AliasID:   &aliasID,
				LinkType:  ptr("RESPONSIBLE_PARTY"),
				Metadata:  map[string]any{"source": "api"},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model MongoDBModel
			model.FromEntity(tt.holderLink)

			assert.Equal(t, tt.holderLink.ID, model.ID)
			assert.Equal(t, tt.holderLink.HolderID, model.HolderID)
			assert.Equal(t, tt.holderLink.AliasID, model.AliasID)
			assert.Equal(t, tt.holderLink.LinkType, model.LinkType)

			require.NotNil(t, model.CreatedAt)
			assert.Equal(t, tt.holderLink.CreatedAt, *model.CreatedAt)

			require.NotNil(t, model.UpdatedAt)
			assert.Equal(t, tt.holderLink.UpdatedAt, *model.UpdatedAt)

			assert.Equal(t, tt.holderLink.DeletedAt, model.DeletedAt)

			// Metadata should never be nil
			require.NotNil(t, model.Metadata, "Metadata should never be nil")
			if tt.holderLink.Metadata != nil {
				assert.Equal(t, tt.holderLink.Metadata, model.Metadata)
			} else {
				assert.Empty(t, model.Metadata)
			}
		})
	}
}

func TestMongoDBModel_ToEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	id := uuid.New()
	holderID := uuid.New()
	aliasID := uuid.New()

	tests := []struct {
		name  string
		model *MongoDBModel
	}{
		{
			name: "complete_model",
			model: &MongoDBModel{
				ID:       &id,
				HolderID: &holderID,
				AliasID:  &aliasID,
				LinkType: ptr("PRIMARY_HOLDER"),
				Metadata: map[string]any{
					"key1": "value1",
				},
				CreatedAt: &now,
				UpdatedAt: &now,
				DeletedAt: &now,
			},
		},
		{
			name: "nil_timestamps",
			model: &MongoDBModel{
				ID:        &id,
				HolderID:  &holderID,
				AliasID:   &aliasID,
				LinkType:  ptr("LEGAL_REPRESENTATIVE"),
				Metadata:  map[string]any{},
				CreatedAt: nil,
				UpdatedAt: nil,
				DeletedAt: nil,
			},
		},
		{
			name: "minimal_model",
			model: &MongoDBModel{
				ID:        &id,
				HolderID:  &holderID,
				AliasID:   &aliasID,
				CreatedAt: &now,
				UpdatedAt: &now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.model.ToEntity()

			require.NotNil(t, result)
			assert.Equal(t, tt.model.ID, result.ID)
			assert.Equal(t, tt.model.HolderID, result.HolderID)
			assert.Equal(t, tt.model.AliasID, result.AliasID)
			assert.Equal(t, tt.model.LinkType, result.LinkType)
			assert.Equal(t, tt.model.Metadata, result.Metadata)
			assert.Equal(t, tt.model.DeletedAt, result.DeletedAt)

			// CreatedAt/UpdatedAt should be zero value if nil in model
			if tt.model.CreatedAt != nil {
				assert.Equal(t, *tt.model.CreatedAt, result.CreatedAt)
			} else {
				assert.True(t, result.CreatedAt.IsZero())
			}

			if tt.model.UpdatedAt != nil {
				assert.Equal(t, *tt.model.UpdatedAt, result.UpdatedAt)
			} else {
				assert.True(t, result.UpdatedAt.IsZero())
			}
		})
	}
}

func TestMongoDBModel_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	id := uuid.New()
	holderID := uuid.New()
	aliasID := uuid.New()

	original := &mmodel.HolderLink{
		ID:       &id,
		HolderID: &holderID,
		AliasID:  &aliasID,
		LinkType: ptr("PRIMARY_HOLDER"),
		Metadata: map[string]any{
			"round": "trip",
			"count": 42,
		},
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: nil,
	}

	// Entity -> Model
	var model MongoDBModel
	model.FromEntity(original)

	// Model -> Entity
	result := model.ToEntity()

	// Verify round-trip preserves data
	assert.Equal(t, original.ID, result.ID)
	assert.Equal(t, original.HolderID, result.HolderID)
	assert.Equal(t, original.AliasID, result.AliasID)
	assert.Equal(t, original.LinkType, result.LinkType)
	assert.Equal(t, original.Metadata, result.Metadata)
	assert.Equal(t, original.CreatedAt, result.CreatedAt)
	assert.Equal(t, original.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, original.DeletedAt, result.DeletedAt)
}
