// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// BalanceEngineSnapshotPool keeps explicit transaction targets separate from
// the complete balance inventory available to the accounting engine.
type BalanceEngineSnapshotPool struct {
	ExplicitBalances []*mmodel.Balance
	Balances         []*mmodel.Balance
	Snapshots        []engine.BalanceSnapshot
}

// BalanceEngineSnapshotLoader is the scoped balance read used to assemble an
// engine snapshot pool. Its shape matches query.UseCase.GetBalances.
type BalanceEngineSnapshotLoader func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error)

// LoadBalanceEngineSnapshotPool loads explicit balances and any available
// overdraft companions in the same organization and ledger scope.
func LoadBalanceEngineSnapshotPool(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	explicitAliases []string,
	loader BalanceEngineSnapshotLoader,
) (BalanceEngineSnapshotPool, error) {
	if err := ctx.Err(); err != nil {
		return BalanceEngineSnapshotPool{}, fmt.Errorf("load balance engine snapshot pool: %w", err)
	}

	if organizationID == uuid.Nil || ledgerID == uuid.Nil {
		return BalanceEngineSnapshotPool{}, fmt.Errorf("load balance engine snapshot pool: organization and ledger IDs must be nonzero")
	}

	if loader == nil {
		return BalanceEngineSnapshotPool{}, fmt.Errorf("load balance engine snapshot pool: balance loader is required")
	}

	explicitAliases = sortedUniqueStrings(explicitAliases)

	explicit, err := loader(ctx, organizationID, ledgerID, explicitAliases)
	if err != nil {
		return BalanceEngineSnapshotPool{}, fmt.Errorf("load explicit balance snapshots: %w", err)
	}

	explicitEntries, err := balanceSnapshotEntries(organizationID, ledgerID, explicit)
	if err != nil {
		return BalanceEngineSnapshotPool{}, err
	}

	if err := validateUniqueSnapshotEntries(explicitEntries); err != nil {
		return BalanceEngineSnapshotPool{}, err
	}

	requestedExplicit := make(map[string]struct{}, len(explicitAliases))
	for _, alias := range explicitAliases {
		requestedExplicit[alias] = struct{}{}
	}

	for _, entry := range explicitEntries {
		if _, requested := requestedExplicit[entry.snapshot.BalanceRef]; !requested {
			return BalanceEngineSnapshotPool{}, fmt.Errorf("load balance engine snapshot pool: explicit loader returned unrequested balance %q", entry.snapshot.BalanceRef)
		}
	}

	companionAliases, companionAccounts, err := overdraftCompanionAliases(explicitEntries)
	if err != nil {
		return BalanceEngineSnapshotPool{}, err
	}

	companionEntries := make([]balanceSnapshotEntry, 0)

	if len(companionAliases) > 0 {
		if err := ctx.Err(); err != nil {
			return BalanceEngineSnapshotPool{}, fmt.Errorf("load balance engine snapshot pool: %w", err)
		}

		companions, loadErr := loader(ctx, organizationID, ledgerID, companionAliases)
		if loadErr != nil {
			return BalanceEngineSnapshotPool{}, fmt.Errorf("load optional overdraft companion snapshots: %w", loadErr)
		}

		companionEntries, err = balanceSnapshotEntries(organizationID, ledgerID, companions)
		if err != nil {
			return BalanceEngineSnapshotPool{}, err
		}

		if err := validateCompanionSnapshotAccounts(companionEntries, companionAccounts); err != nil {
			return BalanceEngineSnapshotPool{}, err
		}
	}

	if err := validateUniqueSnapshotEntries(explicitEntries, companionEntries); err != nil {
		return BalanceEngineSnapshotPool{}, err
	}

	sortSnapshotEntries(explicitEntries)
	allEntries := append(append(make([]balanceSnapshotEntry, 0, len(explicitEntries)+len(companionEntries)), explicitEntries...), companionEntries...)
	sortSnapshotEntries(allEntries)

	return BalanceEngineSnapshotPool{
		ExplicitBalances: balancesFromSnapshotEntries(explicitEntries),
		Balances:         balancesFromSnapshotEntries(allEntries),
		Snapshots:        snapshotsFromEntries(allEntries),
	}, nil
}

type balanceSnapshotEntry struct {
	balance  *mmodel.Balance
	snapshot engine.BalanceSnapshot
}

func validateCompanionSnapshotAccounts(entries []balanceSnapshotEntry, accounts map[string]uuid.UUID) error {
	for _, entry := range entries {
		expectedAccountID, requested := accounts[entry.snapshot.BalanceRef]
		if !requested {
			return fmt.Errorf("load balance engine snapshot pool: companion loader returned unrequested balance %q", entry.snapshot.BalanceRef)
		}

		if entry.snapshot.AccountID != expectedAccountID {
			return fmt.Errorf("load balance engine snapshot pool: companion %q has inconsistent account identity", entry.snapshot.BalanceRef)
		}
	}

	return nil
}

func balanceSnapshotEntries(organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) ([]balanceSnapshotEntry, error) {
	entries := make([]balanceSnapshotEntry, 0, len(balances))
	for _, balance := range balances {
		snapshot, err := balanceToEngineSnapshot(organizationID, ledgerID, balance)
		if err != nil {
			return nil, err
		}

		entries = append(entries, balanceSnapshotEntry{balance: balance, snapshot: snapshot})
	}

	return entries, nil
}

