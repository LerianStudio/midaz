// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// TestAuthorizeRequest_UsesSuppliedShardOverridingFNV asserts that when
// AuthorizeRequest.Metadata carries a "shard:<alias>#<balanceKey>" entry,
// the authorizer honors it instead of FNV-resolving the alias. Without the
// override, the FNV shard and the manager-override shard can disagree for a
// migrated account; the balance lives at the override shard's worker but
// FNV would look it up in the legacy shard and return BALANCE_NOT_FOUND.
//
// To simulate the divergence, we seed the balance into a shard that FNV does
// NOT resolve to, then issue an Authorize request with a metadata override
// pointing at the correct shard. With the override consumed, authorization
// succeeds; without it (control group), authorization fails with
// BALANCE_NOT_FOUND.
func TestAuthorizeRequest_UsesSuppliedShardOverridingFNV(t *testing.T) {
	router := shard.NewRouter(8)

	const (
		orgID    = "org-1"
		ledgerID = "ledger-1"
		alias    = "@alice"
	)

	fnvShard := router.ResolveBalance(alias, constant.DefaultBalanceKey)
	overrideShard := (fnvShard + 1) % router.ShardCount()

	eng := New(router, wal.NewNoopWriter())
	defer eng.Close()

	// Seed the balance into the OVERRIDE shard's worker directly, bypassing
	// UpsertBalances (which uses FNV to place the balance) so we can observe
	// the divergence condition. UpsertBalances would normally co-locate the
	// insert with the FNV shard, hiding the bug.
	worker := eng.workers[overrideShard]
	worker.mu.Lock()
	worker.balances[balanceLookupKey(orgID, ledgerID, alias, constant.DefaultBalanceKey)] = &Balance{
		ID:             "b1",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountAlias:   alias,
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      10000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}
	worker.balances[balanceLookupKey(orgID, ledgerID, "@counter", constant.DefaultBalanceKey)] = &Balance{
		ID:             "b2",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountAlias:   "@counter",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      10000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}
	worker.mu.Unlock()

	counterFNVShard := router.ResolveBalance("@counter", constant.DefaultBalanceKey)
	if counterFNVShard != overrideShard {
		// Also place the counter in its FNV worker so the override-less
		// control request would not trip over the counter balance first.
		counterWorker := eng.workers[counterFNVShard]
		counterWorker.mu.Lock()
		counterWorker.balances[balanceLookupKey(orgID, ledgerID, "@counter", constant.DefaultBalanceKey)] = &Balance{
			ID:             "b2",
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			AccountAlias:   "@counter",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		}
		counterWorker.mu.Unlock()
	}

	makeRequest := func(metadata map[string]string) *authorizerv1.AuthorizeRequest {
		return &authorizerv1.AuthorizeRequest{
			TransactionId:     "tx-1",
			OrganizationId:    orgID,
			LedgerId:          ledgerID,
			TransactionStatus: constant.CREATED,
			Operations: []*authorizerv1.BalanceOperation{
				{OperationAlias: "0", AccountAlias: alias, BalanceKey: constant.DefaultBalanceKey, AssetCode: "USD", Amount: 100, Scale: 2, Operation: constant.DEBIT},
				{OperationAlias: "1", AccountAlias: "@counter", BalanceKey: constant.DefaultBalanceKey, AssetCode: "USD", Amount: 100, Scale: 2, Operation: constant.CREDIT},
			},
			Metadata: metadata,
		}
	}

	// Control: no metadata override. FNV routes the alias to the wrong shard;
	// the balance can't be found.
	controlResp, err := eng.Authorize(makeRequest(nil))
	require.NoError(t, err)
	require.False(t, controlResp.GetAuthorized(), "control (no override) must fail: FNV resolves to the wrong shard")
	assert.Equal(t, RejectionBalanceNotFound, controlResp.GetRejectionCode())

	// With metadata override, the authorizer consults the caller-supplied
	// shard ID and locates the balance in the override worker.
	metadata := map[string]string{
		shardOverrideMetadataKey(alias, constant.DefaultBalanceKey): strconv.Itoa(overrideShard),
	}

	resp, err := eng.Authorize(makeRequest(metadata))
	require.NoError(t, err)
	require.True(t, resp.GetAuthorized(), "override-driven authorize should succeed (rejection=%s msg=%s)",
		resp.GetRejectionCode(), resp.GetRejectionMessage())
}

// TestAuthorizeRequest_InvalidShardOverrideFallsBackToFNV asserts that a
// malformed or out-of-range metadata override is treated as absent: the
// authorizer falls back to the FNV behaviour instead of panicking or rejecting.
func TestAuthorizeRequest_InvalidShardOverrideFallsBackToFNV(t *testing.T) {
	router := shard.NewRouter(4)

	eng := New(router, wal.NewNoopWriter())
	defer eng.Close()

	const (
		orgID    = "org-x"
		ledgerID = "ledger-x"
		alias    = "@trader"
	)

	fnvShard := router.ResolveBalance(alias, constant.DefaultBalanceKey)

	// Seed both balances into their respective FNV workers so the
	// override-less fallback path can resolve them.
	seed := func(a, id string) {
		wid := router.ResolveBalance(a, constant.DefaultBalanceKey)
		w := eng.workers[wid]
		w.mu.Lock()
		w.balances[balanceLookupKey(orgID, ledgerID, a, constant.DefaultBalanceKey)] = &Balance{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			AccountAlias:   a,
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		}
		w.mu.Unlock()
	}

	seed(alias, "b1")
	seed("@counter", "b2")

	_ = fnvShard // retained for documentation / future assertions

	cases := map[string]string{
		"out_of_range": strconv.Itoa(router.ShardCount() + 5),
		"negative":     "-1",
		"not_numeric":  "not-a-number",
	}

	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			metadata := map[string]string{
				shardOverrideMetadataKey(alias, constant.DefaultBalanceKey): bad,
			}

			resp, err := eng.Authorize(&authorizerv1.AuthorizeRequest{
				TransactionId:     "tx-" + name,
				OrganizationId:    orgID,
				LedgerId:          ledgerID,
				TransactionStatus: constant.CREATED,
				Operations: []*authorizerv1.BalanceOperation{
					{OperationAlias: "0", AccountAlias: alias, BalanceKey: constant.DefaultBalanceKey, AssetCode: "USD", Amount: 50, Scale: 2, Operation: constant.DEBIT},
					{OperationAlias: "1", AccountAlias: "@counter", BalanceKey: constant.DefaultBalanceKey, AssetCode: "USD", Amount: 50, Scale: 2, Operation: constant.CREDIT},
				},
				Metadata: metadata,
			})
			require.NoError(t, err)
			assert.True(t, resp.GetAuthorized(), "invalid override should fall back to FNV, not reject (got code=%s msg=%s)",
				resp.GetRejectionCode(), resp.GetRejectionMessage())
		})
	}
}
