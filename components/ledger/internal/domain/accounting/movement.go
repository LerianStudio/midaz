// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounting

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	// RolePrimary identifies the movement directly requested by a posting.
	RolePrimary = "primary"
	// RoleOverdraftCompanion identifies its generated debt-account movement.
	RoleOverdraftCompanion = "overdraft_companion"
	// RoleFeeDebtDebit is a collect posting's one movement on its debtor (ordinal 0).
	RoleFeeDebtDebit = "fee_debt_debit"
	// RoleFeeDebtCredit is a collect posting's movement on one settled debt's creditor;
	// its ordinal is the debt's index in the posting's Items.
	RoleFeeDebtCredit = "fee_debt_credit"
)

// FeeDebtChangeKind names how a change moved a debt's remaining amount.
type FeeDebtChangeKind string

const (
	FeeDebtOpened   FeeDebtChangeKind = "opened"
	FeeDebtSettled  FeeDebtChangeKind = "settled"
	FeeDebtCanceled FeeDebtChangeKind = "canceled"
	FeeDebtReopened FeeDebtChangeKind = "reopened"
)

// FeeDebtChange is one applied change to one debt, in the debtor's (transaction)
// scope. Amount is positive; remaining = opened - settled - canceled + reopened.
// PostingRef is empty for canceled and reopened changes.
type FeeDebtChange struct {
	TransactionID       uuid.UUID         `json:"transactionId"`
	PostingRef          string            `json:"postingRef"`
	Kind                FeeDebtChangeKind `json:"kind"`
	DebtID              string            `json:"debtId"`
	DebtorRef           string            `json:"debtorRef"`
	CreditRef           string            `json:"creditRef"`
	OriginTransactionID uuid.UUID         `json:"originTransactionId"`
	Seq                 int64             `json:"seq,string"`
	AssetCode           string            `json:"assetCode"`
	Amount              decimal.Decimal   `json:"amount"`
}

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

// ExecutionResult preserves execution order in Movements and contains one
// final snapshot per touched physical balance in deterministic order, excluding
// unused seeds.
type ExecutionResult struct {
	Movements          []Movement        `json:"movements"`
	Final              []BalanceSnapshot `json:"final"`
	AppliedAtUnixMicro int64             `json:"appliedAtUnixMicro,omitempty"`
	// FeeDebt lists fee-debt changes in execution order; absent when there are none.
	FeeDebt []FeeDebtChange `json:"feeDebt,omitempty"`
}
