// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// pgCheckViolation is the SQLSTATE PostgreSQL raises when a row fails a CHECK
// constraint.
const pgCheckViolation = "23514"

// TestLimitRepository_ResetTime_Integration verifies that a limit's reset time
// is persisted on create, read back on every read path, and never rewritten by
// an update, and that the limits table itself refuses malformed or misplaced
// reset_time values, including one strictly inside the active time window.
func TestLimitRepository_ResetTime_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := NewLimitRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	ctx := context.Background()
	createdAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	t.Run("reset time round-trips through GetByID and List", func(t *testing.T) {
		lmt := createResetTimeLimit(ctx, t, db, repo, "reset-time-round-trip", 27001, "09:00", createdAt)

		got, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)
		require.NotNil(t, got.ResetTime, "GetByID must load reset_time")
		assert.Equal(t, "09:00", got.ResetTime.String())

		listed := listLimitByName(ctx, t, repo, lmt.Name)
		require.NotNil(t, listed.ResetTime, "List must load reset_time")
		assert.Equal(t, "09:00", listed.ResetTime.String())
	})

	t.Run("limit without reset time reads back nil", func(t *testing.T) {
		lmt := createResetTimeLimit(ctx, t, db, repo, "reset-time-absent", 27002, "", createdAt)

		got, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)
		assert.Nil(t, got.ResetTime, "GetByID must keep an absent reset time nil")

		listed := listLimitByName(ctx, t, repo, lmt.Name)
		assert.Nil(t, listed.ResetTime, "List must keep an absent reset time nil")
	})

	t.Run("update keeps the stored reset time", func(t *testing.T) {
		lmt := createResetTimeLimit(ctx, t, db, repo, "reset-time-update", 27003, "09:00", createdAt)

		loaded, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)

		newName := lmt.Name + "-renamed"
		require.NoError(t, loaded.Update(&newName, nil, nil, nil, nil, nil, nil, nil, createdAt.Add(time.Hour)))
		require.NoError(t, repo.UpdateWithTx(ctx, db, loaded))

		got, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)
		assert.Equal(t, newName, got.Name)
		require.NotNil(t, got.ResetTime, "an update must not clear reset_time")
		assert.Equal(t, "09:00", got.ResetTime.String())

		otherResetTime, err := model.NewTimeOfDay("10:00")
		require.NoError(t, err)

		got.ResetTime = &otherResetTime
		require.NoError(t, repo.UpdateWithTx(ctx, db, got))

		reread, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)
		require.NotNil(t, reread.ResetTime)
		assert.Equal(t, "09:00", reread.ResetTime.String(), "reset_time is immutable: an update must never write it")
	})

	t.Run("malformed reset_time is refused by the table", func(t *testing.T) {
		_, err := db.ExecContext(ctx,
			`INSERT INTO limits (name, limit_type, max_amount, asset, reset_time)
			 VALUES ('reset-time-malformed', 'DAILY', 1000, 'BRL', '9:00')`)
		requireCheckViolation(t, err, "chk_limits_reset_time_format")
	})

	t.Run("reset_time on a non-periodic limit is refused by the table", func(t *testing.T) {
		_, err := db.ExecContext(ctx,
			`INSERT INTO limits (name, limit_type, max_amount, asset, reset_time)
			 VALUES ('reset-time-per-transaction', 'PER_TRANSACTION', 1000, 'BRL', '09:00')`)
		requireCheckViolation(t, err, "chk_limits_reset_time_period_type")

		_, err = db.ExecContext(ctx,
			`INSERT INTO limits (name, limit_type, max_amount, asset, reset_time,
				custom_start_date, custom_end_date)
			 VALUES ('reset-time-custom', 'CUSTOM', 1000, 'BRL', '09:00',
				'2026-10-01T00:00:00Z'::timestamptz, '2026-10-31T00:00:00Z'::timestamptz)`)
		requireCheckViolation(t, err, "chk_limits_reset_time_period_type")
	})

	t.Run("window moved over the stored reset time is refused by the table", func(t *testing.T) {
		lmt := createResetTimeLimit(ctx, t, db, repo, "reset-time-window-update", 27004, "09:00", createdAt)

		_, err := db.ExecContext(ctx,
			`UPDATE limits SET active_time_start = '08:00', active_time_end = '10:00' WHERE id = $1`, lmt.ID)
		requireCheckViolation(t, err, "chk_limits_reset_time_outside_window")

		_, err = db.ExecContext(ctx,
			`UPDATE limits SET active_time_start = '23:00', active_time_end = '09:30' WHERE id = $1`, lmt.ID)
		requireCheckViolation(t, err, "chk_limits_reset_time_outside_window")

		got, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)
		assert.Nil(t, got.ActiveTimeStart, "a refused update must leave the stored window unchanged")
		assert.Nil(t, got.ActiveTimeEnd)
	})

	t.Run("repository update writing a window over the reset time is refused", func(t *testing.T) {
		lmt := createResetTimeLimit(ctx, t, db, repo, "reset-time-window-repository", 27005, "09:00", createdAt)

		loaded, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)

		// Assigning the window directly skips the domain check, as a binary
		// that predates reset times does.
		start, err := model.NewTimeOfDay("08:00")
		require.NoError(t, err)
		end, err := model.NewTimeOfDay("10:00")
		require.NoError(t, err)

		loaded.ActiveTimeStart = &start
		loaded.ActiveTimeEnd = &end

		err = repo.UpdateWithTx(ctx, db, loaded)
		requireCheckViolation(t, err, "chk_limits_reset_time_outside_window")
	})

	t.Run("reset time inside an overnight window is refused on insert", func(t *testing.T) {
		_, err := db.ExecContext(ctx,
			`INSERT INTO limits (name, limit_type, max_amount, asset, reset_time,
				active_time_start, active_time_end)
			 VALUES ('reset-time-inside-overnight', 'DAILY', 1000, 'BRL', '00:00', '23:00', '09:00')`)
		requireCheckViolation(t, err, "chk_limits_reset_time_outside_window")
	})

	t.Run("reset time on a window edge or outside it is accepted by the table", func(t *testing.T) {
		lmt := createResetTimeLimit(ctx, t, db, repo, "reset-time-window-allowed", 27006, "09:00", createdAt)

		for _, window := range []struct {
			name, start, end string
		}{
			{"equal to the start", "09:00", "12:00"},
			{"equal to the end", "06:00", "09:00"},
			{"equal to the end of an overnight window", "23:00", "09:00"},
			{"outside a same-day window", "10:00", "12:00"},
			{"outside an overnight window", "20:00", "06:00"},
		} {
			t.Run(window.name, func(t *testing.T) {
				result, err := db.ExecContext(ctx,
					`UPDATE limits SET active_time_start = $2, active_time_end = $3 WHERE id = $1`,
					lmt.ID, window.start, window.end)
				require.NoError(t, err)

				rows, err := result.RowsAffected()
				require.NoError(t, err)
				assert.Equal(t, int64(1), rows)
			})
		}

		result, err := db.ExecContext(ctx,
			`UPDATE limits SET active_time_start = NULL, active_time_end = NULL WHERE id = $1`, lmt.ID)
		require.NoError(t, err, "removing the window must be accepted")

		rows, err := result.RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), rows)
	})
}

