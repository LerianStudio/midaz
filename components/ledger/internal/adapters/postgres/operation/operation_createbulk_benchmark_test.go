//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// =============================================================================
// BENCHMARK TESTS - Operation CreateBulk
// =============================================================================

// benchmarkInfra holds infrastructure for benchmarks.
type benchmarkInfra struct {
	container     *pgtestutil.ContainerResult
	repo          *OperationPostgreSQLRepository
	orgID         uuid.UUID
	ledgerID      uuid.UUID
	accountID     uuid.UUID
	balanceID     uuid.UUID
	transactionID uuid.UUID
}

// setupBenchmarkInfra creates infrastructure for benchmark tests.
func setupBenchmarkInfra(b *testing.B) *benchmarkInfra {
	b.Helper()

	t := &testing.T{}
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	// Create required entities
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// Create balance (required FK)
	balanceParams := pgtestutil.DefaultBalanceParams()
	balanceParams.Alias = "@benchmark-balance"
	balanceParams.AssetCode = "USD"
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, balanceParams)

	// Create transaction (required FK)
	transactionID := pgtestutil.CreateTestTransactionWithStatus(t, container.DB, orgID, ledgerID, "APPROVED", decimal.NewFromInt(1000000), "USD")

	return &benchmarkInfra{
		container:     container,
		repo:          repo,
		orgID:         orgID,
		ledgerID:      ledgerID,
		accountID:     accountID,
		balanceID:     balanceID,
		transactionID: transactionID,
	}
}

// createBenchmarkOperation creates an operation for benchmarking.
func createBenchmarkOperation(infra *benchmarkInfra, index int) *Operation {
	opID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	amount := decimal.NewFromInt(int64(100 + index))
	availableBefore := decimal.NewFromInt(1000000)
	onHoldBefore := decimal.Zero
	availableAfter := availableBefore.Sub(amount)
	onHoldAfter := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)

	return &Operation{
		ID:              opID.String(),
		TransactionID:   infra.transactionID.String(),
		Description:     fmt.Sprintf("Benchmark operation %d", index),
		Type:            "DEBIT",
		AssetCode:       "USD",
		ChartOfAccounts: "1000",
		Amount:          Amount{Value: &amount},
		Balance: Balance{
			Available: &availableBefore,
			OnHold:    &onHoldBefore,
			Version:   &versionBefore,
		},
		BalanceAfter: Balance{
			Available: &availableAfter,
			OnHold:    &onHoldAfter,
			Version:   &versionAfter,
		},
		Status:          Status{Code: "APPROVED"},
		AccountID:       infra.accountID.String(),
		AccountAlias:    "@benchmark-account",
		BalanceKey:      "default",
		BalanceID:       infra.balanceID.String(),
		OrganizationID:  infra.orgID.String(),
		LedgerID:        infra.ledgerID.String(),
		BalanceAffected: true,
		Direction:       "debit",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// createBenchmarkBatch creates a batch of operations for benchmarking.
func createBenchmarkBatch(infra *benchmarkInfra, count int) []*Operation {
	operations := make([]*Operation, count)
	for i := 0; i < count; i++ {
		operations[i] = createBenchmarkOperation(infra, i)
	}

	return operations
}

// BenchmarkOperation_CreateBulk_BatchSizes benchmarks CreateBulk with different batch sizes.
func BenchmarkOperation_CreateBulk_BatchSizes(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"BatchSize10", 10},
		{"BatchSize50", 50},
		{"BatchSize100", 100},
		{"BatchSize500", 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				operations := createBenchmarkBatch(infra, bm.batchSize)
				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, operations)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkOperation_CreateBulk_Throughput measures operations per second.
func BenchmarkOperation_CreateBulk_Throughput(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	const batchSize = 100

	b.ResetTimer()
	b.ReportAllocs()

	totalOperations := 0

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		operations := createBenchmarkBatch(infra, batchSize)
		b.StartTimer()

		result, err := infra.repo.CreateBulk(ctx, operations)
		if err != nil {
			b.Fatalf("CreateBulk failed: %v", err)
		}

		totalOperations += int(result.Inserted)
	}

	b.ReportMetric(float64(totalOperations)/b.Elapsed().Seconds(), "ops/sec")
}

