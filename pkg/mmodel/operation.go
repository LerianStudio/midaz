// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/shopspring/decimal"
)

// OperationSnapshot carries system-generated per-operation context fields.
// Always populated on every operation record — non-overdraft operations carry
// "0" / "0" rather than absent fields, so the wire shape is uniform across
// the entire ledger. Additional fields may be added in the future without a
// schema migration.
//
// Key conventions (enforced via code review):
//   - camelCase (matches Midaz API JSON convention).
//   - Additive-only over time — never remove a shipped key, never repurpose.
//   - All decimals are string-encoded (preserves precision regardless of client parser).
//   - Always present. Non-overdraft operations carry "0" for both fields.
//   - Reserved for system-generated context; user-supplied tagging goes in the separate `metadata` column.
//
// For companion rows on `@account#overdraft`, the snapshot mirrors the DEFAULT
// balance's before/after — same values appear on both the primary and companion
// rows so the lifecycle is visible from either.
//
// @Description Per-operation state snapshot. Read-only, system-generated. Always populated.
type OperationSnapshot struct {
	// String-encoded decimal of the default balance's overdraftUsed BEFORE this operation mutated it.
	// "0" when the operation does not participate in the overdraft lifecycle.
	OverdraftUsedBefore string `json:"overdraftUsedBefore" example:"50"`
	// String-encoded decimal of the default balance's overdraftUsed AFTER this operation committed.
	// "0" when the operation does not participate in the overdraft lifecycle.
	OverdraftUsedAfter string `json:"overdraftUsedAfter" example:"130"`
} // @name OperationSnapshot

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
	Direction             string          `json:"direction,omitempty" enums:"debit,credit"`
	RouteID               *string         `json:"routeId,omitempty"`
	RouteCode             *string         `json:"routeCode,omitempty"`
	RouteDescription      *string         `json:"routeDescription,omitempty"`
	Metadata              map[string]any  `json:"metadata,omitempty"`
	// Snapshot is always populated — non-overdraft operations carry zero
	// values rather than absent fields, matching the always-populated wire-
	// shape contract.
	Snapshot OperationSnapshot `json:"snapshot"`
}
