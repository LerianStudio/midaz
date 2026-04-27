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

// indexFixedTime is shared so tests don't rely on time.Now() — per Midaz CLAUDE.md
// rule "do not use time.Now() in tests".
var indexFixedTime = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

// TestMetadataIndexMongoDBModel_ToEntity covers the index-row → entity translation.
// Every field must round-trip — drift here is invisible at compile time but surfaces
// as 500s on the metadata-index admin endpoints in production.
func TestMetadataIndexMongoDBModel_ToEntity(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()

	model := &MetadataIndexMongoDBModel{
		ID:          id,
		EntityName:  "Transaction",
		MetadataKey: "trace_id",
		Unique:      true,
		Sparse:      false,
		CreatedAt:   indexFixedTime,
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID)
	assert.Equal(t, "Transaction", entity.EntityName)
	assert.Equal(t, "trace_id", entity.MetadataKey)
	assert.True(t, entity.Unique)
	assert.False(t, entity.Sparse)
	assert.Equal(t, indexFixedTime, entity.CreatedAt)
}

// TestMetadataIndexMongoDBModel_ToEntity_AllFalseFlags exercises the boolean wiring
// when both Unique and Sparse are false. Without an explicit assertion, the default
// zero-value of bool would mask a bug where the field was never assigned at all.
func TestMetadataIndexMongoDBModel_ToEntity_AllFalseFlags(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()

	model := &MetadataIndexMongoDBModel{
		ID:          id,
		EntityName:  "Operation",
		MetadataKey: "op_key",
		Unique:      false,
		Sparse:      false,
		CreatedAt:   indexFixedTime,
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.False(t, entity.Unique)
	assert.False(t, entity.Sparse)
}

// TestMetadataIndexMongoDBModel_FromEntity covers the inverse direction. The
// transaction-package implementation stamps CreatedAt with time.Now() (unlike the
// onboarding sibling, which copies). The test pins that distinction by allowing any
// recent timestamp, not the input.
func TestMetadataIndexMongoDBModel_FromEntity(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()

	entity := &MetadataIndex{
		ID:          id,
		EntityName:  "Asset",
		MetadataKey: "asset_key",
		Unique:      false,
		Sparse:      true,
		CreatedAt:   indexFixedTime, // intentionally not preserved by FromEntity
	}

	model := &MetadataIndexMongoDBModel{}
	beforeCall := time.Now().UTC()

	err := model.FromEntity(entity)
	require.NoError(t, err)

	afterCall := time.Now().UTC()

	assert.Equal(t, id, model.ID)
	assert.Equal(t, "Asset", model.EntityName)
	assert.Equal(t, "asset_key", model.MetadataKey)
	assert.False(t, model.Unique)
	assert.True(t, model.Sparse)

	// FromEntity overwrites CreatedAt with time.Now() — so it must NOT equal the input
	// (the input is a fixed past time) and MUST fall inside the [before, after] window.
	assert.NotEqual(t, indexFixedTime, model.CreatedAt,
		"CreatedAt must be stamped fresh, not copied from the entity")
	assert.True(t, !model.CreatedAt.Before(beforeCall) && !model.CreatedAt.After(afterCall),
		"CreatedAt must fall inside the [beforeCall, afterCall] window")
}

// TestMetadataMongoDBModel_ToEntity_TransactionPackage covers the document-level
// metadata translation in the transaction package. The onboarding package has its own
// equivalent test; the implementations are independent files that can diverge silently.
func TestMetadataMongoDBModel_ToEntity_TransactionPackage(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()

	model := &MetadataMongoDBModel{
		ID:         id,
		EntityID:   "tx-789",
		EntityName: "Transaction",
		Data:       JSON{"hash": "abc123"},
		CreatedAt:  indexFixedTime,
		UpdatedAt:  indexFixedTime.Add(time.Minute),
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID)
	assert.Equal(t, "tx-789", entity.EntityID)
	assert.Equal(t, "Transaction", entity.EntityName)
	assert.Equal(t, JSON{"hash": "abc123"}, entity.Data)
	assert.Equal(t, indexFixedTime, entity.CreatedAt)
	assert.Equal(t, indexFixedTime.Add(time.Minute), entity.UpdatedAt)
}
