// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
)

// TestIntegration_GetLimitUsage_ReportsOnlyTheLivePeriod is the customer-facing
// lock on GET /v1/limits/{id}/usage.
//
// Usage counters are retained past the end of their period, so a recurring limit
// accumulates one dead counter per elapsed period. Summing all of them turned the
// snapshot into a lifetime total: on a daily limit the reported utilisation ran
// past 100% of the cap after a couple of days of ordinary traffic and the
// nearLimit flag latched on for good, with the cap never once reached — so an
// operator watching it warned customers who had most of their headroom left.
//
// Scenario: a daily cap of 1000. Today two scopes have spent 300 and 200. The two
// previous days spent 900 each and their counters are still in the table.
//
//	summing every period: currentUsage 2300, utilisation 230%, nearLimit true
//	the live period only:  currentUsage  500, utilisation  50%, nearLimit false
func TestIntegration_GetLimitUsage_ReportsOnlyTheLivePeriod(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	ctx := context.Background()

	// A fixed clock pins which period is "live", so the assertion never depends
	// on the wall clock or on which side of midnight the suite runs.
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	livePeriod := "2026-03-15"

	limitID := seedDailyLimit(t, db, 1000)
	t.Cleanup(func() {
		if _, err := db.Exec("DELETE FROM limits WHERE id = $1", limitID); err != nil {
			t.Logf("cleanup limit %s: %v", limitID, err)
		}
	})

	// Two scopes inside the live period, and two whole periods that have elapsed.
	seedCounter(t, db, limitID, "acct:live-a", livePeriod, "300")
	seedCounter(t, db, limitID, "acct:live-b", livePeriod, "200")
	seedCounter(t, db, limitID, "acct:live-a", "2026-03-14", "900")
	seedCounter(t, db, limitID, "acct:live-a", "2026-03-13", "900")

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	limitRepo := postgres.NewLimitRepositoryWithConnection(adapter)
	counterRepo := postgres.NewUsageCounterRepositoryWithConnection(adapter)

	getLimitQuery := query.NewGetLimitQuery(limitRepo)

	// Only getQuery, usageCounterRepo and the clock take part in GetLimitUsage;
	// the write commands are not on this path.
	svc := services.NewLimitService(
		nil, nil, nil, nil, nil, nil,
		getLimitQuery, nil, counterRepo, clock.NewFixedClock(now),
	)

	snapshot, err := svc.GetLimitUsage(ctx, limitID)
	require.NoError(t, err, "GetLimitUsage must succeed")

	require.Equal(t, "500", snapshot.CurrentUsage.String(),
		"currentUsage MUST cover only the live period; counting the two elapsed days gives 2300 against a cap of 1000")
	require.InDelta(t, 50.0, snapshot.UtilizationPercent, 0.001,
		"utilizationPercent MUST be the live period's share of the cap, not a lifetime ratio")
	require.False(t, snapshot.NearLimit,
		"nearLimit MUST stay off at half the cap; summing elapsed periods latches it on permanently")
	require.Equal(t, "1000", snapshot.LimitAmount.String(), "the cap itself is unchanged")
}

func seedDailyLimit(t *testing.T, db *sql.DB, maxAmount int64) uuid.UUID {
	t.Helper()

	limitID := uuid.New()

	_, err := db.Exec(`
		INSERT INTO limits (id, name, limit_type, max_amount, asset, scopes, status)
		VALUES ($1, $2, 'DAILY', $3, 'USD', '[]', 'ACTIVE')`,
		limitID, "usage-period-"+limitID.String()[:8], decimal.NewFromInt(maxAmount))
	require.NoError(t, err, "seed the limit the counters hang off")

	return limitID
}

func seedCounter(t *testing.T, db *sql.DB, limitID uuid.UUID, scopeKey, periodKey, amount string) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO usage_counters (id, limit_id, scope_key, period_key, current_usage)
		VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), limitID, scopeKey, periodKey, decimal.RequireFromString(amount))
	require.NoError(t, err, "seed usage counter %s/%s", scopeKey, periodKey)
}
