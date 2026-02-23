// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// Balance is the in-memory balance state for authorization decisions.
type Balance struct {
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

	copy := *b

	return &copy
}
