// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/shopspring/decimal"
)

// OperationRedis is a flat Redis cache representation of an operation.
// It mirrors the essential fields from the internal operation.Operation type
// without nested structs, enabling storage in pkg/mmodel without importing
// component-internal packages.
type OperationRedis struct {
	ID                    string          `json:"id"`
	TransactionID         string          `json:"transactionId"`
	Description           string          `json:"description"`
	Type                  string          `json:"type"`
	AssetCode             string          `json:"assetCode"`
	ChartOfAccounts       string          `json:"chartOfAccounts"`
	AmountValue           decimal.Decimal `json:"amountValue"`
	BalanceAvailable      decimal.Decimal `json:"balanceAvailable"`
	BalanceOnHold         decimal.Decimal `json:"balanceOnHold"`
	BalanceVersion        int64           `json:"balanceVersion"`
	BalanceAfterAvailable decimal.Decimal `json:"balanceAfterAvailable"`
	BalanceAfterOnHold    decimal.Decimal `json:"balanceAfterOnHold"`
	BalanceAfterVersion   int64           `json:"balanceAfterVersion"`
	StatusCode            string          `json:"statusCode,omitempty"`
	StatusDescription     *string         `json:"statusDescription,omitempty"`
	BalanceID             string          `json:"balanceId"`
	AccountID             string          `json:"accountId"`
	AccountAlias          string          `json:"accountAlias"`
	BalanceKey            string          `json:"balanceKey"`
	OrganizationID        string          `json:"organizationId"`
	LedgerID              string          `json:"ledgerId"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
	Route                 string          `json:"route"`
	BalanceAffected       bool            `json:"balanceAffected"`
	Metadata              map[string]any  `json:"metadata,omitempty"`
}
