// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/mongotest"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// The repository is a thin driver wrapper with no unit seam for a successful call, so
// these tests drive the failure path against an unreachable server; the success path
// ("Upsert de metadado com sucesso") is covered by the integration suite.

// Scenario: Falha do driver fica no span da operação. The driver failure is a technical
// error on the single mongodb.create_metadata span; there is no .upsert child.
func TestMetadataSpans_DriverFailureStaysOnTheOperationSpan(t *testing.T) {
	t.Parallel()

	ctx, recorder := spantest.RecordingContext()
	ctx = tmcore.ContextWithMB(ctx, mongotest.UnreachableDatabase(t))

	err := (&MetadataMongoDBRepository{}).Create(ctx, "transaction", &Metadata{EntityID: "entity-1", EntityName: "Transaction"})
	require.Error(t, err)

	spantest.AssertNoSpan(t, recorder, "mongodb.create_metadata.upsert")
	assert.Equal(t, []string{"mongodb.create_metadata"}, spantest.SpanNames(recorder))

	span := spantest.FindSpan(t, recorder, "mongodb.create_metadata")
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Failed to upsert metadata")
}

// Every other repository operation is one span too, with the driver error on it.
func TestMetadataSpans_ReadsAndWritesOpenOneSpanPerOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(ctx context.Context, repo *MetadataMongoDBRepository) error
	}{
		{"mongodb.find_list", func(ctx context.Context, repo *MetadataMongoDBRepository) error {
			_, err := repo.FindList(ctx, "transaction", http.QueryHeader{})
			return err
		}},
		{"mongodb.create_index", func(ctx context.Context, repo *MetadataMongoDBRepository) error {
			_, err := repo.CreateIndex(ctx, "transaction", &mmodel.CreateMetadataIndexInput{MetadataKey: "k"})
			return err
		}},
		{"mongodb.find_by_entity", func(ctx context.Context, repo *MetadataMongoDBRepository) error {
			_, err := repo.FindByEntity(ctx, "transaction", "entity-1")
			return err
		}},
		{"mongodb.find_by_entity_ids", func(ctx context.Context, repo *MetadataMongoDBRepository) error {
			_, err := repo.FindByEntityIDs(ctx, "transaction", []string{"entity-1"})
			return err
		}},
		{"mongodb.update_metadata", func(ctx context.Context, repo *MetadataMongoDBRepository) error {
			return repo.Update(ctx, "transaction", "entity-1", map[string]any{"k": "v"})
		}},
		{"mongodb.delete_metadata", func(ctx context.Context, repo *MetadataMongoDBRepository) error {
			return repo.Delete(ctx, "transaction", "entity-1")
		}},
		{"mongodb.find_all_indexes", func(ctx context.Context, repo *MetadataMongoDBRepository) error {
			_, err := repo.FindAllIndexes(ctx, "transaction")
			return err
		}},
		{"mongodb.delete_index", func(ctx context.Context, repo *MetadataMongoDBRepository) error {
			return repo.DeleteIndex(ctx, "transaction", "metadata.k_1")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, recorder := spantest.RecordingContext()
			ctx = tmcore.ContextWithMB(ctx, mongotest.UnreachableDatabase(t))

			require.Error(t, tt.call(ctx, &MetadataMongoDBRepository{}))

			assert.Equal(t, []string{tt.name}, spantest.SpanNames(recorder), "one span per operation, no driver child")
			assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, tt.name).Status().Code)
		})
	}
}

// A bulk insert is one mongodb.create_bulk_metadata span: each BulkWrite chunk opens no span
// of its own, and a chunk failure is recorded once, with its step, on the bulk span.
func TestMetadataSpans_BulkChunkFailureIsRecordedOnceOnTheBulkSpan(t *testing.T) {
	t.Parallel()

	ctx, recorder := spantest.RecordingContext()
	ctx = tmcore.ContextWithMB(ctx, mongotest.UnreachableDatabase(t))

	result, err := (&MetadataMongoDBRepository{}).CreateBulk(ctx, "transaction", []*Metadata{
		{EntityID: "entity-1", EntityName: "Transaction"},
		{EntityID: "entity-2", EntityName: "Transaction"},
	})
	require.Error(t, err)
	require.NotNil(t, result, "the partial result is still returned")
	assert.Equal(t, int64(2), result.Attempted)

	spantest.AssertNoSpan(t, recorder, "mongodb.insert_metadata_chunk")
	assert.Equal(t, []string{"mongodb.create_bulk_metadata"}, spantest.SpanNames(recorder))

	span := spantest.FindSpan(t, recorder, "mongodb.create_bulk_metadata")
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "BulkWrite failed")

	exceptions := 0

	for _, event := range span.Events() {
		if event.Name == "exception" {
			exceptions++
		}
	}

	assert.Equal(t, 1, exceptions, "the chunk failure must be recorded exactly once")
}
