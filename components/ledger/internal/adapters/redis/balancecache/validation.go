// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

func validateSnapshot(snapshot *accounting.BalanceSnapshot, allowMissingAlias bool) error {
	if snapshot.ID == uuid.Nil || snapshot.AccountID == uuid.Nil || snapshot.AccountType == "" || snapshot.AssetCode == "" {
		return errors.New("incomplete balance cache identity")
	}

	if snapshot.Version < 0 || snapshot.OnHold.IsNegative() || snapshot.OverdraftUsed.IsNegative() || snapshot.OverdraftLimit.IsNegative() {
		return errors.New("invalid balance cache numeric state")
	}

	if snapshot.Direction != "" && snapshot.Direction != "credit" && snapshot.Direction != "debit" {
		return errors.New("invalid balance cache direction")
	}

	if snapshot.BalanceScope == "" {
		snapshot.BalanceScope = "transactional"
	}

	if snapshot.BalanceScope != "transactional" && snapshot.BalanceScope != "internal" {
		return errors.New("invalid balance cache scope")
	}

	return normalizeLogicalIdentity(snapshot, allowMissingAlias)
}

func normalizeLogicalIdentity(snapshot *accounting.BalanceSnapshot, allowMissingAlias bool) error {
	if snapshot.Key == "" {
		snapshot.Key = "default"
	}

	if strings.Contains(snapshot.Key, "#") {
		return errors.New("invalid balance cache key")
	}

	if allowMissingAlias && snapshot.Alias == "" {
		return nil
	}

	alias := snapshot.Alias
	if parts := strings.Split(alias, "#"); len(parts) == 2 && parts[1] == snapshot.Key {
		alias = parts[0]
	} else if entryAlias, indexed := indexedEntryAlias(alias, snapshot.Key); indexed {
		alias = entryAlias
	}

	if alias == "" || strings.Contains(alias, "#") {
		return errors.New("inconsistent balance alias and key")
	}

	ref := alias + "#" + snapshot.Key
	if snapshot.BalanceRef != "" && snapshot.BalanceRef != ref {
		return errors.New("inconsistent balance reference")
	}

	snapshot.Alias, snapshot.BalanceRef = alias, ref

	return nil
}

// indexedEntryAlias reads the transaction entry key "<index>#<alias>#<key>"
// that 4.0.x and 3.8.x engine scripts cached as Alias. It matches only three
// parts with an ASCII-digit index and the balance key as the last part, so a
// digits-only alias in the two-part "alias#key" form is never read as an index.
func indexedEntryAlias(value, key string) (string, bool) {
	parts := strings.Split(value, "#")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] != key {
		return "", false
	}

	for _, digit := range parts[0] {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}

	return parts[1], true
}
