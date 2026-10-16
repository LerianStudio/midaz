//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestIntegration_AccountClosingRepairNeverRecreatesAnEvictedBalance covers AS-06
// and AS-17 on the cache-writer side: once a closing has evicted the balances of an
// account, the configuration copies that a PATCH triggers must leave the key
// absent. Recreating it would readmit a balance of a closed account through a
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

	t.Run("a balance configuration copy leaves the key absent", func(t *testing.T) {
		outcome, err := infra.repo.CopyBalanceConfigToBlob(ctx, organizationID, ledgerID, cacheKey, balanceConfigAt(uuid.New(), 1, true))
		require.NoError(t, err)
		assert.Equal(t, BlobCopyBlobAbsent, outcome)

		requireAbsent(t, "a balance configuration copy")
	})

	for _, blocked := range []bool{true, false} {
		t.Run(fmt.Sprintf("an account configuration copy with blocked=%t leaves the key absent", blocked), func(t *testing.T) {
			outcomes, err := infra.repo.CopyAccountConfigToBlobs(ctx, organizationID, ledgerID, []string{cacheKey},
				accounting.AccountConfig{AccountID: uuid.New(), Version: 1, Blocked: blocked})
			require.NoError(t, err)
			assert.Equal(t, map[string]BlobCopyOutcome{cacheKey: BlobCopyBlobAbsent}, outcomes)

			requireAbsent(t, "an account configuration copy")
		})
	}

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

	accountID := uuid.New()
	blob := `{"ID":"` + uuid.NewString() + `","AccountID":"` + accountID.String() + `","Alias":"` + alias + `",` +
		`"Key":"default","Available":"120","OnHold":"5","Version":3,"AccountType":"deposit",` +
		`"AllowSending":1,"AllowReceiving":1,"AssetCode":"BRL","Blocked":false,"OverdraftUsed":"0"}`

	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, blob, 0).Err())

	outcomes, err := infra.repo.CopyAccountConfigToBlobs(ctx, organizationID, ledgerID, []string{cacheKey},
		accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: true})
	require.NoError(t, err)
	require.Equal(t, map[string]BlobCopyOutcome{cacheKey: BlobCopyWritten}, outcomes)

	fields := readCachedBalanceFields(t, infra, internalKey)

	assert.JSONEq(t, `1`, string(fields["Blocked"]), "the propagated flag must land")
	assert.JSONEq(t, `"120"`, string(fields["Available"]), "the live available amount must survive")
	assert.JSONEq(t, `"5"`, string(fields["OnHold"]), "the live on-hold amount must survive")
	assert.JSONEq(t, `3`, string(fields["Version"]), "the live version must survive")
}
