// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestLoadBalanceEngineSnapshotPoolIncludesOptionalCompanionsIndependentlyOfSnapshotSettings(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), snapshotPoolContextKey{}, "same-context")
	organizationID := uuid.MustParse("bca1bafb-812f-4d44-b8ff-67992b8f15d9")
	ledgerID := uuid.MustParse("b67e5284-04f8-49ea-8c83-17a086e755ca")
	accountID := uuid.MustParse("693f80b0-e7bd-4487-869f-8bd24996b02e")
	explicit := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", constant.DefaultBalanceKey)
	explicit.Settings = nil
	companion := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", constant.OverdraftBalanceKey)
	companion.Direction = constant.DirectionDebit
	companion.Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}

	var calls [][]string
	loader := func(gotCtx context.Context, gotOrganizationID, gotLedgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
		require.Same(t, ctx, gotCtx)
		assert.Equal(t, organizationID, gotOrganizationID)
		assert.Equal(t, ledgerID, gotLedgerID)
		calls = append(calls, append([]string(nil), aliases...))

		if len(calls) == 1 {
			return []*mmodel.Balance{explicit}, nil
		}

		return []*mmodel.Balance{companion}, nil
	}

	pool, err := LoadBalanceEngineSnapshotPool(ctx, organizationID, ledgerID, []string{"@alice#default"}, loader)
	require.NoError(t, err)
	require.Equal(t, [][]string{{"@alice#default"}, {"@alice#overdraft"}}, calls)
	require.Equal(t, []*mmodel.Balance{explicit}, pool.ExplicitBalances)
	require.Equal(t, []*mmodel.Balance{explicit, companion}, pool.Balances)
	require.Len(t, pool.Snapshots, 2)

	assert.Equal(t, "@alice#default", pool.Snapshots[0].BalanceRef)
	assert.Equal(t, mmodel.BalanceScopeTransactional, pool.Snapshots[0].BalanceScope)
	assert.False(t, pool.Snapshots[0].AllowOverdraft)
	assert.Equal(t, "@alice#overdraft", pool.Snapshots[1].BalanceRef)
	assert.Equal(t, mmodel.BalanceScopeInternal, pool.Snapshots[1].BalanceScope)
	assert.Equal(t, accountID, pool.Snapshots[1].AccountID)
	assert.NoError(t, rejectInternalScopeBalances(context.Background(), pool.ExplicitBalances))
	assert.Error(t, rejectInternalScopeBalances(context.Background(), pool.Balances),
		"the full pool is intentionally not the explicit-target validation set")
}

func TestLoadBalanceEngineSnapshotPoolTreatsOnlyAbsentCompanionsAsOptional(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("8f809d3e-48e8-4dc8-b850-3d7c78516316")
	ledgerID := uuid.MustParse("5e874897-0ff7-4b1d-9a18-3db752b1f5c8")
	explicit := snapshotPoolBalance(organizationID, ledgerID,
		uuid.MustParse("83b13c86-aeb0-4f5a-970c-8790fed9a49e"), "@alice", constant.DefaultBalanceKey)

	t.Run("empty optional read leaves the companion absent", func(t *testing.T) {
		calls := 0
		pool, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
			[]string{"@alice#default"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
				calls++
				if calls == 1 {
					return []*mmodel.Balance{explicit}, nil
				}

				return []*mmodel.Balance{}, nil
			})

		require.NoError(t, err)
		assert.Equal(t, 2, calls)
		assert.Equal(t, []*mmodel.Balance{explicit}, pool.ExplicitBalances)
		assert.Equal(t, []*mmodel.Balance{explicit}, pool.Balances)
		require.Len(t, pool.Snapshots, 1)
		assert.Equal(t, "@alice#default", pool.Snapshots[0].BalanceRef)
	})

	t.Run("optional read infrastructure error is not disguised as absence", func(t *testing.T) {
		infrastructureErr := errors.New("database unavailable")
		calls := 0
		_, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
			[]string{"@alice#default"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
				calls++
				if calls == 1 {
					return []*mmodel.Balance{explicit}, nil
				}

				return nil, infrastructureErr
			})

		require.ErrorIs(t, err, infrastructureErr)
		assert.Contains(t, err.Error(), "optional overdraft companion")
	})

	t.Run("partial optional read includes only the companion that exists", func(t *testing.T) {
		bob := snapshotPoolBalance(organizationID, ledgerID,
			uuid.MustParse("bd8a93f5-c099-4b53-9351-f5f53227a36e"), "@bob", constant.DefaultBalanceKey)
		companion := snapshotPoolBalance(organizationID, ledgerID,
			uuid.MustParse(explicit.AccountID), "@alice", constant.OverdraftBalanceKey)
		calls := 0
		pool, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
			[]string{"@alice#default", "@bob#default"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
				calls++
				if calls == 1 {
					return []*mmodel.Balance{bob, explicit}, nil
				}

				return []*mmodel.Balance{companion}, nil
			})

		require.NoError(t, err)
		assert.Equal(t, []string{"@alice#default", "@alice#overdraft", "@bob#default"}, snapshotPoolRefs(pool.Snapshots))
	})
}

