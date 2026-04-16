//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
)

// BenchmarkTransactionsContention_HotBalance measures realistic write-path
// contention on a single balance row under parallel DEBIT load.
//
// The benchmark models the worst-case authorize scenario: one balance with a
// finite Available (1000 units) being drained by 1000 concurrent 1-unit
// debits. This exercises the exact code paths that CREDIT-only benchmarks
// sidestep — optimistic-version collision, deterministic locking, and the
// retry loop for 40001/40P01/55P03. A CREDIT-only benchmark starting at 1e12
// would never reach Available=0 and therefore never produce the version-churn
// hotspot that production sees when an account is being drained aggressively.
//
// The benchmark reports three derived metrics:
//   - attempts/sec — parallel-throughput proxy (concurrency / iteration wall-clock)
//   - retries/success — average retries per successful debit (higher = more contention)
//   - failures — total unrecoverable errors (must stay 0 for a healthy system)
//
// It is intentionally a benchmark (not a Test) so it participates in
// `go test -tags=integration -run=^$ -bench=...` runs rather than per-commit
// CI; baseline numbers are recorded in the commit message for later comparison.
//
// Companion to Batch A's authorizer-side hot-balance benchmark. Where that
// benchmark measures in-memory lock contention, this one measures the end-to-
// end PostgreSQL write path (including pgx connection overhead, SELECT FOR
// UPDATE deterministic ordering, and the UPDATE VALUES query) so the two
// numbers together give an honest picture of the hot-path envelope.
//
//nolint:funlen,gocognit // benchmark end-to-end lifecycle is clearer as a single function
func BenchmarkTransactionsContention_HotBalance(b *testing.B) {
	const (
		concurrency    = 1000
		initialBalance = 1000
		debitAmount    = 1
	)

	container := pgtestutil.SetupContainer(b)
	repo := createBenchRepository(b, container)

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	balanceID := seedHotBalance(b, container, orgID, ledgerID, accountID, "@hot", initialBalance)

	ctx := context.Background()

	var (
		totalSuccesses atomic.Int64
		totalRetries   atomic.Int64
		totalFailures  atomic.Int64
	)

	b.ResetTimer()

	for iter := 0; iter < b.N; iter++ {
		// Reset to initialBalance, version=0 at the start of each iteration so
		// repeated iterations under `-benchtime=Nx` observe identical contention.
		resetHotBalance(b, container, balanceID, initialBalance)

		var wg sync.WaitGroup

		wg.Add(concurrency)

		start := make(chan struct{})

		for worker := 0; worker < concurrency; worker++ {
			go func() {
				defer wg.Done()
				<-start

				// Each worker attempts a single 1-unit DEBIT with optimistic
				// versioning: read current version, compute new Available,
				// BalancesUpdate with Version+1. Collisions are resolved by
				// the deterministic SELECT FOR UPDATE ordering plus the retry
				// loop for retryable PG error codes (40001, 40P01, 55P03).
				retries := 0
				for attempt := 0; attempt < 20; attempt++ {
					_ = attempt

					current, findErr := repo.Find(ctx, orgID, ledgerID, balanceID)
					if findErr != nil {
						totalFailures.Add(1)
						return
					}

					next := current.Available.Sub(decimal.NewFromInt(debitAmount))
					if next.Sign() < 0 {
						// Balance exhausted — expected when the first 1000 debits
						// succeed. Not a failure; the worker terminates early.
						return
					}

					updated := &mmodel.Balance{
						ID:        balanceID.String(),
						Available: next,
						OnHold:    current.OnHold,
						Version:   current.Version + 1,
					}

					updErr := repo.BalancesUpdate(ctx, orgID, ledgerID, []*mmodel.Balance{updated})
					if updErr == nil {
						totalSuccesses.Add(1)
						totalRetries.Add(int64(retries))

						return
					}

					if !isRetryableBatchBalanceUpdateError(updErr) {
						// BalancesUpdate swallows the "highest-version-wins"
						// no-op case as a success return; genuine non-retryable
						// errors surface here.
						totalFailures.Add(1)
						return
					}

					retries++
				}
			}()
		}

		iterStart := time.Now()
		close(start)
		wg.Wait()

		elapsed := time.Since(iterStart)
		if elapsed > 0 {
			b.ReportMetric(float64(concurrency)/elapsed.Seconds(), "attempts/sec")
		}
	}

	b.StopTimer()

	successes := totalSuccesses.Load()
	retries := totalRetries.Load()
	failures := totalFailures.Load()

	if successes == 0 {
		b.Fatalf("benchmark degenerate: zero successful debits (retries=%d, failures=%d)", retries, failures)
	}

	b.ReportMetric(float64(retries)/float64(successes), "retries/success")
	b.ReportMetric(float64(failures), "failures")
	b.ReportMetric(float64(successes), "successes_total")

	b.Logf("hot-balance contention: successes=%d retries=%d failures=%d", successes, retries, failures)
}

// createBenchRepository is the *testing.B sibling of createRepository. It
// constructs a BalancePostgreSQLRepository wired to the test container with
// the standard zap logger. The separate constructor keeps the existing
// *testing.T-based helper in balance.postgresql_integration_test.go intact.
func createBenchRepository(b *testing.B, container *pgtestutil.ContainerResult) *BalancePostgreSQLRepository {
	b.Helper()

	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(b, "transaction")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           container.Config.DBName,
		ReplicaDBName:           container.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	repo, err := NewBalancePostgreSQLRepository(conn)
	require.NoError(b, err, "failed to create balance repository")

	return repo
}

// seedHotBalance inserts a single balance row with the given alias and initial
// Available. Returns its UUID.
func seedHotBalance(b *testing.B, container *pgtestutil.ContainerResult, orgID, ledgerID, accountID uuid.UUID, alias string, available int64) uuid.UUID {
	b.Helper()

	id := libCommons.GenerateUUIDv7()
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.ExecContext(context.Background(), `
		INSERT INTO balance (id, organization_id, ledger_id, account_id, alias, key, asset_code, available, on_hold, version, account_type, allow_sending, allow_receiving, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, id, orgID, ledgerID, accountID, alias, "default", "USD",
		decimal.NewFromInt(available), decimal.Zero, 0, "deposit", true, true, now, now)
	require.NoError(b, err, "seed hot balance")

	return id
}

// resetHotBalance rewinds the balance to (available, version=0) for the next
// iteration.
func resetHotBalance(b *testing.B, container *pgtestutil.ContainerResult, id uuid.UUID, available int64) {
	b.Helper()

	_, err := container.DB.ExecContext(context.Background(),
		`UPDATE balance SET available = $1, version = 0, updated_at = now() WHERE id = $2`,
		decimal.NewFromInt(available), id)
	require.NoError(b, err, "reset hot balance")
}
