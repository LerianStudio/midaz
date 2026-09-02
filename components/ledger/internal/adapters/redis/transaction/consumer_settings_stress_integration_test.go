//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// This file reproduces G2: a PATCH to a balance's Settings races the atomic
// Lua debit script for the SAME cache key. UpdateBalanceCacheSettings used to
// read the cached JSON blob, mutate it in a Go map, and blindly SET the whole
// blob back — a GET-then-SET window with no atomicity guarantee. Any Lua
// write (balance_atomic_operation.lua) landing inside that window was
// silently reverted: the settings SET restored Available/Version/OverdraftUsed
// to whatever the settings update had read, permanently losing the debit(s)
// that ran in between. Since the cache is authoritative between PostgreSQL
// syncs, the loss survives the next sync (the guard in balance.postgresql.go
// only accepts a higher-versioned write).
//
// The fix moves the settings mutation into a single Lua EVAL
// (update_balance_settings.lua): Redis serializes EVAL execution, so the
// settings write and any concurrent balance_atomic_operation.lua debit on
// the same key can never interleave. There is no read-then-write window to
// race and therefore no retry path — every call either commits on its first
// and only attempt or reports a cache miss / corrupt blob.

// settingsStressDebits is the number of concurrent debit workers, each
// issuing one real Lua debit through ProcessBalanceAtomicOperation. Kept well
// under the seeded Available so every debit succeeds and no overdraft/version
// gate in the Lua script is triggered, keeping Version increments strictly
// tied to successful writes.
const settingsStressDebits = 400

// settingsStressSettingsIterations is the number of sequential
// UpdateBalanceCacheSettings calls fired from a single goroutine while the
// debit workers run concurrently. Sequential (not concurrent with each
// other) so "the last settings call" is an unambiguous invariant even though
// it races the debit workers in wall-clock time.
const settingsStressSettingsIterations = 400

// buildSettingsStressDebitOp builds a plain, non-overdraft DEBIT operation
// against a pre-seeded balance. Direction/AllowOverdraft are deliberately
// disabled and Available never approaches zero, so the Lua script never
// enters an overdraft or stale-version branch — every debit is a pure
// "read live cache, subtract, write back, Version++".
func buildSettingsStressDebitOp(orgID, ledgerID uuid.UUID, alias, internalKey string, amount decimal.Decimal) mmodel.BalanceOperation {
	return overdraftOp(orgID, ledgerID, alias, "deposit", "credit",
		decimal.Zero, decimal.Zero, 1, nil,
		constant.DEBIT, amount)
}

