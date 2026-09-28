//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestIntegration_AccountClosingRepairNeverRecreatesAnEvictedBalance covers AS-06
// and AS-17 on the cache-writer side: once a closing has evicted the balances of an
// account, the in-place writers that a PATCH or a repair triggers must leave the
// key absent. Recreating it would readmit a balance of a closed account through a
// path that never asked whether the account is still open.
func TestIntegration_AccountClosingRepairNeverRecreatesAnEvictedBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	alias := "@account-closing-evicted-" + uuid.NewString()
	cacheKey := alias + "#default"
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, cacheKey)

	requireAbsent := func(t *testing.T, step string) {
		t.Helper()

		exists, err := infra.redisContainer.Client.Exists(ctx, internalKey).Result()
		require.NoError(t, err)
		assert.Zero(t, exists, "%s must not recreate an evicted balance", step)
	}

	requireAbsent(t, "the fixture")

	t.Run("a settings PATCH leaves the key absent", func(t *testing.T) {
		limit := "100"
		err := infra.repo.UpdateBalanceCacheSettings(ctx, organizationID, ledgerID, cacheKey, &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        &limit,
		})
		require.NoError(t, err)

		requireAbsent(t, "a settings update")
	})

	t.Run("a blocked propagation leaves the key absent", func(t *testing.T) {
		require.NoError(t, infra.repo.UpdateBalanceCacheBlocked(ctx, organizationID, ledgerID, []string{cacheKey}, true))

		requireAbsent(t, "a blocked propagation")
	})

	t.Run("an unblock leaves the key absent", func(t *testing.T) {
		require.NoError(t, infra.repo.UpdateBalanceCacheBlocked(ctx, organizationID, ledgerID, []string{cacheKey}, false))

		requireAbsent(t, "an unblock")
	})

	t.Run("an allow-flags PATCH leaves the key absent", func(t *testing.T) {
		allow := true
		require.NoError(t, infra.repo.UpdateBalanceCacheAllowFlags(ctx, organizationID, ledgerID, cacheKey, &allow, &allow))

		requireAbsent(t, "an allow-flags update")
	})

	t.Run("reading the balance reports a miss rather than a fabricated blob", func(t *testing.T) {
		balances, err := infra.repo.GetBalancesByKeys(ctx, []string{internalKey})
		require.NoError(t, err)
		assert.Nil(t, balances[internalKey], "an evicted balance must read as a miss")

		requireAbsent(t, "a cache read")
	})
}

// TestIntegration_AccountClosingWritersPreserveALiveBalance is the other half of
// the same contract: while the balance is present, the very same writers keep
// updating it in place, with the live transactional state untouched. The refusal
// above is about absent keys only, not about disabling the writers.
func TestIntegration_AccountClosingWritersPreserveALiveBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	alias := "@account-closing-live-" + uuid.NewString()
	cacheKey := alias + "#default"
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, cacheKey)

	blob := `{"ID":"` + uuid.NewString() + `","AccountID":"` + uuid.NewString() + `","Alias":"` + alias + `",` +
		`"Key":"default","Available":"120","OnHold":"5","Version":3,"AccountType":"deposit",` +
		`"AllowSending":1,"AllowReceiving":1,"AssetCode":"BRL","Blocked":false,"OverdraftUsed":"0"}`

	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, blob, 0).Err())

	require.NoError(t, infra.repo.UpdateBalanceCacheBlocked(ctx, organizationID, ledgerID, []string{cacheKey}, true))

	fields := readCachedBalanceFields(t, infra, internalKey)

	assert.JSONEq(t, `1`, string(fields["Blocked"]), "the propagated flag must land")
	assert.JSONEq(t, `"120"`, string(fields["Available"]), "the live available amount must survive")
	assert.JSONEq(t, `"5"`, string(fields["OnHold"]), "the live on-hold amount must survive")
	assert.JSONEq(t, `3`, string(fields["Version"]), "the live version must survive")
}
