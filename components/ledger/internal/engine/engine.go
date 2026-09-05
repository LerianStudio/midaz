// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package engine defines the ledger's accounting execution contract.
// It does not own transaction composition, accounting-row projection, or storage.
package engine

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PostingType identifies an accounting mutation, independently of transaction status.
type PostingType string

const (
	// PostingDebit decreases available funds for credit or empty direction, and
	// increases them for debit direction. Only credit-direction, non-external
	// accounts with live overdraft permission and DrawAllowed may draw debt.
	PostingDebit PostingType = "debit"
	// PostingCredit reverses debit arithmetic. For credit or empty direction,
	// non-external accounts repay debt before increasing available funds, capped
	// by a positive OverdraftAmount. Debit-direction credits enforce the floor.
	PostingCredit PostingType = "credit"
	// PostingReserve increases on-hold funds without checking available funds
	// again. A preceding debit must forbid overdraft when composing a hold.
	PostingReserve PostingType = "reserve"
	// PostingUnreserve decreases on-hold funds without changing available funds.
	// Insufficient on-hold funds are an integrity failure before any write.
	PostingUnreserve PostingType = "unreserve"
	// PostingHold combines debit arithmetic and reserve in one movement and one
	// version increment. It never draws debt; external accounts retain their
	// arithmetic floor exemption, with targeting restrictions enforced by callers.
	PostingHold PostingType = "hold"
	// PostingRelease decreases on-hold funds and restores available funds using
	// the account direction. It does not perform general credit repayment: only
	// a positive OverdraftAmount permits capped repayment on a non-external,
	// credit-direction account. On-hold underflow and the directional floor apply.
	PostingRelease PostingType = "release"
)

// DrawPolicy describes path and route permission, not cached account settings.
// Execution always combines it with live account direction and overdraft settings.
type DrawPolicy string

const (
	// DrawForbidden prevents debt creation, including for pending debits.
	DrawForbidden DrawPolicy = "forbidden"
	// DrawAllowed permits debt creation only when live account settings allow it.
	DrawAllowed DrawPolicy = "allowed"
	// DrawRouteDenied preserves route denial separately from account eligibility.
	DrawRouteDenied DrawPolicy = "route_denied"
)

// Posting addresses a logical alias#key, never a physical storage key.
// Ref must be unique within its transaction. Amount must be positive and
// OverdraftAmount nonnegative; zero means no cap for credit, but no repay for release.
type Posting struct {
	Ref             string          `json:"ref"`
	BalanceRef      string          `json:"balanceRef"`
	Type            PostingType     `json:"type"`
	Amount          decimal.Decimal `json:"amount"`
	DrawPolicy      DrawPolicy      `json:"drawPolicy"`
	OverdraftAmount decimal.Decimal `json:"overdraftAmount"`
}

// Transaction groups postings in their execution order.
type Transaction struct {
	ID       uuid.UUID `json:"id"`
	Postings []Posting `json:"postings"`
}

// BalanceSnapshot supplies a cache-miss seed and an initial version for CAS.
// Live settings take precedence over the seed. A snapshot in the pool need not
// be touched; overdraft companions are required only when actually needed.
type BalanceSnapshot struct {
	BalanceRef            string          `json:"balanceRef"`
	ID                    uuid.UUID       `json:"id"`
	AccountID             uuid.UUID       `json:"accountId"`
	AccountType           string          `json:"accountType"`
	AssetCode             string          `json:"assetCode"`
	Alias                 string          `json:"alias"`
	Key                   string          `json:"key"`
	Direction             string          `json:"direction"`
	BalanceScope          string          `json:"balanceScope"`
	Available             decimal.Decimal `json:"available"`
	OnHold                decimal.Decimal `json:"onHold"`
	OverdraftUsed         decimal.Decimal `json:"overdraftUsed"`
	OverdraftLimit        decimal.Decimal `json:"overdraftLimit"`
	Version               int64           `json:"version"`
	AllowSending          bool            `json:"allowSending"`
	AllowReceiving        bool            `json:"allowReceiving"`
	Blocked               bool            `json:"blocked"`
	AllowOverdraft        bool            `json:"allowOverdraft"`
	OverdraftLimitEnabled bool            `json:"overdraftLimitEnabled"`
}