func TestLoadBalanceEngineSnapshotPoolMapsTheCompleteSnapshot(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("9d314aa6-a8d4-4e3b-a33d-c7aaa269b223")
	ledgerID := uuid.MustParse("f41b3ab0-3c99-4dfa-b86b-5f8b37697f22")
	accountID := uuid.MustParse("ad9d70a9-f128-4272-90ff-9e0bc7b0ff12")
	balanceID := uuid.MustParse("bf4b9194-dc49-44f8-824d-c5ad619d5388")
	limit := "125.50"
	explicit := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", "available")
	explicit.ID = balanceID.String()
	explicit.AccountType = "checking"
	explicit.AssetCode = "USD"
	explicit.Available = decimal.RequireFromString("90.25")
	explicit.OnHold = decimal.RequireFromString("3.75")
	explicit.OverdraftUsed = decimal.RequireFromString("10.5")
	explicit.Version = 42
	explicit.AllowSending = false
	explicit.AllowReceiving = true
	explicit.Direction = constant.DirectionDebit
	explicit.Settings = &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}
	calls := 0
	pool, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
		[]string{"@alice#available"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
			calls++
			if calls == 1 {
				return []*mmodel.Balance{explicit}, nil
			}

			return nil, nil
		})

	require.NoError(t, err)
	require.Len(t, pool.Snapshots, 1)
	assert.Equal(t, engine.BalanceSnapshot{
		BalanceRef:            "@alice#available",
		ID:                    balanceID,
		AccountID:             accountID,
		AccountType:           "checking",
		AssetCode:             "USD",
		Alias:                 "@alice",
		Key:                   "available",
		Direction:             constant.DirectionDebit,
		BalanceScope:          mmodel.BalanceScopeTransactional,
		Available:             decimal.RequireFromString("90.25"),
		OnHold:                decimal.RequireFromString("3.75"),
		OverdraftUsed:         decimal.RequireFromString("10.5"),
		OverdraftLimit:        decimal.RequireFromString("125.50"),
		Version:               42,
		AllowSending:          false,
		AllowReceiving:        true,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
	}, pool.Snapshots[0])
}