// TestIntegration_UpdateBalanceCacheSettings_ConcurrentWithAtomicDebits_G2 is
// the stress reproduction for G2. It seeds one balance, then races
// settingsStressDebits concurrent Lua debits against
// settingsStressSettingsIterations sequential settings PATCHes on the same
// key, and pins four invariants that a lost update would violate:
//
//  1. Available == seeded Available − Σ(debit amounts)
//  2. Version   == seeded Version + number of successful Lua writes
//  3. the settings fields in cache match the LAST settings call issued
//  4. every settings call succeeds on its first attempt — there is no retry
//     budget to exhaust under heavy concurrent write pressure
//
// Against the pre-fix GET-then-SET implementation this test failed on (1)
// and/or (2): a settings PATCH racing a debit's Lua write reverted
// Available/Version to a stale snapshot, permanently losing one or more
// debits.
func TestIntegration_UpdateBalanceCacheSettings_ConcurrentWithAtomicDebits_G2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@g2-settings-stress"
	balanceKey := alias + "#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	const seededAvailable = 1_000_000
	const seededVersion = int64(1)

	seeded := cachedBalance{
		ID:                    uuid.New().String(),
		Available:             decimal.NewFromInt(seededAvailable).String(),
		OnHold:                "0",
		Version:               seededVersion,
		AccountType:           "deposit",
		AccountID:             uuid.New().String(),
		AssetCode:             "USD",
		AllowSending:          1,
		AllowReceiving:        1,
		Key:                   balanceKey,
		Direction:             "credit",
		OverdraftUsed:         "0",
		AllowOverdraft:        0,
		OverdraftLimitEnabled: 0,
		OverdraftLimit:        "0",
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}
	payload, err := json.Marshal(seeded)
	require.NoError(t, err)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, payload, time.Hour).Err())

	debitAmount := decimal.NewFromInt(1)

	// lastSettings and settingsErr are written only by the settings-updater
	// goroutine and read only after wg.Wait(), so the sequential writes and
	// the final reads never race each other (happens-before via the WaitGroup).
	var lastSettings *mmodel.BalanceSettings

	var settingsErr error

	start := make(chan struct{})

	var wg sync.WaitGroup

	var debitErrs atomic.Int64

	var settingsCallsSucceeded atomic.Int64

	for i := 0; i < settingsStressDebits; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			op := buildSettingsStressDebitOp(orgID, ledgerID, alias, internalKey, debitAmount)

			_, opErr := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
				uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})
			if opErr != nil {
				debitErrs.Add(1)
			}
		}()
	}

	wg.Add(1)

	go func() {
		defer wg.Done()
		<-start

		for i := 0; i < settingsStressSettingsIterations; i++ {
			limit := decimal.NewFromInt(int64(1000 + i)).String()
			settings := &mmodel.BalanceSettings{
				BalanceScope:          mmodel.BalanceScopeTransactional,
				AllowOverdraft:        i%2 == 0,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &limit,
			}

			// Best-effort by contract, but under this design a non-nil error
			// here can only be a genuine infrastructure failure (transport
			// error or a corrupt cached blob) — there is no retry-exhaustion
			// outcome to tolerate, so any error is a real test failure.
			// FailNow must not run off the test goroutine, so the error is
			// recorded here and asserted after wg.Wait().
			if uErr := infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, balanceKey, settings); uErr != nil {
				settingsErr = uErr

				return
			}

			settingsCallsSucceeded.Add(1)
			lastSettings = settings
		}
	}()

	close(start)
	wg.Wait()

	require.NoError(t, settingsErr, "settings update failed under debit storm")
	require.Zero(t, debitErrs.Load(), "every debit is well within Available and must not be rejected")
	require.NotNil(t, lastSettings)

	// Every settings call committed on its first (and only) EVAL — the
	// in-Lua design has no retry loop and therefore no attempt budget that
	// could be exhausted under this write pressure.
	assert.EqualValues(t, settingsStressSettingsIterations, settingsCallsSucceeded.Load(),
		"every settings update must succeed in a single atomic call, even under concurrent debit pressure")

	final := readCachedBalance(t, infra, internalKey)

	expectedAvailable := decimal.NewFromInt(seededAvailable).
		Sub(debitAmount.Mul(decimal.NewFromInt(settingsStressDebits)))
	finalAvailable, err := decimal.NewFromString(final.Available)
	require.NoError(t, err)
	assert.Truef(t, finalAvailable.Equal(expectedAvailable),
		"Available must equal seeded minus every successful debit (lost update if not): want %s, got %s",
		expectedAvailable, finalAvailable)

	expectedVersion := seededVersion + settingsStressDebits
	assert.Equalf(t, expectedVersion, final.Version,
		"Version must equal seeded plus one increment per successful Lua write (lost update if not)")

	assert.Equal(t, boolToInt(lastSettings.AllowOverdraft), final.AllowOverdraft,
		"cached AllowOverdraft must reflect the last settings call issued")
	assert.Equal(t, boolToInt(lastSettings.OverdraftLimitEnabled), final.OverdraftLimitEnabled,
		"cached OverdraftLimitEnabled must reflect the last settings call issued")
	assert.Equal(t, *lastSettings.OverdraftLimit, final.OverdraftLimit,
		"cached OverdraftLimit must reflect the last settings call issued")
	assert.Equal(t, lastSettings.BalanceScope, final.BalanceScope,
		"cached BalanceScope must reflect the last settings call issued")
}
