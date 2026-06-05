// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
)

// upsertReserveSQL is the expected SQL for UpsertAndReserveAtomic using the
// reserve CTE. It mirrors upsertAtomicSQL but moves the amount into
// reserved_usage and guards on the three-term sum
// current_usage + reserved_usage + $9 <= $10.
const upsertReserveSQL = `
		WITH attempt AS (
			INSERT INTO usage_counters (id, limit_id, scope_key, period_key, current_usage, reserved_usage, last_updated_at, expires_at)
			VALUES ($1, $2, $3, $4, 0, $5, $6, $11)
			ON CONFLICT (limit_id, scope_key, period_key)
			DO UPDATE SET
				reserved_usage = usage_counters.reserved_usage + $7,
				last_updated_at = $8,
				expires_at = $11
			WHERE usage_counters.current_usage + usage_counters.reserved_usage + $9 <= $10
			RETURNING reserved_usage, true as succeeded
		)
		SELECT
			COALESCE(
				(SELECT reserved_usage FROM attempt),
				(SELECT reserved_usage FROM usage_counters
				 WHERE limit_id = $2 AND scope_key = $3 AND period_key = $4),
				$5
			) as reserved_usage,
			COALESCE(
				(SELECT succeeded FROM attempt),
				false
			) as succeeded
	`

// TestUsageCounterReserveCTEThreeTermGuard locks the critical correctness line:
// the reserve CTE's WHERE guard must account for BOTH committed and outstanding
// usage (current_usage + reserved_usage + amount <= maxAmount). A two-term guard
// would reintroduce the TOCTOU over-limit bug.
func TestUsageCounterReserveCTEThreeTermGuard(t *testing.T) {
	// Normalize whitespace the same way sqlmock does so the assertion is robust
	// to formatting (collapse runs of whitespace to a single space).
	normalized := regexp.MustCompile(`\s+`).ReplaceAllString(upsertAndReserveCTEQuery, " ")

	assert.Contains(t, normalized,
		"WHERE usage_counters.current_usage + usage_counters.reserved_usage + $9 <= $10",
		"reserve CTE must guard on the three-term sum current_usage + reserved_usage + amount")
	assert.Contains(t, normalized,
		"reserved_usage = usage_counters.reserved_usage + $7",
		"reserve CTE UPDATE must increment reserved_usage, not current_usage")
	// The INSERT branch must seed current_usage = 0 and reserved_usage = amount.
	assert.Contains(t, normalized,
		"VALUES ($1, $2, $3, $4, 0, $5, $6, $11)",
		"reserve CTE INSERT must seed current_usage = 0 and reserved_usage = amount")
	// Guard against a regression to the legacy two-term increment guard.
	assert.False(t, strings.Contains(normalized, "WHERE usage_counters.current_usage + $9 <= $10"),
		"reserve CTE must NOT use the legacy two-term increment guard")
}