func TestLoadBalanceEngineSnapshotPoolDeduplicatesCandidatesAndOrdersThePool(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("7253e939-fbe5-416f-aeef-7c8487c24ee8")
	ledgerID := uuid.MustParse("9450249c-f36c-42d7-82e5-ae43b8286856")
	aliceAccountID := uuid.MustParse("f5247506-a806-4f54-b369-e7832c7913b3")
	bobAccountID := uuid.MustParse("d587585f-5458-46c5-81c4-22faf98a8c57")
	aliceDefault := snapshotPoolBalance(organizationID, ledgerID, aliceAccountID, "@alice", constant.DefaultBalanceKey)
	aliceAvailable := snapshotPoolBalance(organizationID, ledgerID, aliceAccountID, "@alice", "available")
	bobDefault := snapshotPoolBalance(organizationID, ledgerID, bobAccountID, "@bob", constant.DefaultBalanceKey)
	aliceCompanion := snapshotPoolBalance(organizationID, ledgerID, aliceAccountID, "@alice", constant.OverdraftBalanceKey)
	bobCompanion := snapshotPoolBalance(organizationID, ledgerID, bobAccountID, "@bob", constant.OverdraftBalanceKey)

	var calls [][]string
	pool, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
		[]string{"@bob#default", "@alice#available", "@alice#default", "@bob#default"},
		func(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			calls = append(calls, append([]string(nil), aliases...))
			if len(calls) == 1 {
				return []*mmodel.Balance{bobDefault, aliceDefault, aliceAvailable}, nil
			}

			return []*mmodel.Balance{bobCompanion, aliceCompanion}, nil
		})

	require.NoError(t, err)
	assert.Equal(t, [][]string{
		{"@alice#available", "@alice#default", "@bob#default"},
		{"@alice#overdraft", "@bob#overdraft"},
	}, calls)
	assert.Equal(t, []string{"@alice#available", "@alice#default", "@alice#overdraft", "@bob#default", "@bob#overdraft"}, snapshotPoolRefs(pool.Snapshots))
	assert.Equal(t, []string{"@alice#available", "@alice#default", "@bob#default"}, snapshotPoolBalanceRefs(pool.ExplicitBalances))
	assert.Equal(t, snapshotPoolRefs(pool.Snapshots), snapshotPoolBalanceRefs(pool.Balances), "models and snapshots must stay aligned")
}

func TestLoadBalanceEngineSnapshotPoolRejectsCompanionWithWrongAccountIdentity(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("36e42be2-8e77-4435-899f-4883a7fdb72d")
	ledgerID := uuid.MustParse("eed14797-4081-40b2-b0db-eb6025f0a822")
	explicit := snapshotPoolBalance(organizationID, ledgerID,
		uuid.MustParse("46df7421-bb65-4aa5-927b-bd50bcb55b39"), "@alice", constant.DefaultBalanceKey)
	wrongCompanion := snapshotPoolBalance(organizationID, ledgerID,
		uuid.MustParse("a7276afa-ef78-407f-8abd-fadfd08ace6f"), "@alice", constant.OverdraftBalanceKey)
	calls := 0

	_, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
		[]string{"@alice#default"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
			calls++
			if calls == 1 {
				return []*mmodel.Balance{explicit}, nil
			}

			return []*mmodel.Balance{wrongCompanion}, nil
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "account identity")
}

