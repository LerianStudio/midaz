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

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	"github.com/google/uuid"
)

// benchSink prevents compiler optimization of benchmark results.
var benchSink any

// =============================================================================
// BENCHMARK TESTS - MongoDB Metadata CreateBulk
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

			for b.Loop() {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(bm.batchSize)
				b.StartTimer()

				result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}

				benchSink = result
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

	for b.Loop() {
		b.StopTimer()
		metadata := createBenchmarkMetadataBatch(batchSize)
		b.StartTimer()

		result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
		if err != nil {
			b.Fatalf("CreateBulk failed: %v", err)
		}

		totalDocuments += result.Inserted
		benchSink = result
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

			for b.Loop() {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(bm.count)
				b.StartTimer()

				for _, m := range metadata {
					err := infra.repo.Create(ctx, "benchmark_entity", m)
					if err != nil {
						b.Fatalf("Create failed: %v", err)
					}
				}

				benchSink = metadata
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

			for b.Loop() {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(testSize)
				b.StartTimer()

				for _, m := range metadata {
					err := infra.repo.Create(ctx, "benchmark_entity", m)
					if err != nil {
						b.Fatalf("Create failed: %v", err)
					}
				}

				benchSink = metadata
			}
		})

		b.Run(fmt.Sprintf("Size%d_Bulk", testSize), func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for b.Loop() {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(testSize)
				b.StartTimer()

				result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}

				benchSink = result
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

			for b.Loop() {
				var wg sync.WaitGroup

				var errOnce sync.Once

				var firstErr error

				for g := 0; g < bm.goroutines; g++ {
					wg.Add(1)

					go func() {
						defer wg.Done()

						metadata := createBenchmarkMetadataBatch(bm.batchSize)

						_, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
						if err != nil {
							errOnce.Do(func() { firstErr = err })
						}
					}()
				}

				wg.Wait()

				if firstErr != nil {
					b.Fatalf("CreateBulk failed: %v", firstErr)
				}
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
	}{
		{"SingleChunk_500", 500},
		{"SingleChunk_1000", 1000},
		{"TwoChunks_1500", 1500},
		{"TwoChunks_2000", 2000},
		{"ThreeChunks_2500", 2500},
		{"TenChunks_10000", 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mongotestutil.ClearCollection(b, infra.container.Database, "benchmark_entity")

			b.ResetTimer()

			for b.Loop() {
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

				benchSink = result
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

			iterCount := 0
			for b.Loop() {
				b.StopTimer()

				// Create batch with some duplicates and some new
				batch := make([]*Metadata, batchSize)

				// Copy duplicates
				for j := 0; j < duplicateCount; j++ {
					batch[j] = duplicates[j]
				}

				// Create new documents
				for j := duplicateCount; j < batchSize; j++ {
					batch[j] = createBenchmarkMetadata(10000 + iterCount*batchSize + j)
				}

				b.StartTimer()

				result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", batch)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}

				benchSink = result
				iterCount++
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

			for b.Loop() {
				b.StopTimer()
				metadata := createBenchmarkMetadataBatch(bm.batchSize)
				b.StartTimer()

				result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}

				benchSink = result
			}
		})
	}
}

// =============================================================================
// Idempotency Benchmarks
// =============================================================================

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

	for b.Loop() {
		// Re-insert the same documents (should be idempotent - no new inserts)
		result, err := infra.repo.CreateBulk(ctx, "benchmark_entity", metadata)
		if err != nil {
			b.Fatalf("CreateBulk failed: %v", err)
		}

		if result.Inserted != 0 {
			b.Fatalf("expected 0 inserted (idempotent), got %d", result.Inserted)
		}

		benchSink = result
	}
}