// BenchmarkOperation_Create_Individual benchmarks individual Create calls for comparison.
func BenchmarkOperation_Create_Individual(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name  string
		count int
	}{
		{"Count10", 10},
		{"Count50", 50},
		{"Count100", 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				operations := createBenchmarkBatch(infra, bm.count)
				b.StartTimer()

				for _, op := range operations {
					_, err := infra.repo.Create(ctx, op)
					if err != nil {
						b.Fatalf("Create failed: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkOperation_CreateBulk_vs_Individual compares bulk vs individual inserts.
// This benchmark demonstrates the performance advantage of bulk operations.
func BenchmarkOperation_CreateBulk_vs_Individual(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	const testSize = 50

	b.Run("Individual", func(b *testing.B) {
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			b.StopTimer()
			operations := createBenchmarkBatch(infra, testSize)
			b.StartTimer()

			for _, op := range operations {
				_, err := infra.repo.Create(ctx, op)
				if err != nil {
					b.Fatalf("Create failed: %v", err)
				}
			}
		}
	})

	b.Run("Bulk", func(b *testing.B) {
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			b.StopTimer()
			operations := createBenchmarkBatch(infra, testSize)
			b.StartTimer()

			_, err := infra.repo.CreateBulk(ctx, operations)
			if err != nil {
				b.Fatalf("CreateBulk failed: %v", err)
			}
		}
	})
}

// BenchmarkOperation_CreateBulk_Concurrent benchmarks concurrent bulk inserts.
func BenchmarkOperation_CreateBulk_Concurrent(b *testing.B) {
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
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				for g := 0; g < bm.goroutines; g++ {
					wg.Add(1)

					go func() {
						defer wg.Done()

						operations := createBenchmarkBatch(infra, bm.batchSize)

						_, err := infra.repo.CreateBulk(ctx, operations)
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

// BenchmarkOperation_CreateBulk_Chunking benchmarks large batches that require chunking.
// Operation has 30 columns, so chunks are 1000 rows each.
func BenchmarkOperation_CreateBulk_Chunking(b *testing.B) {
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
		{"ThreeChunks_2500", 2500, 3},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				operations := createBenchmarkBatch(infra, bm.batchSize)
				b.StartTimer()

				result, err := infra.repo.CreateBulk(ctx, operations)
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

// BenchmarkOperation_CreateBulk_DuplicateRatio benchmarks with varying duplicate ratios.
func BenchmarkOperation_CreateBulk_DuplicateRatio(b *testing.B) {
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
			// Pre-insert the duplicate operations
			b.StopTimer()
			duplicateCount := int(float64(batchSize) * bm.duplicateRatio)

			duplicates := createBenchmarkBatch(infra, duplicateCount)
			if duplicateCount > 0 {
				_, err := infra.repo.CreateBulk(ctx, duplicates)
				if err != nil {
					b.Fatalf("failed to pre-insert duplicates: %v", err)
				}
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Create batch with some duplicates and some new
				batch := make([]*Operation, batchSize)

				// Copy duplicates
				for j := 0; j < duplicateCount; j++ {
					batch[j] = duplicates[j]
				}

				// Create new operations
				for j := duplicateCount; j < batchSize; j++ {
					batch[j] = createBenchmarkOperation(infra, 10000+i*batchSize+j)
				}

				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, batch)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkOperation_CreateBulk_MemoryAllocation focuses on memory allocation patterns.
func BenchmarkOperation_CreateBulk_MemoryAllocation(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"Small_10", 10},
		{"Medium_100", 100},
		{"Large_500", 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				operations := createBenchmarkBatch(infra, bm.batchSize)
				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, operations)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkOperation_CreateBulk_FieldCount demonstrates impact of Operation's 30 columns
// compared to simpler entities. This benchmark helps understand query building overhead.
func BenchmarkOperation_CreateBulk_FieldCount(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	// Operation has 30 columns, testing query building impact at different sizes
	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"10ops_300params", 10},     // 10 × 30 = 300 params
		{"50ops_1500params", 50},    // 50 × 30 = 1,500 params
		{"100ops_3000params", 100},  // 100 × 30 = 3,000 params
		{"500ops_15000params", 500}, // 500 × 30 = 15,000 params
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				operations := createBenchmarkBatch(infra, bm.batchSize)
				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, operations)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}