func TestLoadBalanceEngineSnapshotPoolRejectsInvalidIdentityAndScope(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("43be61e5-aec0-4cc3-91b8-116838c5f30a")
	ledgerID := uuid.MustParse("1c2d667a-e92d-4703-94cf-94321b8853a3")
	accountID := uuid.MustParse("cab9eebd-7a7e-4b26-93f0-b53a09062d4f")
	otherOrganizationID := uuid.MustParse("4e99fdf4-0c6b-434c-975a-7117ae928d9d")
	otherLedgerID := uuid.MustParse("d815da43-a50c-458b-bade-4ef09d5cf644")
	invalidLimit := "not-a-decimal"

	tests := []struct {
		name        string
		mutate      func(*mmodel.Balance)
		wantMessage string
	}{
		{name: "nil balance UUID", mutate: func(balance *mmodel.Balance) { balance.ID = uuid.Nil.String() }, wantMessage: "invalid balance ID"},
		{name: "invalid balance UUID", mutate: func(balance *mmodel.Balance) { balance.ID = "invalid" }, wantMessage: "invalid balance ID"},
		{name: "nil account UUID", mutate: func(balance *mmodel.Balance) { balance.AccountID = uuid.Nil.String() }, wantMessage: "invalid account ID"},
		{name: "invalid account UUID", mutate: func(balance *mmodel.Balance) { balance.AccountID = "invalid" }, wantMessage: "invalid account ID"},
		{name: "organization mismatch", mutate: func(balance *mmodel.Balance) { balance.OrganizationID = otherOrganizationID.String() }, wantMessage: "outside the requested scope"},
		{name: "ledger mismatch", mutate: func(balance *mmodel.Balance) { balance.LedgerID = otherLedgerID.String() }, wantMessage: "outside the requested scope"},
		{name: "invalid balance scope", mutate: func(balance *mmodel.Balance) {
			balance.Settings = &mmodel.BalanceSettings{BalanceScope: "private"}
		}, wantMessage: "invalid scope"},
		{name: "invalid overdraft limit", mutate: func(balance *mmodel.Balance) {
			balance.Settings = &mmodel.BalanceSettings{
				BalanceScope:          mmodel.BalanceScopeTransactional,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &invalidLimit,
			}
		}, wantMessage: "invalid OverdraftLimit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balance := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", constant.DefaultBalanceKey)
			tt.mutate(balance)
			_, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
				[]string{"@alice#default"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
					return []*mmodel.Balance{balance}, nil
				})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

func TestLoadBalanceEngineSnapshotPoolRejectsNilRowsAndDuplicateLogicalReferences(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("e32de2b9-496d-4855-bf58-4cf4f37c4a04")
	ledgerID := uuid.MustParse("be73813c-64ce-45f8-965d-b35c39f62a4e")
	accountID := uuid.MustParse("b456ffb1-179c-498d-81f8-58d61033e670")
	balance := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", constant.DefaultBalanceKey)

	t.Run("nil row", func(t *testing.T) {
		_, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
			[]string{"@alice#default"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
				return []*mmodel.Balance{nil}, nil
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil balance")
	})

	t.Run("duplicate logical reference", func(t *testing.T) {
		duplicate := *balance
		duplicate.ID = uuid.New().String()
		_, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
			[]string{"@alice#default"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
				return []*mmodel.Balance{balance, &duplicate}, nil
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate balance reference")
	})
}

func TestLoadBalanceEngineSnapshotPoolNormalizesAliasAndDefaultKeyOnlyOnce(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("580c767d-89cc-45d3-ac5e-f31766437fa2")
	ledgerID := uuid.MustParse("d9c9ff61-0d10-4285-bda0-6451ebc0b0ec")
	accountID := uuid.MustParse("6981a457-4bb6-40e9-9ac6-18f8eae4668e")
	explicit := snapshotPoolBalance(organizationID, ledgerID, accountID, "0#@alice#default", "")
	companion := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", constant.OverdraftBalanceKey)
	calls := 0

	pool, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
		[]string{"@alice#default"}, func(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			calls++
			if calls == 1 {
				return []*mmodel.Balance{explicit}, nil
			}

			assert.Equal(t, []string{"@alice#overdraft"}, aliases)
			return []*mmodel.Balance{companion}, nil
		})

	require.NoError(t, err)
	assert.Equal(t, []string{"@alice#default", "@alice#overdraft"}, snapshotPoolRefs(pool.Snapshots))
	assert.NotContains(t, snapshotPoolRefs(pool.Snapshots), "@alice#default#default")
}

func TestLoadBalanceEngineSnapshotPoolStopsOnCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	_, err := LoadBalanceEngineSnapshotPool(ctx, uuid.New(), uuid.New(), []string{"@alice#default"},
		func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
			called = true
			return nil, nil
		})

	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, called)
}

