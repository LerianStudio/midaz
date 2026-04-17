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
	createdAt := time.Now().UTC()

	model := &MetadataIndexMongoDBModel{
		ID:          id,
		EntityName:  "segment",
		MetadataKey: "tier",
		Unique:      true,
		Sparse:      true,
		CreatedAt:   createdAt,
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID)
	assert.Equal(t, "segment", entity.EntityName)
	assert.Equal(t, "tier", entity.MetadataKey)
	assert.True(t, entity.Unique)
	assert.True(t, entity.Sparse)
	assert.Equal(t, createdAt, entity.CreatedAt)
}

func TestMetadataIndexMongoDBModel_FromEntity(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()
	createdAt := time.Now().UTC()

	entity := &MetadataIndex{
		ID:          id,
		EntityName:  "account",
		MetadataKey: "segmentation",
		Unique:      false,
		Sparse:      false,
		CreatedAt:   createdAt,
	}

	var model MetadataIndexMongoDBModel
	require.NoError(t, model.FromEntity(entity))

	assert.Equal(t, id, model.ID)
	assert.Equal(t, "account", model.EntityName)
	assert.Equal(t, "segmentation", model.MetadataKey)
	assert.False(t, model.Unique)
	assert.False(t, model.Sparse)
	assert.Equal(t, createdAt, model.CreatedAt)
}

func TestMetadataIndexMongoDBModel_Roundtrip(t *testing.T) {
	t.Parallel()

	original := &MetadataIndex{
		ID:          primitive.NewObjectID(),
		EntityName:  "ledger",
		MetadataKey: "tenant",
		Unique:      true,
		Sparse:      false,
		CreatedAt:   time.Now().UTC(),
	}

	var model MetadataIndexMongoDBModel
	require.NoError(t, model.FromEntity(original))

	got := model.ToEntity()
	assert.Equal(t, original.ID, got.ID)
	assert.Equal(t, original.EntityName, got.EntityName)
	assert.Equal(t, original.MetadataKey, got.MetadataKey)
	assert.Equal(t, original.Unique, got.Unique)
	assert.Equal(t, original.Sparse, got.Sparse)
	assert.Equal(t, original.CreatedAt, got.CreatedAt)
}
