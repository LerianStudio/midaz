// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package accounting defines the ledger's replaceable accounting-engine
// contract. An engine implementation owns atomic live-balance validation and
// mutation. It does not own transaction composition, durable row projection, or
// storage-specific recovery orchestration.
package accounting

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountBlockException is the single-use grant one transaction presents to
// authorize a specific primary outflow while the account is blocked. The
// engine validates the live Redis grant and consumes it in the same atomic
// commit that publishes the balance movements.
type AccountBlockException struct {
	ExceptionID       uuid.UUID       `json:"exceptionId"`
	Alias             string          `json:"alias"`
	Amount            decimal.Decimal `json:"amount"`
	PrimaryPostingRef string          `json:"primaryPostingRef"`
}

// Transaction groups postings in their execution order.
type Transaction struct {
	OrganizationID uuid.UUID `json:"organizationId"`
	LedgerID       uuid.UUID `json:"ledgerId"`
	ID             uuid.UUID `json:"id"`
	// RejectBlockedBalances applies the live account-level barrier to every
	// balance this transaction touches. Cancellation disables the barrier so
	// held funds can always be returned.
	RejectBlockedBalances bool                   `json:"rejectBlockedBalances"`
	AccountBlockException *AccountBlockException `json:"accountBlockException,omitempty"`
	BalanceRequirements   []BalanceRequirement   `json:"balanceRequirements"`
	Postings              []Posting              `json:"postings"`
}

// Execution is one ordered accounting operation whose organization and ledger
// identify the primary scope for receipts, guards, protection, and recovery.
// Individual transactions and balances may belong to other scopes. All UUIDs
// must be nonzero. ExecutionID identifies one execution of an action and differs
// between creation, commitment and cancellation.
// Balances is the available snapshot pool, not the set of explicit input legs.
// Implementations validate references and numeric invariants before any writes,
// and evaluate balance requirements against live cached values before writes.
// Storage adapters own the wire DTO, including canonical decimal strings,
// lossless versions and empty-array encoding; these types are not a Lua wire format.
type Execution struct {
	OrganizationID uuid.UUID         `json:"organizationId"`
	LedgerID       uuid.UUID         `json:"ledgerId"`
	ExecutionID    uuid.UUID         `json:"executionId"`
	Transactions   []Transaction     `json:"transactions"`
	Balances       []BalanceSnapshot `json:"balances"`
}
