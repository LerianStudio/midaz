//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

// =============================================================================
// INTEGRATION TESTS — CountByFilters Repository Method
//
// These tests verify CountByFilters against a real PostgreSQL container with
// known test data. They use testcontainers for isolation and the project's
// standard fixture helpers.
//
// Test data layout:
//   - 5 transactions: route="PIX", status="APPROVED", created_at=today
//   - 3 transactions: route="PIX", status="PENDING",  created_at=today
//   - 2 transactions: route="TED", status="APPROVED", created_at=today
//   - 1 transaction:  route="PIX", status="APPROVED", deleted_at=now (soft-deleted)
//   - 2 transactions: route="PIX", status="APPROVED", created_at=yesterday
//
// Run with:
//
//	go test -tags integration -run TestIntegration_CountByFilters -v -count=1 \
//	    ./components/ledger/internal/adapters/postgres/transaction/
//
// =============================================================================

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countByFiltersInfra holds infrastructure for CountByFilters integration tests.
type countByFiltersInfra struct {
	pgContainer *pgtestutil.ContainerResult
	repo        *TransactionPostgreSQLRepository
	orgID       uuid.UUID
	ledgerID    uuid.UUID
	seededAt    time.Time
}

// setupCountByFiltersInfra creates infrastructure and inserts the standard
// test data set for CountByFilters tests.
func setupCountByFiltersInfra(t *testing.T) *countByFiltersInfra {
	t.Helper()

	pgContainer := pgtestutil.SetupContainer(t)

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)

	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, pgContainer.Config.DBName, migrationsPath)

	repo := NewTransactionPostgreSQLRepository(conn)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	now := time.Now().UTC().Truncate(time.Microsecond)
	yesterday := now.Add(-24 * time.Hour)

	body := `{"send":{"asset":"USD","value":"100"}}`

	// 5 transactions: route="PIX", status="APPROVED", created_at=today
	for i := 0; i < 5; i++ {
		insertTransaction(t, pgContainer.DB, orgID, ledgerID, "PIX", "APPROVED", &body, now, nil)
	}

	// 3 transactions: route="PIX", status="PENDING", created_at=today
	for i := 0; i < 3; i++ {
		insertTransaction(t, pgContainer.DB, orgID, ledgerID, "PIX", "PENDING", &body, now, nil)
	}

	// 2 transactions: route="TED", status="APPROVED", created_at=today
	for i := 0; i < 2; i++ {
		insertTransaction(t, pgContainer.DB, orgID, ledgerID, "TED", "APPROVED", &body, now, nil)
	}

	// 1 soft-deleted transaction: route="PIX", status="APPROVED", deleted_at=now
	deletedAt := now
	insertTransaction(t, pgContainer.DB, orgID, ledgerID, "PIX", "APPROVED", &body, now, &deletedAt)

	// 2 transactions: route="PIX", status="APPROVED", created_at=yesterday
	for i := 0; i < 2; i++ {
		insertTransaction(t, pgContainer.DB, orgID, ledgerID, "PIX", "APPROVED", &body, yesterday, nil)
	}

	return &countByFiltersInfra{
		pgContainer: pgContainer,
		repo:        repo,
		orgID:       orgID,
		ledgerID:    ledgerID,
		seededAt:    now,
	}
}

