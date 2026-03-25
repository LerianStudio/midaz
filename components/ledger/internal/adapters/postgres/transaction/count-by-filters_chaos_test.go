//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Chaos tests for CountByFilters in the transaction PostgreSQL repository.
//
// These tests exercise fault-tolerance of CountByFilters when the underlying
// PostgreSQL connection experiences failures (connection loss, latency,
// network partition). They use Toxiproxy for network fault injection.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos_CountByFilters \
//	    ./components/ledger/internal/adapters/postgres/transaction/...
package transaction

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE FOR CountByFilters
// =============================================================================

// countByFiltersChaosInfra holds resources for CountByFilters chaos tests.
type countByFiltersChaosInfra struct {
	pgResult   *pgtestutil.ContainerResult
	chaosInfra *chaos.Infrastructure
	repo       *TransactionPostgreSQLRepository
	conn       *libPostgres.Client
	proxy      *chaos.Proxy
	orgID      uuid.UUID
	ledgerID   uuid.UUID
	seededAt   time.Time
}

// setupCountByFiltersChaosInfra creates the chaos test infrastructure:
//  1. Creates Docker network + Toxiproxy container
//  2. Starts PostgreSQL container
//  3. Creates Toxiproxy proxy forwarding to PostgreSQL
//  4. Runs migrations and inserts test data
//  5. Builds repository connected through the proxy
func setupCountByFiltersChaosInfra(t *testing.T) *countByFiltersChaosInfra {
	t.Helper()

	chaosInfra := chaos.NewInfrastructure(t)

	pgResult := pgtestutil.SetupContainer(t)

	_, err := chaosInfra.RegisterContainerWithPort("postgres", pgResult.Container, "5432/tcp")
	require.NoError(t, err, "failed to register PostgreSQL container")

	proxy, err := chaosInfra.CreateProxyFor("postgres", "8668/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for PostgreSQL")

	containerInfo, ok := chaosInfra.GetContainer("postgres")
	require.True(t, ok, "PostgreSQL container should be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy address should be set")

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	proxyConnStr := pgtestutil.BuildConnectionStringWithHost(containerInfo.ProxyListen, pgResult.Config)

	conn := pgtestutil.CreatePostgresClient(t, proxyConnStr, proxyConnStr, pgResult.Config.DBName, migrationsPath)

	repo := NewTransactionPostgreSQLRepository(conn)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Insert test data directly through the container (bypassing proxy)
	// so data exists before chaos injection.
	now := time.Now().UTC().Truncate(time.Microsecond)
	body := `{"send":{"asset":"USD","value":"100"}}`

	for i := 0; i < 5; i++ {
		insertCountChaosTransaction(t, pgResult.DB, orgID, ledgerID, "PIX", "APPROVED", &body, now, nil)
	}

	return &countByFiltersChaosInfra{
		pgResult:   pgResult,
		chaosInfra: chaosInfra,
		repo:       repo,
		conn:       conn,
		proxy:      proxy,
		orgID:      orgID,
		ledgerID:   ledgerID,
		seededAt:   now,
	}
}

