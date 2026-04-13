//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

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
// BENCHMARK TESTS - Transaction CreateBulk
// =============================================================================

// benchmarkInfra holds infrastructure for benchmarks.
type benchmarkInfra struct {
	container *pgtestutil.ContainerResult
	repo      *TransactionPostgreSQLRepository
	orgID     uuid.UUID
	ledgerID  uuid.UUID
}

// setupBenchmarkInfra creates infrastructure for benchmark tests.
func setupBenchmarkInfra(b *testing.B) *benchmarkInfra {
	b.Helper()

	t := &testing.T{}
	container := pgtestutil.SetupContainer(t)

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, container.Config.DBName, migrationsPath)

	repo := NewTransactionPostgreSQLRepository(conn)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	return &benchmarkInfra{
		container: container,
		repo:      repo,
		orgID:     orgID,
		ledgerID:  ledgerID,
	}
}

// createBenchmarkTransaction creates a transaction for benchmarking.
func createBenchmarkTransaction(orgID, ledgerID uuid.UUID, index int) *Transaction {
	txID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)
	amount := decimal.NewFromInt(int64(1000 + index))

	return &Transaction{
		ID:                       txID.String(),
		Description:              fmt.Sprintf("Benchmark transaction %d", index),
		Status:                   Status{Code: "APPROVED"},
		Amount:                   &amount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "DEFAULT",
		LedgerID:                 ledgerID.String(),
		OrganizationID:           orgID.String(),
		CreatedAt:                now,
		UpdatedAt:                now,
	}
}

// createBenchmarkBatch creates a batch of transactions for benchmarking.
func createBenchmarkBatch(orgID, ledgerID uuid.UUID, count int) []*Transaction {
	transactions := make([]*Transaction, count)
	for i := 0; i < count; i++ {
		transactions[i] = createBenchmarkTransaction(orgID, ledgerID, i)
	}

	return transactions
}

// BenchmarkTransaction_CreateBulk_BatchSizes benchmarks CreateBulk with different batch sizes.
func BenchmarkTransaction_CreateBulk_BatchSizes(b *testing.B) {
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
				transactions := createBenchmarkBatch(infra.orgID, infra.ledgerID, bm.batchSize)
				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, transactions)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkTransaction_CreateBulk_Throughput measures transactions per second.
func BenchmarkTransaction_CreateBulk_Throughput(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	const batchSize = 100

	b.ResetTimer()
	b.ReportAllocs()

	totalTransactions := 0

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		transactions := createBenchmarkBatch(infra.orgID, infra.ledgerID, batchSize)
		b.StartTimer()

		result, err := infra.repo.CreateBulk(ctx, transactions)
		if err != nil {
			b.Fatalf("CreateBulk failed: %v", err)
		}

		totalTransactions += int(result.Inserted)
	}

	b.ReportMetric(float64(totalTransactions)/b.Elapsed().Seconds(), "tx/sec")
}

// BenchmarkTransaction_Create_Individual benchmarks individual Create calls for comparison.
func BenchmarkTransaction_Create_Individual(b *testing.B) {
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
				transactions := createBenchmarkBatch(infra.orgID, infra.ledgerID, bm.count)
				b.StartTimer()

				for _, tx := range transactions {
					_, err := infra.repo.Create(ctx, tx)
					if err != nil {
						b.Fatalf("Create failed: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkTransaction_CreateBulk_vs_Individual compares bulk vs individual inserts.
// This benchmark demonstrates the performance advantage of bulk operations.
func BenchmarkTransaction_CreateBulk_vs_Individual(b *testing.B) {
	infra := setupBenchmarkInfra(b)
	ctx := context.Background()

	const testSize = 50

	b.Run("Individual", func(b *testing.B) {
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			b.StopTimer()
			transactions := createBenchmarkBatch(infra.orgID, infra.ledgerID, testSize)
			b.StartTimer()

			for _, tx := range transactions {
				_, err := infra.repo.Create(ctx, tx)
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
			transactions := createBenchmarkBatch(infra.orgID, infra.ledgerID, testSize)
			b.StartTimer()

			_, err := infra.repo.CreateBulk(ctx, transactions)
			if err != nil {
				b.Fatalf("CreateBulk failed: %v", err)
			}
		}
	})
}

// BenchmarkTransaction_CreateBulk_Concurrent benchmarks concurrent bulk inserts.
func BenchmarkTransaction_CreateBulk_Concurrent(b *testing.B) {
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

						transactions := createBenchmarkBatch(infra.orgID, infra.ledgerID, bm.batchSize)

						_, err := infra.repo.CreateBulk(ctx, transactions)
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

// BenchmarkTransaction_CreateBulk_Chunking benchmarks large batches that require chunking.
func BenchmarkTransaction_CreateBulk_Chunking(b *testing.B) {
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
				transactions := createBenchmarkBatch(infra.orgID, infra.ledgerID, bm.batchSize)
				b.StartTimer()

				result, err := infra.repo.CreateBulk(ctx, transactions)
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

// BenchmarkTransaction_CreateBulk_DuplicateRatio benchmarks with varying duplicate ratios.
func BenchmarkTransaction_CreateBulk_DuplicateRatio(b *testing.B) {
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
			// Pre-insert the duplicate transactions
			b.StopTimer()
			duplicateCount := int(float64(batchSize) * bm.duplicateRatio)

			duplicates := createBenchmarkBatch(infra.orgID, infra.ledgerID, duplicateCount)
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
				batch := make([]*Transaction, batchSize)

				// Copy duplicates
				for j := 0; j < duplicateCount; j++ {
					batch[j] = duplicates[j]
				}

				// Create new transactions
				for j := duplicateCount; j < batchSize; j++ {
					batch[j] = createBenchmarkTransaction(infra.orgID, infra.ledgerID, 10000+i*batchSize+j)
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

// BenchmarkTransaction_CreateBulk_MemoryAllocation focuses on memory allocation patterns.
func BenchmarkTransaction_CreateBulk_MemoryAllocation(b *testing.B) {
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
				transactions := createBenchmarkBatch(infra.orgID, infra.ledgerID, bm.batchSize)
				b.StartTimer()

				_, err := infra.repo.CreateBulk(ctx, transactions)
				if err != nil {
					b.Fatalf("CreateBulk failed: %v", err)
				}
			}
		})
	}
}
