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

// ExecutionResult preserves execution order in Movements and contains one
// final snapshot per touched physical balance in deterministic order, excluding
// unused seeds.
type ExecutionResult struct {
	Movements []Movement        `json:"movements"`
	Final     []BalanceSnapshot `json:"final"`
}
