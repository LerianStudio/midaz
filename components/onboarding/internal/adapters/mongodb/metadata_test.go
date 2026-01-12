package mongodb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestJSON_Value(t *testing.T) {
	tests := []struct {
		name    string
		json    JSON
		wantErr bool
	}{
		{
			name:    "empty JSON",
			json:    JSON{},
			wantErr: false,
		},
		{
			name: "simple key-value pairs",
			json: JSON{
				"key1": "value1",
				"key2": 123,
			},
			wantErr: false,
		},
		{
			name: "nested structures",
			json: JSON{
				"nested": map[string]any{
					"inner": "value",
				},
				"array": []any{1, 2, 3},
			},
			wantErr: false,
		},
		{
			name: "various types",
			json: JSON{
				"string":  "test",
				"int":     42,
				"float":   3.14,
				"bool":    true,
				"null":    nil,
				"array":   []any{"a", "b"},
				"object":  map[string]any{"x": 1},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := tt.json.Value()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, value)

			// Value should be a byte slice (marshalled JSON)
			bytes, ok := value.([]byte)
			assert.True(t, ok, "Value should return []byte")
			assert.NotEmpty(t, bytes)
		})
	}
}

func TestJSON_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected JSON
		wantErr  bool
	}{
		{
			name:     "valid JSON bytes",
			input:    []byte(`{"key": "value"}`),
			expected: JSON{"key": "value"},
			wantErr:  false,
		},
		{
			name:     "empty object",
			input:    []byte(`{}`),
			expected: JSON{},
			wantErr:  false,
		},
		{
			name:  "complex JSON",
			input: []byte(`{"name": "test", "count": 42, "active": true}`),
			expected: JSON{
				"name":   "test",
				"count":  float64(42), // JSON numbers unmarshall to float64
				"active": true,
			},
			wantErr: false,
		},
		{
			name:    "non-byte input",
			input:   "not bytes",
			wantErr: true,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name:    "invalid JSON bytes",
			input:   []byte(`{invalid json}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var j JSON
			err := j.Scan(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, j)
		})
	}
}

func TestJSON_RoundTrip(t *testing.T) {
	original := JSON{
		"string":   "hello",
		"number":   float64(123),
		"boolean":  true,
		"nullVal":  nil,
		"array":    []any{"a", "b"},
	}

	value, err := original.Value()
	require.NoError(t, err)

	var restored JSON
	err = restored.Scan(value)
	require.NoError(t, err)

	assert.Equal(t, original["string"], restored["string"])
	assert.Equal(t, original["number"], restored["number"])
	assert.Equal(t, original["boolean"], restored["boolean"])
	assert.Nil(t, restored["nullVal"])
	assert.Equal(t, original["array"], restored["array"])
}

func TestMetadataMongoDBModel_ToEntity(t *testing.T) {
	objectID := primitive.NewObjectID()
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name  string
		model *MetadataMongoDBModel
	}{
		{
			name: "complete model",
			model: &MetadataMongoDBModel{
				ID:         objectID,
				EntityID:   "entity-123",
				EntityName: "account",
				Data:       JSON{"key": "value"},
				CreatedAt:  now,
				UpdatedAt:  now.Add(time.Hour),
			},
		},
		{
			name: "empty data",
			model: &MetadataMongoDBModel{
				ID:         objectID,
				EntityID:   "entity-456",
				EntityName: "ledger",
				Data:       JSON{},
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
		{
			name: "complex data",
			model: &MetadataMongoDBModel{
				ID:         objectID,
				EntityID:   "entity-789",
				EntityName: "organization",
				Data: JSON{
					"nested": map[string]any{
						"level1": map[string]any{
							"level2": "deep",
						},
					},
					"tags": []any{"tag1", "tag2"},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := tt.model.ToEntity()

			require.NotNil(t, entity)
			assert.Equal(t, tt.model.ID, entity.ID)
			assert.Equal(t, tt.model.EntityID, entity.EntityID)
			assert.Equal(t, tt.model.EntityName, entity.EntityName)
			assert.Equal(t, tt.model.Data, entity.Data)
			assert.Equal(t, tt.model.CreatedAt, entity.CreatedAt)
			assert.Equal(t, tt.model.UpdatedAt, entity.UpdatedAt)
		})
	}
}

func TestMetadataMongoDBModel_FromEntity(t *testing.T) {
	objectID := primitive.NewObjectID()
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name   string
		entity *Metadata
	}{
		{
			name: "complete entity",
			entity: &Metadata{
				ID:         objectID,
				EntityID:   "entity-abc",
				EntityName: "transaction",
				Data:       JSON{"status": "active"},
				CreatedAt:  now,
				UpdatedAt:  now.Add(time.Minute),
			},
		},
		{
			name: "minimal entity",
			entity: &Metadata{
				ID:         objectID,
				EntityID:   "entity-def",
				EntityName: "asset",
				Data:       nil,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model MetadataMongoDBModel
			err := model.FromEntity(tt.entity)

			require.NoError(t, err)
			assert.Equal(t, tt.entity.ID, model.ID)
			assert.Equal(t, tt.entity.EntityID, model.EntityID)
			assert.Equal(t, tt.entity.EntityName, model.EntityName)
			assert.Equal(t, tt.entity.Data, model.Data)
			assert.Equal(t, tt.entity.CreatedAt, model.CreatedAt)
			assert.Equal(t, tt.entity.UpdatedAt, model.UpdatedAt)
		})
	}
}

func TestMetadataMongoDBModel_RoundTrip(t *testing.T) {
	objectID := primitive.NewObjectID()
	now := time.Now().UTC().Truncate(time.Second)

	original := &Metadata{
		ID:         objectID,
		EntityID:   "roundtrip-entity",
		EntityName: "test-entity",
		Data: JSON{
			"key1": "value1",
			"key2": float64(100),
		},
		CreatedAt: now,
		UpdatedAt: now.Add(time.Hour * 24),
	}

	var model MetadataMongoDBModel
	err := model.FromEntity(original)
	require.NoError(t, err)

	result := model.ToEntity()

	assert.Equal(t, original.ID, result.ID)
	assert.Equal(t, original.EntityID, result.EntityID)
	assert.Equal(t, original.EntityName, result.EntityName)
	assert.Equal(t, original.Data, result.Data)
	assert.Equal(t, original.CreatedAt, result.CreatedAt)
	assert.Equal(t, original.UpdatedAt, result.UpdatedAt)
}
