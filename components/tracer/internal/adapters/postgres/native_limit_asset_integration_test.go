//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"regexp"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	tracerpkg "github.com/LerianStudio/midaz/v4/components/tracer/pkg"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestIntegrationNativeLimitAssetsPreserveHistory(t *testing.T) {
	db := completionDatabase(t)
	account := testutil.MustDeterministicUUID(89001)
	limit := contextLimitRow(t, db, 89002, account)
	scope := "acct:" + account.String()
	_, err := db.ExecContext(t.Context(), `INSERT INTO usage_counters (limit_id,scope_key,period_key,current_usage,reserved_usage) VALUES ($1,$2,'2026-09-24',7.125,10.125)`, limit, scope)
	require.NoError(t, err)
	reservation := testutil.MustDeterministicUUID(89004)
	transaction := testutil.MustDeterministicUUID(89005)
	_, err = db.ExecContext(t.Context(), `INSERT INTO usage_reservations (id,limit_id,scope_key,period_key,amount,transaction_id,reservation_expires_at) VALUES ($1,$2,$3,'2026-09-24',10.125,$4,$5)`, reservation, limit, scope, transaction, testutil.FixedTime())
	require.NoError(t, err)
	// Existing ISO data can roll back without rewriting history.
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000033_native_limit_asset_codes.down.sql"))
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000033_native_limit_asset_codes.up.sql"))
	require.NoError(t, err)
	var used, reserved string
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT current_usage::text,reserved_usage::text FROM usage_counters WHERE limit_id=$1`, limit).Scan(&used, &reserved))
	require.Equal(t, "7.125", used)
	require.Equal(t, "10.125", reserved)
	_, err = db.ExecContext(t.Context(), `UPDATE limits SET asset='BTC' WHERE id=$1`, limit)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000033_native_limit_asset_codes.down.sql"))
	require.Error(t, err) // Three uppercase letters alone do not prove legacy ISO support.
	_, err = db.ExecContext(t.Context(), `UPDATE limits SET asset='wBTC' WHERE id=$1`, limit)
	require.NoError(t, err)
	command, _ := assetBindingCommand(t, db, true)
	facts := assetBindingFacts(account)
	facts[0].Asset.Code = "wBTC"
	ref, err := command.Execute(assetBindingContext(t.Context()), limit, facts)
	require.NoError(t, err)
	require.Equal(t, "wBTC", ref.Code)
	repo := NewLimitRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	for _, code := range []string{"wBTC", "WBTC"} {
		result, err := repo.List(t.Context(), &model.ListLimitsFilter{Asset: &code, Limit: 10})
		require.NoError(t, err)
		if code == "wBTC" {
			require.Len(t, result.Limits, 1)
			require.Equal(t, limit, result.Limits[0].ID)
		} else {
			require.Empty(t, result.Limits)
		}
	}
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000033_native_limit_asset_codes.down.sql"))
	require.Error(t, err)
	// Failed downgrade does not remove bounds, reference, audit or consumption.
	var code string
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT asset_code FROM limit_asset_references WHERE limit_id=$1`, limit).Scan(&code))
	require.Equal(t, "wBTC", code)
	require.Len(t, completionEvents(t, db, limit), 1)
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT current_usage::text,reserved_usage::text FROM usage_counters WHERE limit_id=$1`, limit).Scan(&used, &reserved))
	require.Equal(t, "7.125", used)
	require.Equal(t, "10.125", reserved)
	var amount string
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT amount::text FROM usage_reservations WHERE id=$1 AND limit_id=$2`, reservation, limit).Scan(&amount))
	require.Equal(t, "10.125", amount)
	another := contextLimitRow(t, db, 89003, account)
	_, err = db.ExecContext(t.Context(), `UPDATE limits SET asset=$2 WHERE id=$1`, another, strings.Repeat("x", 257))
	require.Error(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE limits SET asset='BTC' WHERE id=$1`, another)
	require.NoError(t, err)
}

func TestNativeCodeDowngradeListMatchesPreviousValidator(t *testing.T) {
	down := capacityMigration(t, "000033_native_limit_asset_codes.down.sql")
	codes := map[string]bool{}
	for _, match := range regexp.MustCompile(`'([A-Z]{3})'`).FindAllStringSubmatch(down, -1) {
		codes[match[1]] = true
	}
	for a := 'A'; a <= 'Z'; a++ {
		for b := 'A'; b <= 'Z'; b++ {
			for c := 'A'; c <= 'Z'; c++ {
				code := string([]rune{a, b, c})
				require.Equal(t, tracerpkg.IsValidCurrency(code), codes[code], code)
			}
		}
	}
	require.False(t, codes["BTC"])
}