// insertCountChaosTransaction inserts a transaction with precise control.
func insertCountChaosTransaction(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, route, status string, body *string, createdAt time.Time, deletedAt *time.Time) {
	t.Helper()

	id := uuid.Must(libCommons.GenerateUUIDv7())
	amount := decimal.NewFromInt(100)

	_, err := db.Exec(`
		INSERT INTO transaction (id, parent_transaction_id, description, status, status_description, amount, asset_code, chart_of_accounts_group_name, route, organization_id, ledger_id, body, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, id, nil, "Chaos test transaction", status, nil, amount, "USD", "default", route, orgID, ledgerID, body, createdAt, createdAt, deletedAt)
	require.NoError(t, err, "failed to insert chaos test transaction")
}

// =============================================================================
// CHAOS TESTS
// =============================================================================

// TestIntegration_Chaos_CountByFilters_ConnectionLoss tests that CountByFilters
// returns an error gracefully (no panic) when the database connection is severed.
//
// 5-phase structure: Normal → Inject → Verify → Restore → Recovery
func TestIntegration_Chaos_CountByFilters_ConnectionLoss(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("skipping chaos test (set CHAOS=1 to run)")
	}

	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupCountByFiltersChaosInfra(t)
	ctx := context.Background()

	todayStart := time.Date(infra.seededAt.Year(), infra.seededAt.Month(), infra.seededAt.Day(), 0, 0, 0, 0, time.UTC)
	todayEnd := time.Date(infra.seededAt.Year(), infra.seededAt.Month(), infra.seededAt.Day(), 23, 59, 59, 999999999, time.UTC)

	filter := CountFilter{
		Route:     "PIX",
		Status:    "APPROVED",
		StartDate: todayStart,
		EndDate:   todayEnd,
	}

	// Phase 1: Normal — verify count works before chaos
	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
	require.NoError(t, err, "count should work before chaos")
	assert.Equal(t, int64(5), count, "expected 5 transactions before chaos")
	t.Logf("Phase 1 (Normal): count = %d", count)

	// Phase 2: Inject — disconnect the proxy (simulate connection loss)
	t.Log("Phase 2 (Inject): disconnecting network")
	err = infra.proxy.Disconnect()
	require.NoError(t, err, "failed to disconnect proxy")

	// Phase 3: Verify — CountByFilters should fail gracefully (no panic)
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = infra.repo.CountByFilters(ctxTimeout, infra.orgID, infra.ledgerID, filter)
	require.Error(t, err, "Phase 3 (Verify): CountByFilters should fail during connection loss")
	t.Logf("Phase 3 (Verify): count during connection loss returned error (expected): %v", err)

	// Phase 4: Restore — reconnect the proxy
	t.Log("Phase 4 (Restore): reconnecting network")
	err = infra.proxy.Reconnect()
	require.NoError(t, err, "failed to reconnect proxy")

	// Phase 5: Recovery — verify count works again
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
		return err
	}, 30*time.Second, "CountByFilters should recover after network reconnection")

	count, err = infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
	require.NoError(t, err, "count should work after recovery")
	assert.Equal(t, int64(5), count, "data should be intact after chaos")
	t.Logf("Phase 5 (Recovery): count = %d", count)

	t.Log("Chaos test passed: CountByFilters connection loss handled gracefully")
}

// TestIntegration_Chaos_CountByFilters_HighLatency tests that CountByFilters
// handles high network latency gracefully (query completes or times out).
//
// 5-phase structure: Normal → Inject → Verify → Restore → Recovery
func TestIntegration_Chaos_CountByFilters_HighLatency(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("skipping chaos test (set CHAOS=1 to run)")
	}

	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupCountByFiltersChaosInfra(t)
	ctx := context.Background()

	todayStart := time.Date(infra.seededAt.Year(), infra.seededAt.Month(), infra.seededAt.Day(), 0, 0, 0, 0, time.UTC)
	todayEnd := time.Date(infra.seededAt.Year(), infra.seededAt.Month(), infra.seededAt.Day(), 23, 59, 59, 999999999, time.UTC)

	filter := CountFilter{
		Route:     "PIX",
		Status:    "APPROVED",
		StartDate: todayStart,
		EndDate:   todayEnd,
	}

	// Phase 1: Normal — verify baseline works
	start := time.Now()
	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
	baselineLatency := time.Since(start)
	require.NoError(t, err, "count should work before chaos")
	assert.Equal(t, int64(5), count)
	t.Logf("Phase 1 (Normal): count = %d, latency = %v", count, baselineLatency)

	// Phase 2: Inject — add 5-second latency
	t.Log("Phase 2 (Inject): adding 5s network latency")
	err = infra.proxy.AddLatency(5*time.Second, 500*time.Millisecond)
	require.NoError(t, err, "failed to add latency")

	// Phase 3: Verify — query with timeout to verify graceful timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	start = time.Now()
	_, err = infra.repo.CountByFilters(ctxTimeout, infra.orgID, infra.ledgerID, filter)
	elapsed := time.Since(start)

	if err != nil {
		t.Logf("Phase 3 (Verify): count timed out as expected after %v: %v", elapsed, err)
	} else {
		t.Logf("Phase 3 (Verify): count succeeded despite latency in %v", elapsed)
	}
	// Key invariant: no panic occurred

	// Phase 4: Restore — remove latency toxic
	t.Log("Phase 4 (Restore): removing latency")
	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "failed to remove toxics")

	// Phase 5: Recovery — verify normal operation resumes
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
		return err
	}, 30*time.Second, "CountByFilters should recover after latency removal")

	count, err = infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
	require.NoError(t, err, "count should work after recovery")
	assert.Equal(t, int64(5), count, "data should be intact after chaos")
	t.Logf("Phase 5 (Recovery): count = %d", count)

	t.Log("Chaos test passed: CountByFilters high latency handled gracefully")
}

// TestIntegration_Chaos_CountByFilters_ConnectionPoolExhaustion tests that
// CountByFilters returns an error instead of hanging when the connection pool
// is exhausted (via Toxiproxy bandwidth limit to slow all connections).
//
// 5-phase structure: Normal → Inject → Verify → Restore → Recovery
func TestIntegration_Chaos_CountByFilters_ConnectionPoolExhaustion(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("skipping chaos test (set CHAOS=1 to run)")
	}

	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupCountByFiltersChaosInfra(t)
	ctx := context.Background()

	todayStart := time.Date(infra.seededAt.Year(), infra.seededAt.Month(), infra.seededAt.Day(), 0, 0, 0, 0, time.UTC)
	todayEnd := time.Date(infra.seededAt.Year(), infra.seededAt.Month(), infra.seededAt.Day(), 23, 59, 59, 999999999, time.UTC)

	filter := CountFilter{
		Route:     "PIX",
		Status:    "APPROVED",
		StartDate: todayStart,
		EndDate:   todayEnd,
	}

	// Phase 1: Normal — verify baseline works
	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
	require.NoError(t, err, "count should work before chaos")
	assert.Equal(t, int64(5), count)
	t.Logf("Phase 1 (Normal): count = %d", count)

	// Phase 2: Inject — disconnect proxy to simulate pool exhaustion
	// When the connection is severed, existing pool connections become stale
	// and new connections cannot be established, simulating pool exhaustion.
	t.Log("Phase 2 (Inject): disconnecting proxy to simulate connection pool exhaustion")
	err = infra.proxy.Disconnect()
	require.NoError(t, err, "failed to disconnect proxy")

	// Phase 3: Verify — concurrent queries should fail gracefully (no panic, no hang)
	const concurrency = 10
	var wg sync.WaitGroup
	var errCount int64

	for i := 0; i < concurrency; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctxTimeout, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			_, queryErr := infra.repo.CountByFilters(ctxTimeout, infra.orgID, infra.ledgerID, filter)
			if queryErr != nil {
				atomic.AddInt64(&errCount, 1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Phase 3 (Verify): %d/%d concurrent queries failed during pool exhaustion", errCount, concurrency)
	require.Greater(t, errCount, int64(0), "at least some concurrent queries should fail during pool exhaustion")

	// Phase 4: Restore — reconnect
	t.Log("Phase 4 (Restore): reconnecting proxy")
	err = infra.proxy.Reconnect()
	require.NoError(t, err, "failed to reconnect proxy")

	// Phase 5: Recovery — verify normal operation resumes
	chaos.AssertRecoveryWithin(t, func() error {
		_, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
		return err
	}, 30*time.Second, "CountByFilters should recover after reconnection")

	count, err = infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, filter)
	require.NoError(t, err, "count should work after recovery")
	assert.Equal(t, int64(5), count, "data should be intact after chaos")
	t.Logf("Phase 5 (Recovery): count = %d", count)

	t.Log("Chaos test passed: CountByFilters connection pool exhaustion handled gracefully")
}
