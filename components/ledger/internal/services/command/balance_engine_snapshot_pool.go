// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// BalanceEngineSnapshotPool keeps explicit transaction targets separate from
// the complete balance inventory available to the accounting accounting.
type BalanceEngineSnapshotPool struct {
	ExplicitBalances []*mmodel.Balance
	Balances         []*mmodel.Balance
	Snapshots        []accounting.BalanceSnapshot
}

// BalanceEngineSnapshotLoader is the scoped balance read used by tests and
// standalone composition helpers. Production transaction paths use the
// reader's complete-pool operation.
type BalanceEngineSnapshotLoader func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error)

// LoadBalanceEngineSnapshotPool loads and converts a complete scoped pool.
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

	companionAliases, _, err := overdraftCompanionAliases(explicitEntries)
	if err != nil {
		return BalanceEngineSnapshotPool{}, err
	}

	companions := make([]*mmodel.Balance, 0, len(companionAliases))
	if len(companionAliases) > 0 {
		if err := ctx.Err(); err != nil {
			return BalanceEngineSnapshotPool{}, fmt.Errorf("load balance engine snapshot pool: %w", err)
		}

		companions, err = loader(ctx, organizationID, ledgerID, companionAliases)
		if err != nil {
			return BalanceEngineSnapshotPool{}, fmt.Errorf("load optional overdraft companion snapshots: %w", err)
		}
	}

	all := append(append(make([]*mmodel.Balance, 0, len(explicit)+len(companions)), explicit...), companions...)

	return BuildBalanceEngineSnapshotPool(ctx, organizationID, ledgerID, explicitAliases, explicit, all)
}

// BuildBalanceEngineSnapshotPool validates and converts balances already
// loaded by the query layer.
func BuildBalanceEngineSnapshotPool(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	explicitAliases []string,
	explicit, balances []*mmodel.Balance,
) (BalanceEngineSnapshotPool, error) {
	if err := ctx.Err(); err != nil {
		return BalanceEngineSnapshotPool{}, fmt.Errorf("load balance engine snapshot pool: %w", err)
	}

	if organizationID == uuid.Nil || ledgerID == uuid.Nil {
		return BalanceEngineSnapshotPool{}, fmt.Errorf("load balance engine snapshot pool: organization and ledger IDs must be nonzero")
	}

	explicitAliases = sortedUniqueStrings(explicitAliases)

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

	allEntries, err := balanceSnapshotEntries(organizationID, ledgerID, balances)
	if err != nil {
		return BalanceEngineSnapshotPool{}, err
	}

	if err := validateUniqueSnapshotEntries(allEntries); err != nil {
		return BalanceEngineSnapshotPool{}, err
	}

	if err := validateSnapshotPoolCoverage(explicitEntries, allEntries); err != nil {
		return BalanceEngineSnapshotPool{}, err
	}

	sortSnapshotEntries(explicitEntries)
	sortSnapshotEntries(allEntries)

	return BalanceEngineSnapshotPool{
		ExplicitBalances: balancesFromSnapshotEntries(explicitEntries),
		Balances:         balancesFromSnapshotEntries(allEntries),
		Snapshots:        snapshotsFromEntries(allEntries),
	}, nil
}

type balanceSnapshotEntry struct {
	balance  *mmodel.Balance
	snapshot accounting.BalanceSnapshot
}

func validateSnapshotPoolCoverage(explicit, all []balanceSnapshotEntry) error {
	allByRef := make(map[string]balanceSnapshotEntry, len(all))
	for _, entry := range all {
		allByRef[entry.snapshot.BalanceRef] = entry
	}

	allowed := make(map[string]uuid.UUID, len(explicit)*2)
	for _, entry := range explicit {
		allowed[entry.snapshot.BalanceRef] = entry.snapshot.AccountID
		if entry.snapshot.Key != constant.OverdraftBalanceKey {
			allowed[mtransaction.AliasKey(entry.snapshot.Alias, constant.OverdraftBalanceKey)] = entry.snapshot.AccountID
		}

		pooled, ok := allByRef[entry.snapshot.BalanceRef]
		if !ok {
			return fmt.Errorf("load balance engine snapshot pool: explicit balance %q is missing from complete pool", entry.snapshot.BalanceRef)
		}

		if !equalBalanceEngineSnapshot(pooled.snapshot, entry.snapshot) {
			return fmt.Errorf("load balance engine snapshot pool: explicit balance %q diverges from complete pool", entry.snapshot.BalanceRef)
		}
	}

	for _, entry := range all {
		expectedAccountID, ok := allowed[entry.snapshot.BalanceRef]
		if !ok {
			return fmt.Errorf("load balance engine snapshot pool: complete pool returned unrelated balance %q", entry.snapshot.BalanceRef)
		}

		if entry.snapshot.AccountID != expectedAccountID {
			return fmt.Errorf("load balance engine snapshot pool: balance %q has inconsistent account identity", entry.snapshot.BalanceRef)
		}
	}

	return nil
}

func equalBalanceEngineSnapshot(left, right accounting.BalanceSnapshot) bool {
	return left.BalanceRef == right.BalanceRef &&
		left.ID == right.ID &&
		left.AccountID == right.AccountID &&
		left.AccountType == right.AccountType &&
		left.AssetCode == right.AssetCode &&
		left.Alias == right.Alias &&
		left.Key == right.Key &&
		left.Direction == right.Direction &&
		left.BalanceScope == right.BalanceScope &&
		left.Available.Equal(right.Available) &&
		left.OnHold.Equal(right.OnHold) &&
		left.OverdraftUsed.Equal(right.OverdraftUsed) &&
		left.OverdraftLimit.Equal(right.OverdraftLimit) &&
		left.Version == right.Version &&
		left.AllowSending == right.AllowSending &&
		left.AllowReceiving == right.AllowReceiving &&
		left.AllowOverdraft == right.AllowOverdraft &&
		left.OverdraftLimitEnabled == right.OverdraftLimitEnabled
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

func balanceToEngineSnapshot(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) (accounting.BalanceSnapshot, error) {
	if balance == nil {
		return accounting.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: nil balance")
	}

	if balance.OrganizationID != organizationID.String() || balance.LedgerID != ledgerID.String() {
		return accounting.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: balance %q is outside the requested scope", balance.ID)
	}

	balanceID, err := uuid.Parse(balance.ID)
	if err != nil || balanceID == uuid.Nil {
		return accounting.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: invalid balance ID %q", balance.ID)
	}

	accountID, err := uuid.Parse(balance.AccountID)
	if err != nil || accountID == uuid.Nil {
		return accounting.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: invalid account ID %q", balance.AccountID)
	}

	transactionBalance, err := balance.ToTransactionBalance()
	if err != nil {
		return accounting.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: convert balance %q: %w", balance.ID, err)
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
		return accounting.BalanceSnapshot{}, fmt.Errorf("load balance engine snapshot pool: balance %q has invalid scope %q", balance.ID, balanceScope)
	}

	return accounting.BalanceSnapshot{
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

func snapshotsFromEntries(entries []balanceSnapshotEntry) []accounting.BalanceSnapshot {
	snapshots := make([]accounting.BalanceSnapshot, len(entries))
	for i := range entries {
		snapshots[i] = entries[i].snapshot
	}

	return snapshots
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