// Request is one ordered execution within an authenticated tenant and ledger.
// All UUIDs must be nonzero. ExecutionID remains stable across retries of one
// action, but differs between creation, commitment and cancellation.
// Balances is the available snapshot pool, not the set of explicit input legs.
// Implementations validate references and numeric invariants before any writes,
// and compare initial versions once per physical balance actually touched.
// Storage adapters own the wire DTO, including canonical decimal strings,
// lossless versions and empty-array encoding; these types are not a Lua wire format.
type Request struct {
	OrganizationID uuid.UUID         `json:"organizationId"`
	LedgerID       uuid.UUID         `json:"ledgerId"`
	ExecutionID    uuid.UUID         `json:"executionId"`
	Transactions   []Transaction     `json:"transactions"`
	Balances       []BalanceSnapshot `json:"balances"`
}

// BalanceState contains the real monetary state at a movement boundary.
type BalanceState struct {
	Available     decimal.Decimal `json:"available"`
	OnHold        decimal.Decimal `json:"onHold"`
	OverdraftUsed decimal.Decimal `json:"overdraftUsed"`
	Version       int64           `json:"version"`
}

const (
	// RolePrimary identifies the movement directly requested by a posting.
	RolePrimary = "primary"
	// RoleOverdraftCompanion identifies its generated debt-account movement.
	RoleOverdraftCompanion = "overdraft_companion"
)

// Movement records a real change in available, on-hold or overdraft-used funds.
// Amount may be zero when only debt changes; such a movement still increments
// the balance version. Unchanged state produces no movement or version increment.
// Ref is unique and deterministic from transaction ID, posting ref, role and
// ordinal. PostingRef always identifies the originating posting, even for a
// companion. Positive OverdraftDelta is a draw; negative is repayment.
// Before and After must not be replaced by accounting-row projections.
type Movement struct {
	Ref            string          `json:"ref"`
	TransactionID  uuid.UUID       `json:"transactionId"`
	PostingRef     string          `json:"postingRef"`
	Role           string          `json:"role"`
	BalanceRef     string          `json:"balanceRef"`
	Type           PostingType     `json:"type"`
	Amount         decimal.Decimal `json:"amount"`
	OverdraftDelta decimal.Decimal `json:"overdraftDelta"`
	Before         BalanceState    `json:"before"`
	After          BalanceState    `json:"after"`
}

// Result preserves execution order in Movements and contains one Final snapshot
// per touched physical balance in deterministic order, excluding unused seeds.
type Result struct {
	Movements []Movement        `json:"movements"`
	Final     []BalanceSnapshot `json:"final"`
}

const (
	FailureInsufficientFunds         = "insufficient_funds"
	FailureOverdraftLimitExceeded    = "overdraft_limit_exceeded"
	FailureOverdraftNotEligible      = "overdraft_not_eligible"
	FailureOverdraftCompanionMissing = "overdraft_companion_missing"
	FailureStaleVersion              = "stale_version"
	FailureBalanceDeleted            = "balance_deleted"
	FailureOnHoldUnderflow           = "onhold_underflow"
	FailureBalanceMissing            = "balance_missing"
)

// Failure is a recognized refusal produced before any accounting write.
// TransactionIndex and PostingIndex are zero-based, or -1 when not applicable.
// Code must be one of the Failure constants; unknown protocol codes and
// transport, runtime or indeterminate outcomes remain technical errors.
// Missing companions and on-hold underflow indicate integrity failures, not
// ordinary insufficient funds. Public error mapping belongs to the caller.
type Failure struct {
	Code             string `json:"code"`
	TransactionIndex int    `json:"transactionIndex"`
	PostingIndex     int    `json:"postingIndex"`
	BalanceRef       string `json:"balanceRef"`
}

// Error returns the stable accounting failure code.
func (f *Failure) Error() string {
	return f.Code
}

var _ error = (*Failure)(nil)
