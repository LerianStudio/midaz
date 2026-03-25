//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	"github.com/google/uuid"
)

// =============================================================================
// BENCHMARK TESTS - MongoDB Metadata CreateBulk and UpdateBulk
// =============================================================================

// benchmarkInfra holds infrastructure for benchmarks.
type benchmarkInfra struct {
	container *mongotestutil.ContainerResult
	repo      *MetadataMongoDBRepository
}

// setupBenchmarkInfra creates infrastructure for benchmark tests.
func setupBenchmarkInfra(b *testing.B) *benchmarkInfra {
	b.Helper()

	container := mongotestutil.SetupContainer(b)
	conn := mongotestutil.CreateConnection(b, container.URI, container.DBName)
	repo := NewMetadataMongoDBRepository(conn)

	return &benchmarkInfra{
		container: container,
		repo:      repo,
	}
}

// createBenchmarkMetadata creates a metadata entity for benchmarking.
func createBenchmarkMetadata(index int) *Metadata {
	entityID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	now := time.Now().Truncate(time.Microsecond)

	return &Metadata{
		EntityID:   entityID,
		EntityName: "benchmark_entity",
		Data: JSON{
			"index":       index,
			"name":        fmt.Sprintf("Benchmark Metadata %d", index),
			"description": "Test metadata for benchmark",
			"tags":        []string{"benchmark", "test", "bulk"},
			"nested": map[string]any{
				"field1": "value1",
				"field2": index,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// createBenchmarkMetadataBatch creates a batch of metadata entities for benchmarking.
func createBenchmarkMetadataBatch(count int) []*Metadata {
	metadata := make([]*Metadata, count)
	for i := 0; i < count; i++ {
		metadata[i] = createBenchmarkMetadata(i)
	}

	return metadata
}

// createBenchmarkUpdateBatch creates a batch of update operations for benchmarking.
func createBenchmarkUpdateBatch(metadata []*Metadata) []MetadataBulkUpdate {
	updates := make([]MetadataBulkUpdate, len(metadata))
	for i, m := range metadata {
		updates[i] = MetadataBulkUpdate{
			EntityID: m.EntityID,
			Data: map[string]any{
				"index":      i,
				"updated":    true,
				"updated_at": time.Now().Format(time.RFC3339),
			},
		}
	}

	return updates
}

// =============================================================================
// CreateBulk Benchmarks
// =============================================================================

// BenchmarkMetadata_CreateBulk_BatchSizes benchmarks CreateBulk with different batch sizes.
func BenchmarkMetadata_CreateBulk_BatchSizes(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"BatchSize10", 10},
		{"BatchSize20", 20},
		{"BatchSize50", 50},
		{"BatchSize100", 100},
		{"BatchSize1000", 1000},
		{"BatchSize10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Clear collection before each benchmark
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(bm.batchSize)
				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMetadata_CreateBulk_Throughput measures documents per second.
func BenchmarkMetadata_CreateBulk_Throughput(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	const batchSize = 100

	mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

	b.ResetTimer()
	b.ReportAllocs()

	totalDocuments := int64(0)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		metadata := createBenchmarkMetadataBatch(batchSize)
		b.StartTimer()

		result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
		if err != nil {
			b.Fatalf("CreateBulk failed: %v", err)
		}

		totalDocuments += result.Inserted
	}

	b.ReportMetric(float64(totalDocuments)/b.Elapsed().Seconds(), "docs/sec")
}

// BenchmarkMetadata_Create_Individual benchmarks individual Create calls for comparison.
func BenchmarkMetadata_Create_Individual(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name  string
		count int
	}{
		{"Count10", 10},
		{"Count20", 20},
		{"Count50", 50},
		{"Count100", 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(bm.count)
				b.StartTimer()

				for _, m := range metadata {
					err := infra.repo.Create(ctx, "benchmark_entity", m)
					if err != nil {
						b.Fatalf("Create failed: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkMetadata_CreateBulk_vs_Individual compares bulk vs individual inserts.
func BenchmarkMetadata_CreateBulk_vs_Individual(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	testSizes := []int{10, 50, 100}

	for _, testSize := range testSizes {
		b.Run(fmt.Sprintf("Size%d_Individual", testSize), func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(testSize)
				b.StartTimer()

				for _, m := range metadata {
					err := infra.repo.Create(ctx, "benchmark_entity", m)
					if err != nil {
						b.Fatalf("Create failed: %v", err)
					}
				}
			}
		})

		b.Run(fmt.Sprintf("Size%d_Bulk", testSize), func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(testSize)
				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMetadata_CreateBulk_Concurrent benchmarks concurrent bulk inserts.
func BenchmarkMetadata_CreateBulk_Concurrent(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name       string
		goroutines int
		batchSize  int
	}{
		{"Goroutines2_Batch50", 2, 50},
		{"Goroutines4_Batch50", 4, 50},
		{"Goroutines8_Batch50", 8, 50},
		{"Goroutines4_Batch100", 4, 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				for g := 0; g < bm.goroutines; g++ {
					wg.Add(1)

					go func() {
						defer wg.Done()

						metadata := createBenchmarkMetadataBatch(bm.batchSize)

						_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
						if err != nil {
							b.Errorf("CreateBulk failed: %v", err)
						}
					}()
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkMetadata_CreateBulk_Chunking benchmarks large batches that require chunking.
// MongoDB BulkWrite chunks at 1000 documents.
func BenchmarkMetadata_CreateBulk_Chunking(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
		chunks    int // Expected number of chunks
	}{
		{"SingleChunk_500", 500, 1},
		{"SingleChunk_1000", 1000, 1},
		{"TwoChunks_1500", 1500, 2},
		{"TwoChunks_2000", 2000, 2},
		{"ThreeChunks_2500", 2500, 3},
		{"TenChunks_10000", 10000, 10},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(bm.batchSize)
				b.StartTimer()

				result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}

				if result.Inserted != int64(bm.batchSize) {
					b.Fatalf("expected %d inserted, got %d", bm.batchSize, result.Inserted)
				}
			}
		})
	}
}

// BenchmarkMetadata_CreateBulk_DuplicateRatio benchmarks with varying duplicate ratios.
func BenchmarkMetadata_CreateBulk_DuplicateRatio(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	const batchSize = 100

	benchmarks := []struct {
		name           string
		duplicateRatio float64 // 0.0 = no duplicates, 1.0 = all duplicates
	}{
		{"NoDuplicates", 0.0},
		{"25PercentDuplicates", 0.25},
		{"50PercentDuplicates", 0.50},
		{"75PercentDuplicates", 0.75},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			// Pre-insert the duplicate documents
			b.StopTimer()
			duplicateCount := int(float64(batchSize) * bm.duplicateRatio)

			var duplicates []*Metadata
			if duplicateCount > 0 {
				duplicates = createBenchmarkMetadataBatch(duplicateCount)
				_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", duplicates)
				if err != nil {
					b.Fatalf("failed to pre-insert duplicates: %v", err)
				}
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Create batch with some duplicates and some new
				batch := make([]*Metadata, batchSize)

				// Copy duplicates
				for j := 0; j < duplicateCount; j++ {
					batch[j] = duplicates[j]
				}

				// Create new documents
				for j := duplicateCount; j < batchSize; j++ {
					batch[j] = createBenchmarkMetadata(10000 + i*batchSize + j)
				}

				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", batch)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMetadata_CreateBulk_MemoryAllocation focuses on memory allocation patterns.
func BenchmarkMetadata_CreateBulk_MemoryAllocation(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"Small_10", 10},
		{"Medium_100", 100},
		{"Large_1000", 1000},
		{"XLarge_10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(bm.batchSize)
				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}

// =============================================================================
// UpdateBulk Benchmarks
// =============================================================================

// BenchmarkMetadata_UpdateBulk_BatchSizes benchmarks UpdateBulk with different batch sizes.
func BenchmarkMetadata_UpdateBulk_BatchSizes(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"BatchSize10", 10},
		{"BatchSize20", 20},
		{"BatchSize50", 50},
		{"BatchSize100", 100},
		{"BatchSize1000", 1000},
		{"BatchSize10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			// Pre-insert documents to update
			b.StopTimer()
			metadata := createBenchmarkMetadataBatch(bm.batchSize)
			_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
			if err != nil {
				b.Fatalf("failed to pre-insert documents: %v", err)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				updates := createBenchmarkUpdateBatch(metadata)
				b.StartTimer()

				_, err := infra.repo.UpdateBulk(ctx, "benchmark_entity", updates)
				if err != nil {
					b.Fatalf("UpdateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMetadata_UpdateBulk_Throughput measures updates per second.
func BenchmarkMetadata_UpdateBulk_Throughput(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	const batchSize = 100

	mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

	// Pre-insert documents
	metadata := createBenchmarkMetadataBatch(batchSize)
	_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
	if err != nil {
		b.Fatalf("failed to pre-insert documents: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	totalUpdates := int64(0)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		updates := createBenchmarkUpdateBatch(metadata)
		b.StartTimer()

		result, err := infra.repo.UpdateBulk(ctx, "benchmark_entity", updates)
		if err != nil {
			b.Fatalf("UpdateBulk failed: %v", err)
		}

		totalUpdates += result.Modified + result.Matched
	}

	b.ReportMetric(float64(totalUpdates)/b.Elapsed().Seconds(), "updates/sec")
}

// BenchmarkMetadata_Update_Individual benchmarks individual Update calls for comparison.
func BenchmarkMetadata_Update_Individual(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name  string
		count int
	}{
		{"Count10", 10},
		{"Count20", 20},
		{"Count50", 50},
		{"Count100", 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			// Pre-insert documents
			b.StopTimer()
			metadata := createBenchmarkMetadataBatch(bm.count)
			_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
			if err != nil {
				b.Fatalf("failed to pre-insert documents: %v", err)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				updates := createBenchmarkUpdateBatch(metadata)
				b.StartTimer()

				for _, u := range updates {
					err := infra.repo.Update(ctx, "benchmark_entity", u.EntityID, u.Data)
					if err != nil {
						b.Fatalf("Update failed: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkMetadata_UpdateBulk_vs_Individual compares bulk vs individual updates.
func BenchmarkMetadata_UpdateBulk_vs_Individual(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	testSizes := []int{10, 50, 100}

	for _, testSize := range testSizes {
		b.Run(fmt.Sprintf("Size%d_Individual", testSize), func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			// Pre-insert documents
			metadata := createBenchmarkMetadataBatch(testSize)
			_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
			if err != nil {
				b.Fatalf("failed to pre-insert documents: %v", err)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				updates := createBenchmarkUpdateBatch(metadata)
				b.StartTimer()

				for _, u := range updates {
					err := infra.repo.Update(ctx, "benchmark_entity", u.EntityID, u.Data)
					if err != nil {
						b.Fatalf("Update failed: %v", err)
					}
				}
			}
		})

		b.Run(fmt.Sprintf("Size%d_Bulk", testSize), func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			// Pre-insert documents
			metadata := createBenchmarkMetadataBatch(testSize)
			_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
			if err != nil {
				b.Fatalf("failed to pre-insert documents: %v", err)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				updates := createBenchmarkUpdateBatch(metadata)
				b.StartTimer()

				_, err := infra.repo.UpdateBulk(ctx, "benchmark_entity", updates)
				if err != nil {
					b.Fatalf("UpdateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMetadata_UpdateBulk_Concurrent benchmarks concurrent bulk updates.
func BenchmarkMetadata_UpdateBulk_Concurrent(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name       string
		goroutines int
		batchSize  int
	}{
		{"Goroutines2_Batch50", 2, 50},
		{"Goroutines4_Batch50", 4, 50},
		{"Goroutines8_Batch50", 8, 50},
		{"Goroutines4_Batch100", 4, 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			// Pre-insert documents for each goroutine
			allMetadata := make([][]*Metadata, bm.goroutines)
			for g := 0; g < bm.goroutines; g++ {
				metadata := createBenchmarkMetadataBatch(bm.batchSize)
				_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("failed to pre-insert documents: %v", err)
				}
				allMetadata[g] = metadata
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				for g := 0; g < bm.goroutines; g++ {
					wg.Add(1)

					go func(goroutineID int) {
						defer wg.Done()

						updates := createBenchmarkUpdateBatch(allMetadata[goroutineID])

						_, err := infra.repo.UpdateBulk(ctx, "benchmark_entity", updates)
						if err != nil {
							b.Errorf("UpdateBulk failed: %v", err)
						}
					}(g)
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkMetadata_UpdateBulk_Chunking benchmarks large batches that require chunking.
func BenchmarkMetadata_UpdateBulk_Chunking(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
		chunks    int // Expected number of chunks
	}{
		{"SingleChunk_500", 500, 1},
		{"SingleChunk_1000", 1000, 1},
		{"TwoChunks_1500", 1500, 2},
		{"TwoChunks_2000", 2000, 2},
		{"ThreeChunks_2500", 2500, 3},
		{"TenChunks_10000", 10000, 10},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			// Pre-insert documents
			b.StopTimer()
			metadata := createBenchmarkMetadataBatch(bm.batchSize)
			_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
			if err != nil {
				b.Fatalf("failed to pre-insert documents: %v", err)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				updates := createBenchmarkUpdateBatch(metadata)
				b.StartTimer()

				result, err := infra.repo.UpdateBulk(ctx, "benchmark_entity", updates)
				if err != nil {
					b.Fatalf("UpdateBulk failed: %v", err)
				}

				if result.Matched != int64(bm.batchSize) {
					b.Fatalf("expected %d matched, got %d", bm.batchSize, result.Matched)
				}
			}
		})
	}
}

// BenchmarkMetadata_UpdateBulk_MemoryAllocation focuses on memory allocation patterns.
func BenchmarkMetadata_UpdateBulk_MemoryAllocation(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"Small_10", 10},
		{"Medium_100", 100},
		{"Large_1000", 1000},
		{"XLarge_10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			// Pre-insert documents
			metadata := createBenchmarkMetadataBatch(bm.batchSize)
			_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
			if err != nil {
				b.Fatalf("failed to pre-insert documents: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				updates := createBenchmarkUpdateBatch(metadata)
				b.StartTimer()

				_, err := infra.repo.UpdateBulk(ctx, "benchmark_entity", updates)
				if err != nil {
					b.Fatalf("UpdateBulk failed: %v", err)
				}
			}
		})
	}
}

// =============================================================================
// Combined Benchmarks
// =============================================================================

// BenchmarkMetadata_CreateThenUpdate benchmarks a typical workflow of creating and updating.
func BenchmarkMetadata_CreateThenUpdate(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"BatchSize10", 10},
		{"BatchSize50", 50},
		{"BatchSize100", 100},
		{"BatchSize1000", 1000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(bm.batchSize)
				b.StartTimer()

				// Create
				_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}

				// Update
				updates := createBenchmarkUpdateBatch(metadata)
				_, err = infra.repo.UpdateBulk(ctx, "benchmark_entity", updates)
				if err != nil {
					b.Fatalf("UpdateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMetadata_Idempotency benchmarks repeated CreateBulk calls (idempotency).
func BenchmarkMetadata_Idempotency(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	const batchSize = 100

	mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

	// Pre-insert documents
	metadata := createBenchmarkMetadataBatch(batchSize)
	_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
	if err != nil {
		b.Fatalf("failed to pre-insert documents: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Re-insert the same documents (should be idempotent - no new inserts)
		result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
		if err != nil {
			b.Fatalf("CreateBulk failed: %v", err)
		}

		if result.Inserted != 0 {
			b.Fatalf("expected 0 inserted (idempotent), got %d", result.Inserted)
		}
	}
}
