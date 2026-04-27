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
		EntityName:  "Account",
		MetadataKey: "external_id",
		Unique:      true,
		Sparse:      false,
		CreatedAt:   indexFixedTime,
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID)
	assert.Equal(t, "Account", entity.EntityName)
	assert.Equal(t, "external_id", entity.MetadataKey)
	assert.True(t, entity.Unique)
	assert.False(t, entity.Sparse)
	assert.Equal(t, indexFixedTime, entity.CreatedAt)
}

// TestMetadataIndexMongoDBModel_ToEntity_AllFalseFlags is the inverse-flag variant —
// confirms boolean field assignment is wired correctly when both flags are false rather
// than depending on default zero-value behaviour to mask a bug.
func TestMetadataIndexMongoDBModel_ToEntity_AllFalseFlags(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()

	model := &MetadataIndexMongoDBModel{
		ID:          id,
		EntityName:  "Ledger",
		MetadataKey: "ledger_key",
		Unique:      false,
		Sparse:      false,
		CreatedAt:   indexFixedTime,
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.False(t, entity.Unique)
	assert.False(t, entity.Sparse)
}

// TestMetadataIndexMongoDBModel_FromEntity covers the inverse direction. Note: the
// onboarding implementation copies CreatedAt verbatim (the transaction implementation
// stamps time.Now() internally — different rationale, different test in that package).
func TestMetadataIndexMongoDBModel_FromEntity(t *testing.T) {
	t.Parallel()

	id := primitive.NewObjectID()

	entity := &MetadataIndex{
		ID:          id,
		EntityName:  "Transaction",
		MetadataKey: "trace_id",
		Unique:      false,
		Sparse:      true,
		CreatedAt:   indexFixedTime,
	}

	model := &MetadataIndexMongoDBModel{}
	err := model.FromEntity(entity)

	require.NoError(t, err)
	assert.Equal(t, id, model.ID)
	assert.Equal(t, "Transaction", model.EntityName)
	assert.Equal(t, "trace_id", model.MetadataKey)
	assert.False(t, model.Unique)
	assert.True(t, model.Sparse)
	assert.Equal(t, indexFixedTime, model.CreatedAt)
}

// TestMetadataIndexMongoDBModel_RoundTrip asserts FromEntity ∘ ToEntity is the identity
// for every field. This guards against drift where a field gets renamed on one side
// without the other — the diff fails immediately with a useful diff.
func TestMetadataIndexMongoDBModel_RoundTrip(t *testing.T) {
	t.Parallel()

	original := &MetadataIndex{
		ID:          primitive.NewObjectID(),
		EntityName:  "Asset",
		MetadataKey: "asset_code_hash",
		Unique:      true,
		Sparse:      true,
		CreatedAt:   indexFixedTime,
	}

	model := &MetadataIndexMongoDBModel{}
	require.NoError(t, model.FromEntity(original))

	round := model.ToEntity()
	assert.Equal(t, original, round, "round-trip must preserve every field exactly")
}
