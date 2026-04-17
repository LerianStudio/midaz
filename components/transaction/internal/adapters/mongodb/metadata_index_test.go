// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestMetadataIndexMongoDBModel_ToEntity(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()
	created := time.Date(2024, 3, 15, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		model MetadataIndexMongoDBModel
	}{
		{
			name: "fully populated unique sparse index",
			model: MetadataIndexMongoDBModel{
				ID:          id,
				EntityName:  "Transaction",
				MetadataKey: "external_id",
				Unique:      true,
				Sparse:      true,
				CreatedAt:   created,
			},
		},
		{
			name: "non-unique non-sparse index",
			model: MetadataIndexMongoDBModel{
				ID:          primitive.NewObjectID(),
				EntityName:  "Operation",
				MetadataKey: "category",
				Unique:      false,
				Sparse:      false,
				CreatedAt:   created,
			},
		},
		{
			name:  "zero value model",
			model: MetadataIndexMongoDBModel{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := tt.model.ToEntity()
			require.NotNil(t, entity)

			assert.Equal(t, tt.model.ID, entity.ID)
			assert.Equal(t, tt.model.EntityName, entity.EntityName)
			assert.Equal(t, tt.model.MetadataKey, entity.MetadataKey)
			assert.Equal(t, tt.model.Unique, entity.Unique)
			assert.Equal(t, tt.model.Sparse, entity.Sparse)
			assert.Equal(t, tt.model.CreatedAt, entity.CreatedAt)
		})
	}
}

func TestMetadataIndexMongoDBModel_FromEntity(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()
	entityCreated := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		entity *MetadataIndex
	}{
		{
			name: "unique sparse",
			entity: &MetadataIndex{
				ID:          id,
				EntityName:  "Transaction",
				MetadataKey: "external_id",
				Unique:      true,
				Sparse:      true,
				CreatedAt:   entityCreated, // should be ignored; FromEntity stamps time.Now().UTC()
			},
		},
		{
			name: "plain index",
			entity: &MetadataIndex{
				ID:          primitive.NewObjectID(),
				EntityName:  "Operation",
				MetadataKey: "category",
				Unique:      false,
				Sparse:      false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before := time.Now().UTC()
			model := &MetadataIndexMongoDBModel{}

			err := model.FromEntity(tt.entity)

			after := time.Now().UTC()

			require.NoError(t, err)

			assert.Equal(t, tt.entity.ID, model.ID)
			assert.Equal(t, tt.entity.EntityName, model.EntityName)
			assert.Equal(t, tt.entity.MetadataKey, model.MetadataKey)
			assert.Equal(t, tt.entity.Unique, model.Unique)
			assert.Equal(t, tt.entity.Sparse, model.Sparse)

			// FromEntity sets CreatedAt to time.Now().UTC(), ignoring the source value.
			assert.False(t, model.CreatedAt.Before(before.Add(-time.Second)),
				"CreatedAt should be stamped at or after test start")
			assert.False(t, model.CreatedAt.After(after.Add(time.Second)),
				"CreatedAt should be stamped at or before test end")
		})
	}
}

func TestMetadataIndexMongoDBModel_RoundTrip(t *testing.T) {
	t.Parallel()

	original := &MetadataIndex{
		ID:          primitive.NewObjectID(),
		EntityName:  "Transaction",
		MetadataKey: "cust_ref",
		Unique:      true,
		Sparse:      false,
	}

	model := &MetadataIndexMongoDBModel{}
	require.NoError(t, model.FromEntity(original))

	roundTrip := model.ToEntity()
	require.NotNil(t, roundTrip)

	assert.Equal(t, original.ID, roundTrip.ID)
	assert.Equal(t, original.EntityName, roundTrip.EntityName)
	assert.Equal(t, original.MetadataKey, roundTrip.MetadataKey)
	assert.Equal(t, original.Unique, roundTrip.Unique)
	assert.Equal(t, original.Sparse, roundTrip.Sparse)
	// CreatedAt is freshly stamped by FromEntity, so it must be populated.
	assert.False(t, roundTrip.CreatedAt.IsZero())
}