func TestLoadBalanceEngineSnapshotPoolLeavesExplicitInternalTargetsForTheExplicitGuard(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("73340d4e-89b4-4c08-99f1-f86268083b35")
	ledgerID := uuid.MustParse("b45d74f4-cfbf-4983-9c11-2f45289c777e")
	accountID := uuid.MustParse("cc8e131f-4e58-4c26-8e1f-d839d3dfe42d")
	explicit := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", constant.OverdraftBalanceKey)
	explicit.Direction = constant.DirectionDebit
	explicit.Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}
	calls := 0

	pool, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
		[]string{"@alice#overdraft"}, func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
			calls++
			return []*mmodel.Balance{explicit}, nil
		})

	require.NoError(t, err)
	assert.Equal(t, 1, calls, "an explicitly loaded companion must not be qualified or fetched twice")
	require.Len(t, pool.ExplicitBalances, 1)
	assert.Error(t, rejectInternalScopeBalances(context.Background(), pool.ExplicitBalances))
}

func TestLoadBalanceEngineSnapshotPoolDoesNotRefetchMixedExplicitCompanion(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("66093ed1-5ad9-4346-a62b-046628d6aa28")
	ledgerID := uuid.MustParse("6020684a-5782-447a-9127-c45851963f45")
	accountID := uuid.MustParse("2db6e14a-bc51-4ce8-95bf-16de1c6889ad")
	primary := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", constant.DefaultBalanceKey)
	companion := snapshotPoolBalance(organizationID, ledgerID, accountID, "@alice", constant.OverdraftBalanceKey)
	companion.Direction = constant.DirectionDebit
	companion.Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}
	calls := 0

	pool, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
		[]string{"@alice#default", "@alice#overdraft"},
		func(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			calls++
			assert.Equal(t, []string{"@alice#default", "@alice#overdraft"}, aliases)
			return []*mmodel.Balance{companion, primary}, nil
		})

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, []string{"@alice#default", "@alice#overdraft"}, snapshotPoolRefs(pool.Snapshots))
	require.Len(t, pool.ExplicitBalances, 2)
	assert.Error(t, rejectInternalScopeBalances(context.Background(), pool.ExplicitBalances))
}

func TestLoadBalanceEngineSnapshotPoolRejectsMixedExplicitCompanionFromAnotherAccount(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("9c47e03b-51f4-45e4-9a8e-c5a9da4a2035")
	ledgerID := uuid.MustParse("9d4a3c87-06b0-41bd-8d83-e391e8b2be65")
	primary := snapshotPoolBalance(organizationID, ledgerID,
		uuid.MustParse("f4a15c7e-443f-4f78-8835-871a538ff26d"), "@alice", constant.DefaultBalanceKey)
	companion := snapshotPoolBalance(organizationID, ledgerID,
		uuid.MustParse("612a8db1-9708-470d-8543-cd060fd7f6f4"), "@alice", constant.OverdraftBalanceKey)

	_, err := LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID,
		[]string{"@alice#default", "@alice#overdraft"},
		func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
			return []*mmodel.Balance{companion, primary}, nil
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "explicit companion")
	assert.Contains(t, err.Error(), "account identity")
}

type snapshotPoolContextKey struct{}

func snapshotPoolBalance(organizationID, ledgerID, accountID uuid.UUID, alias, key string) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          alias,
		Key:            key,
		AssetCode:      "BRL",
		Available:      decimal.NewFromInt(100),
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionCredit,
		Settings:       mmodel.NewDefaultBalanceSettings(),
	}
}

func snapshotPoolRefs(snapshots []engine.BalanceSnapshot) []string {
	refs := make([]string, len(snapshots))
	for i := range snapshots {
		refs[i] = snapshots[i].BalanceRef
	}

	return refs
}

func snapshotPoolBalanceRefs(balances []*mmodel.Balance) []string {
	refs := make([]string, len(balances))
	for i := range balances {
		refs[i] = mtransaction.AliasKey(mtransaction.SplitAlias(balances[i].Alias), balances[i].Key)
	}

	return refs
}