func overdraftCompanionAliases(explicit []balanceSnapshotEntry) ([]string, map[string]uuid.UUID, error) {
	explicitAccounts := make(map[string]uuid.UUID, len(explicit))
	for _, entry := range explicit {
		explicitAccounts[entry.snapshot.BalanceRef] = entry.snapshot.AccountID
	}

	accounts := make(map[string]uuid.UUID, len(explicit))
	for _, entry := range explicit {
		if entry.snapshot.Key == constant.OverdraftBalanceKey {
			continue
		}

		alias := mtransaction.AliasKey(entry.snapshot.Alias, constant.OverdraftBalanceKey)
		if explicitAccountID, exists := explicitAccounts[alias]; exists {
			if explicitAccountID != entry.snapshot.AccountID {
				return nil, nil, fmt.Errorf("load balance engine snapshot pool: explicit companion %q has inconsistent account identity", alias)
			}

			continue
		}

		if accountID, exists := accounts[alias]; exists && accountID != entry.snapshot.AccountID {
			return nil, nil, fmt.Errorf("load balance engine snapshot pool: companion %q has ambiguous account identity", alias)
		}

		accounts[alias] = entry.snapshot.AccountID
	}

	aliases := make([]string, 0, len(accounts))
	for alias := range accounts {
		aliases = append(aliases, alias)
	}

	sort.Strings(aliases)

	return aliases, accounts, nil
}

func balanceToEngineSnapshot(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) (engine.BalanceSnapshot, error) {
	if balance == nil {
		return engine.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: nil balance")
	}

	if balance.OrganizationID != organizationID.String() || balance.LedgerID != ledgerID.String() {
		return engine.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: balance %q is outside the requested scope", balance.ID)
	}

	balanceID, err := uuid.Parse(balance.ID)
	if err != nil || balanceID == uuid.Nil {
		return engine.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: invalid balance ID %q", balance.ID)
	}

	accountID, err := uuid.Parse(balance.AccountID)
	if err != nil || accountID == uuid.Nil {
		return engine.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: invalid account ID %q", balance.AccountID)
	}

	transactionBalance, err := balance.ToTransactionBalance()
	if err != nil {
		return engine.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: convert balance %q: %w", balance.ID, err)
	}

	key := transactionBalance.Key
	if key == "" {
		key = constant.DefaultBalanceKey
	}

	alias := mtransaction.SplitAlias(transactionBalance.Alias)

	balanceScope := transactionBalance.BalanceScope
	if balanceScope == "" {
		balanceScope = mmodel.BalanceScopeTransactional
	}

	if balanceScope != mmodel.BalanceScopeTransactional && balanceScope != mmodel.BalanceScopeInternal {
		return engine.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: balance %q has invalid scope %q", balance.ID, balanceScope)
	}

	return engine.BalanceSnapshot{
		BalanceRef:            mtransaction.AliasKey(alias, key),
		ID:                    balanceID,
		AccountID:             accountID,
		AccountType:           transactionBalance.AccountType,
		AssetCode:             transactionBalance.AssetCode,
		Alias:                 alias,
		Key:                   key,
		Direction:             transactionBalance.Direction,
		BalanceScope:          balanceScope,
		Available:             transactionBalance.Available,
		OnHold:                transactionBalance.OnHold,
		OverdraftUsed:         transactionBalance.OverdraftUsed,
		OverdraftLimit:        transactionBalance.OverdraftLimit,
		Version:               transactionBalance.Version,
		AllowSending:          transactionBalance.AllowSending,
		AllowReceiving:        transactionBalance.AllowReceiving,
		AllowOverdraft:        transactionBalance.AllowOverdraft,
		OverdraftLimitEnabled: transactionBalance.OverdraftLimitEnabled,
	}, nil
}

func sortedUniqueStrings(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[value] = struct{}{}
	}

	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}

	sort.Strings(result)

	return result
}

func validateUniqueSnapshotEntries(groups ...[]balanceSnapshotEntry) error {
	refs := make(map[string]uuid.UUID)
	ids := make(map[uuid.UUID]string)

	for _, entries := range groups {
		for _, entry := range entries {
			if id, exists := refs[entry.snapshot.BalanceRef]; exists {
				return fmt.Errorf("load balance engine snapshot pool: duplicate balance reference %q for %s and %s", entry.snapshot.BalanceRef, id, entry.snapshot.ID)
			}

			if ref, exists := ids[entry.snapshot.ID]; exists {
				return fmt.Errorf("load balance engine snapshot pool: duplicate balance ID %s for %q and %q", entry.snapshot.ID, ref, entry.snapshot.BalanceRef)
			}

			refs[entry.snapshot.BalanceRef] = entry.snapshot.ID
			ids[entry.snapshot.ID] = entry.snapshot.BalanceRef
		}
	}

	return nil
}

func sortSnapshotEntries(entries []balanceSnapshotEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].snapshot.BalanceRef < entries[j].snapshot.BalanceRef
	})
}

func balancesFromSnapshotEntries(entries []balanceSnapshotEntry) []*mmodel.Balance {
	balances := make([]*mmodel.Balance, len(entries))
	for i := range entries {
		balances[i] = entries[i].balance
	}

	return balances
}

func snapshotsFromEntries(entries []balanceSnapshotEntry) []engine.BalanceSnapshot {
	snapshots := make([]engine.BalanceSnapshot, len(entries))
	for i := range entries {
		snapshots[i] = entries[i].snapshot
	}

	return snapshots
}
