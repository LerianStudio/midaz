// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package accounting defines the ledger's replaceable accounting-engine
// contract. An engine implementation owns atomic live-balance validation and
// mutation. It does not own transaction composition, durable row projection, or
// storage-specific recovery orchestration.
package accounting

import "github.com/google/uuid"

// Transaction groups postings in their execution order.
type Transaction struct {
	ID                  uuid.UUID            `json:"id"`
	BalanceRequirements []BalanceRequirement `json:"balanceRequirements"`
	Postings            []Posting            `json:"postings"`
}

// Execution is one ordered accounting operation within an authenticated ledger
// scope. All UUIDs must be nonzero. ExecutionID identifies one execution of an
// action and differs between creation, commitment and cancellation.
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