func TestUsageCounterRepository_UpsertAndReserveAtomic(t *testing.T) {
	testutil.SetupTestTracing(t)

	limitID := testutil.MustDeterministicUUID(9010)
	scopeKey := "acct:9010"
	periodKey := "2026-06"

	wantReserved := decimal.RequireFromString("1000")

	tests := []struct {
		name         string
		limitID      uuid.UUID
		scopeKey     string
		periodKey    string
		amount       decimal.Decimal
		maxAmount    decimal.Decimal
		mockSetup    func(mock sqlmock.Sqlmock)
		wantErr      error
		wantReserved *decimal.Decimal
	}{
		{
			name:      "Success - reserve within combined capacity",
			limitID:   limitID,
			scopeKey:  scopeKey,
			periodKey: periodKey,
			amount:    decimal.RequireFromString("400"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// current_usage (300) + reserved_usage (300) + amount (400) = 1000 <= 1000.
				rows := sqlmock.NewRows([]string{"reserved_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("700"), true)

				mock.ExpectQuery(regexp.QuoteMeta(upsertReserveSQL)).
					WithArgs(
						sqlmock.AnyArg(), // $1 id
						limitID.String(), // $2 limit_id
						scopeKey,         // $3 scope_key
						periodKey,        // $4 period_key
						sqlmock.AnyArg(), // $5 amount (INSERT reserved_usage)
						sqlmock.AnyArg(), // $6 last_updated_at
						sqlmock.AnyArg(), // $7 amount (UPDATE reserved_usage)
						sqlmock.AnyArg(), // $8 last_updated_at
						sqlmock.AnyArg(), // $9 amount (WHERE guard)
						sqlmock.AnyArg(), // $10 maxAmount (WHERE guard)
						sqlmock.AnyArg(), // $11 expiresAt
					).
					WillReturnRows(rows)
			},
			wantErr:      nil,
			wantReserved: testutil.Ptr(decimal.RequireFromString("700")),
		},
		{
			name:      "Error - combined usage would exceed limit (guard fails)",
			limitID:   testutil.MustDeterministicUUID(9011),
			scopeKey:  "acct:9011",
			periodKey: periodKey,
			amount:    decimal.RequireFromString("600"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// current_usage (300) + reserved_usage (300) + amount (600) = 1200 > 1000.
				// CTE attempt returns no rows; COALESCE falls back to existing reserved_usage.
				rows := sqlmock.NewRows([]string{"reserved_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("300"), false)

				mock.ExpectQuery(regexp.QuoteMeta(upsertReserveSQL)).
					WithArgs(
						sqlmock.AnyArg(),
						testutil.MustDeterministicUUID(9011).String(),
						"acct:9011",
						periodKey,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)
			},
			wantErr:      constant.ErrUsageCounterExceedsLimit,
			wantReserved: testutil.Ptr(decimal.RequireFromString("300")),
		},
		{
			name:      "Boundary - amount equals maxAmount on fresh counter is allowed",
			limitID:   testutil.MustDeterministicUUID(9012),
			scopeKey:  "acct:9012",
			periodKey: periodKey,
			amount:    decimal.RequireFromString("1000"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"reserved_usage", "succeeded"}).
					AddRow(decimal.RequireFromString("1000"), true)

				mock.ExpectQuery(regexp.QuoteMeta(upsertReserveSQL)).
					WithArgs(
						sqlmock.AnyArg(),
						testutil.MustDeterministicUUID(9012).String(),
						"acct:9012",
						periodKey,
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnRows(rows)
			},
			wantErr:      nil,
			wantReserved: &wantReserved,
		},
		{
			name:      "Pre-check - amount > maxAmount rejected before SQL",
			limitID:   testutil.MustDeterministicUUID(9013),
			scopeKey:  "acct:9013",
			periodKey: periodKey,
			amount:    decimal.RequireFromString("1500"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No SQL expected: the Go pre-check catches this. The INSERT path has
				// no WHERE guard, so a fresh counter would otherwise over-reserve.
			},
			wantErr: constant.ErrUsageCounterExceedsLimit,
		},
		{
			name:      "Zero amount is a no-op",
			limitID:   testutil.MustDeterministicUUID(9014),
			scopeKey:  "acct:9014",
			periodKey: periodKey,
			amount:    decimal.RequireFromString("0"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// Zero amount: no-op, no SQL.
			},
			wantErr: nil,
		},
		{
			name:      "Negative amount rejected before SQL",
			limitID:   testutil.MustDeterministicUUID(9015),
			scopeKey:  "acct:9015",
			periodKey: periodKey,
			amount:    decimal.RequireFromString("-10"),
			maxAmount: decimal.RequireFromString("1000"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// Negative amount: rejected before SQL.
			},
			wantErr: constant.ErrUsageCounterIncrementNonNegative,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db, sqlMock, cleanup := setupUsageCounterRepositoryCallerDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			reserved, err := repo.UpsertAndReserveAtomic(ctx, db, tt.limitID, tt.scopeKey, tt.periodKey, tt.amount, tt.maxAmount, nil)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				if tt.wantReserved != nil {
					assert.True(t, tt.wantReserved.Equal(reserved), "expected reserved %s, got %s", tt.wantReserved, reserved)
				}

				return
			}

			require.NoError(t, err)

			if tt.wantReserved != nil {
				assert.True(t, tt.wantReserved.Equal(reserved), "expected reserved %s, got %s", tt.wantReserved, reserved)
			}
		})
	}
}

func TestUpsertAndReserveAtomic_NilDB(t *testing.T) {
	repo := NewUsageCounterRepositoryWithConnection(nil)

	_, err := repo.UpsertAndReserveAtomic(
		context.Background(),
		nil,
		testutil.MustDeterministicUUID(9020),
		"acct:9020",
		"2026-06",
		decimal.RequireFromString("100"),
		decimal.RequireFromString("1000"),
		nil,
	)

	require.ErrorIs(t, err, pgdb.ErrNilConnection)
}
