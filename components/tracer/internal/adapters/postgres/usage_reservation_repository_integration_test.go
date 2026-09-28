// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// newReservationRepoIntegration wires the reservation repository plus the shared
// usage-counter repository over a real PostgreSQL connection.
func newReservationRepoIntegration(db *sql.DB) *UsageReservationRepository {
	adapter := &testutil.IntegrationDBAdapter{DB: db}
	counterRepo := NewUsageCounterRepositoryWithConnection(adapter)

	return NewUsageReservationRepositoryWithConnection(counterRepo)
}

// inRealTx runs fn inside a real *sql.Tx, committing on success and rolling back on
// error — mimicking the reservation service's tx ownership so the repo's *WithTx
// methods are exercised atomically.
func inRealTx(t *testing.T, db *sql.DB, fn func(tx *sql.Tx) error) error {
	t.Helper()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	if err := fn(tx); err != nil {
		require.NoError(t, tx.Rollback())
		return err
	}

	return tx.Commit()
}

// createTestLimitNamed seeds an ACTIVE limit with an explicit, unique name so
// multiple limits can coexist in one test without colliding on the global
// idx_limits_name_active partial unique index (the shared createTestLimit derives
// the name from the UUID prefix, which is identical across deterministic seeds).
func createTestLimitNamed(t *testing.T, db *sql.DB, seed int64, name string) uuid.UUID {
	t.Helper()

	limitID := testutil.MustDeterministicUUID(seed)

	_, err := db.Exec(`
		INSERT INTO limits (id, name, limit_type, max_amount, asset, scopes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, limitID, "Test Limit "+name, "DAILY", decimal.NewFromInt(10000), "USD", "[]", "ACTIVE")
	require.NoError(t, err, "Failed to create named test limit")

	return limitID
}

// readCounterDecimal reads the counter buckets as exact decimals, so fractional
// amounts survive the round-trip (the DECIMAL columns landed in migration 000021).
func readCounterDecimal(t *testing.T, db *sql.DB, limitID uuid.UUID, scopeKey, periodKey string) (current, reserved decimal.Decimal) {
	t.Helper()

	err := db.QueryRow(
		"SELECT current_usage, reserved_usage FROM usage_counters WHERE limit_id = $1 AND scope_key = $2 AND period_key = $3",
		limitID, scopeKey, periodKey,
	).Scan(&current, &reserved)
	require.NoError(t, err, "failed to read counter buckets")

	return current, reserved
}

func readReservationStatus(t *testing.T, db *sql.DB, reservationID uuid.UUID) string {
	t.Helper()

	var status string

	err := db.QueryRow("SELECT status FROM usage_reservations WHERE id = $1", reservationID).Scan(&status)
	require.NoError(t, err, "failed to read reservation status")

	return status
}
