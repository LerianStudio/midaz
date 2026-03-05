// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"sync"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// Balance is the in-memory balance state for authorization decisions.
// Each balance has its own mutex for fine-grained per-account locking,
// replacing the previous shard-level locking that serialized all accounts
// within the same shard.
type Balance struct {
	mu             sync.Mutex `json:"-"` // per-balance lock; NOT copied by clone()
	ID             string
	OrganizationID string
	LedgerID       string
	AccountAlias   string
	BalanceKey     string
	AssetCode      string
	Available      int64
	OnHold         int64
	Scale          int32
	Version        uint64
	AccountType    string
	IsExternal     bool
	AllowSending   bool
	AllowReceiving bool
	AccountID      string
}

// balanceLookupKey builds a composite key for balance lookups.
// NOTE: Uses ":" as delimiter. This is safe because the component
// fields (ledger ID, account alias, asset code) are system-generated
// UUIDs and validated identifiers that cannot contain ":".
func balanceLookupKey(organizationID, ledgerID, alias, balanceKey string) string {
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	return organizationID + ":" + ledgerID + ":" + alias + ":" + balanceKey
}

func (b *Balance) clone() *Balance {
	if b == nil {
		return nil
	}

	return &Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountAlias:   b.AccountAlias,
		BalanceKey:     b.BalanceKey,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Scale:          b.Scale,
		Version:        b.Version,
		AccountType:    b.AccountType,
		IsExternal:     b.IsExternal,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		AccountID:      b.AccountID,
		// mu is intentionally NOT copied - clones are snapshots, not lockable references
	}
}

// overwriteFrom copies all non-mutex fields from src into b.
// PRECONDITION: The caller must hold b.mu. This method does not
// acquire the lock itself to avoid recursive locking.
func (b *Balance) overwriteFrom(snapshot *Balance, normalizedBalanceKey string) {
	if b == nil || snapshot == nil {
		return
	}

	b.ID = snapshot.ID
	b.OrganizationID = snapshot.OrganizationID
	b.LedgerID = snapshot.LedgerID
	b.AccountAlias = snapshot.AccountAlias
	b.BalanceKey = normalizedBalanceKey
	b.AssetCode = snapshot.AssetCode
	b.Available = snapshot.Available
	b.OnHold = snapshot.OnHold
	b.Scale = snapshot.Scale
	b.Version = snapshot.Version
	b.AccountType = snapshot.AccountType
	b.IsExternal = snapshot.IsExternal
	b.AllowSending = snapshot.AllowSending
	b.AllowReceiving = snapshot.AllowReceiving
	b.AccountID = snapshot.AccountID
}

func (b *Balance) overwritePolicyFrom(snapshot *Balance, normalizedBalanceKey string) {
	if b == nil || snapshot == nil {
		return
	}

	b.BalanceKey = normalizedBalanceKey
	b.AccountType = snapshot.AccountType
	b.IsExternal = snapshot.IsExternal
	b.AllowSending = snapshot.AllowSending
	b.AllowReceiving = snapshot.AllowReceiving
}