// createResetTimeLimit persists a DAILY limit with the given reset time ("" for
// none) and registers its removal.
func createResetTimeLimit(
	ctx context.Context,
	t *testing.T,
	db *sql.DB,
	repo *LimitRepository,
	name string,
	accountSeed int64,
	resetTime string,
	createdAt time.Time,
) *model.Limit {
	t.Helper()

	var opts []model.LimitOption

	if resetTime != "" {
		parsed, err := model.NewTimeOfDay(resetTime)
		require.NoError(t, err)

		opts = append(opts, model.WithResetTime(&parsed))
	}

	accountID := testutil.MustDeterministicUUID(accountSeed)

	lmt, err := model.NewLimit(name, model.LimitTypeDaily, decimal.RequireFromString("1000"), "BRL",
		[]model.Scope{{AccountID: &accountID}}, nil, createdAt, opts...)
	require.NoError(t, err, "build limit %s", name)

	require.NoError(t, repo.CreateWithTx(ctx, db, lmt), "persist limit %s", name)

	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM limits WHERE id = $1`, lmt.ID); err != nil {
			t.Logf("cleanup: delete limit %s: %v", lmt.ID, err)
		}
	})

	return lmt
}

// listLimitByName returns the single limit List finds under the given name.
func listLimitByName(ctx context.Context, t *testing.T, repo *LimitRepository, name string) *model.Limit {
	t.Helper()

	result, err := repo.List(ctx, &model.ListLimitsFilter{Name: &name, Limit: 10})
	require.NoError(t, err)

	for i := range result.Limits {
		if result.Limits[i].Name == name {
			return &result.Limits[i]
		}
	}

	require.FailNow(t, "limit not listed", "name %s", name)

	return nil
}

// requireCheckViolation asserts err is a CHECK violation raised by constraint.
func requireCheckViolation(t *testing.T, err error, constraint string) {
	t.Helper()

	require.Error(t, err, "the insert must be refused by %s", constraint)

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "the refusal must be a PostgreSQL error")
	assert.Equal(t, pgCheckViolation, pgErr.Code)
	assert.Equal(t, constraint, pgErr.ConstraintName)
}