// insertTransaction inserts a transaction row with precise control over route,
// status, created_at, and deleted_at timestamps.
func insertTransaction(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, route, status string, body *string, createdAt time.Time, deletedAt *time.Time) uuid.UUID {
	t.Helper()

	id := uuid.Must(libCommons.GenerateUUIDv7())
	amount := decimal.NewFromInt(100)

	_, err := db.Exec(`
		INSERT INTO transaction (id, parent_transaction_id, description, status, status_description, amount, asset_code, chart_of_accounts_group_name, route, organization_id, ledger_id, body, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, id, nil, "Test transaction", status, nil, amount, "USD", "default", route, orgID, ledgerID, body, createdAt, createdAt, deletedAt)
	require.NoError(t, err, "failed to insert test transaction")

	return id
}

// todayRange returns the start and end of the given reference day in UTC.
func todayRange(ref time.Time) (time.Time, time.Time) {
	start := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(ref.Year(), ref.Month(), ref.Day(), 23, 59, 59, 999999999, time.UTC)
	return start, end
}

// fullRange returns a 48-hour range covering the day before ref through ref day.
func fullRange(ref time.Time) (time.Time, time.Time) {
	start := time.Date(ref.Year(), ref.Month(), ref.Day()-1, 0, 0, 0, 0, time.UTC)
	end := time.Date(ref.Year(), ref.Month(), ref.Day(), 23, 59, 59, 999999999, time.UTC)
	return start, end
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

// TestIntegration_CountByFilters_NoFilters counts all non-deleted transactions
// created today (default date range).
func TestIntegration_CountByFilters_NoFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	todayStart, todayEnd := todayRange(infra.seededAt)

	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		StartDate: todayStart,
		EndDate:   todayEnd,
	})
	require.NoError(t, err)

	// Today's non-deleted: 5 PIX/APPROVED + 3 PIX/PENDING + 2 TED/APPROVED = 10
	assert.Equal(t, int64(10), count, "count all today (no filters) should be 10")
}

// TestIntegration_CountByFilters_ByRoute counts transactions filtered by route="PIX".
func TestIntegration_CountByFilters_ByRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	todayStart, todayEnd := todayRange(infra.seededAt)

	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		Route:     "PIX",
		StartDate: todayStart,
		EndDate:   todayEnd,
	})
	require.NoError(t, err)

	// Today's PIX non-deleted: 5 APPROVED + 3 PENDING = 8
	assert.Equal(t, int64(8), count, "count by route=PIX today should be 8")
}

// TestIntegration_CountByFilters_ByStatus counts transactions filtered by status="APPROVED".
func TestIntegration_CountByFilters_ByStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	todayStart, todayEnd := todayRange(infra.seededAt)

	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		Status:    "APPROVED",
		StartDate: todayStart,
		EndDate:   todayEnd,
	})
	require.NoError(t, err)

	// Today's APPROVED non-deleted: 5 PIX + 2 TED = 7
	assert.Equal(t, int64(7), count, "count by status=APPROVED today should be 7")
}

// TestIntegration_CountByFilters_ByRouteAndStatus counts with both route and status.
func TestIntegration_CountByFilters_ByRouteAndStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	todayStart, todayEnd := todayRange(infra.seededAt)

	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		Route:     "PIX",
		Status:    "APPROVED",
		StartDate: todayStart,
		EndDate:   todayEnd,
	})
	require.NoError(t, err)

	// Today's PIX/APPROVED non-deleted: 5
	assert.Equal(t, int64(5), count, "count by route=PIX status=APPROVED today should be 5")
}

// TestIntegration_CountByFilters_ExplicitDateRange counts with a date range
// spanning yesterday through today (includes the 2 yesterday transactions).
func TestIntegration_CountByFilters_ExplicitDateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	start, end := fullRange(infra.seededAt)

	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		StartDate: start,
		EndDate:   end,
	})
	require.NoError(t, err)

	// All non-deleted: 10 today + 2 yesterday = 12
	assert.Equal(t, int64(12), count, "count with full date range should be 12")
}

// TestIntegration_CountByFilters_NonExistentRoute returns 0 for a route with no transactions.
func TestIntegration_CountByFilters_NonExistentRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	start, end := fullRange(infra.seededAt)

	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		Route:     "NONEXISTENT",
		StartDate: start,
		EndDate:   end,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "count for non-existent route should be 0")
}

// TestIntegration_CountByFilters_SoftDeletedExcluded verifies that soft-deleted
// transactions are never counted.
func TestIntegration_CountByFilters_SoftDeletedExcluded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	start, end := fullRange(infra.seededAt)

	// Count all PIX/APPROVED across full range
	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		Route:     "PIX",
		Status:    "APPROVED",
		StartDate: start,
		EndDate:   end,
	})
	require.NoError(t, err)

	// 5 today + 2 yesterday = 7 (soft-deleted excluded)
	assert.Equal(t, int64(7), count, "soft-deleted PIX/APPROVED should be excluded")
}

// TestIntegration_CountByFilters_EmptyOrg returns 0 for an org with no transactions.
func TestIntegration_CountByFilters_EmptyOrg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	start, end := fullRange(infra.seededAt)

	// Use a different org ID that has no transactions
	emptyOrgID := uuid.Must(libCommons.GenerateUUIDv7())

	count, err := infra.repo.CountByFilters(ctx, emptyOrgID, infra.ledgerID, CountFilter{
		StartDate: start,
		EndDate:   end,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "count for org with no transactions should be 0")
}

// TestIntegration_CountByFilters_FilterNarrowing verifies that adding a filter
// never increases the count (monotonic decrease property).
func TestIntegration_CountByFilters_FilterNarrowing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	todayStart, todayEnd := todayRange(infra.seededAt)

	// Count with no filters
	countAll, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		StartDate: todayStart,
		EndDate:   todayEnd,
	})
	require.NoError(t, err)

	// Count with route filter
	countRoute, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		Route:     "PIX",
		StartDate: todayStart,
		EndDate:   todayEnd,
	})
	require.NoError(t, err)

	// Count with route + status filter
	countRouteStatus, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		Route:     "PIX",
		Status:    "APPROVED",
		StartDate: todayStart,
		EndDate:   todayEnd,
	})
	require.NoError(t, err)

	// Narrowing: all >= route >= route+status
	assert.GreaterOrEqual(t, countAll, countRoute,
		"adding route filter should not increase count (%d >= %d)", countAll, countRoute)
	assert.GreaterOrEqual(t, countRoute, countRouteStatus,
		"adding status filter should not increase count (%d >= %d)", countRoute, countRouteStatus)
}

// TestIntegration_CountByFilters_FutureDateRange returns 0 for a future date range.
func TestIntegration_CountByFilters_FutureDateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)
	ctx := context.Background()

	futureStart := time.Now().UTC().Add(48 * time.Hour)
	futureEnd := futureStart.Add(24 * time.Hour)

	count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		StartDate: futureStart,
		EndDate:   futureEnd,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "count for future date range should be 0")
}

// TestIntegration_CountByFilters_CancelledContext returns error on cancelled context.
func TestIntegration_CountByFilters_CancelledContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCountByFiltersInfra(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	todayStart, todayEnd := todayRange(infra.seededAt)

	_, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{
		StartDate: todayStart,
		EndDate:   todayEnd,
	})
	assert.Error(t, err, "cancelled context should produce error")
}
