// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// CompletionPlanRecord carries the completion plan for one applied transaction.
type CompletionPlanRecord struct {
	TransactionID uuid.UUID
	Payload       json.RawMessage
}

// ExecutionGuard fences competing state transitions of the same transaction.
type ExecutionGuard struct {
	TransactionID uuid.UUID
	ExpectedToken string
	NextToken     string
}

// EngineExecution combines balance changes with their completion plans and replay context.
type EngineExecution struct {
	Execution         accounting.Execution
	IntentFingerprint string
	// RetentionSeconds is the effective request idempotency/replay window. Zero
	// means the HTTP default; values above the HTTP maximum are rejected.
	// The cleanup deadline starts only after durable terminal completion, never
	// at EVAL. The recovery consumer removes protection only after that deadline.
	RetentionSeconds int64
	Guards           []ExecutionGuard
	CompletionPlans  []CompletionPlanRecord
}

// BalanceEngine is the ledger's accounting mutation boundary. Execute validates
// live balances and applies postings atomically with the evidence required to
// finish durable transaction projection. A successful result is the point of no
// return: its accounting effect is permanent and may only be counteracted by a
// later explicit transaction such as a revert. Callers must not implicitly retry
// Execute after an error because the outcome may be indeterminate.
type BalanceEngine interface {
	Execute(ctx context.Context, input EngineExecution) (*accounting.ExecutionResult, error)
}

// BalanceEngineGuardBootstrapper conditionally seeds the execution guard for a
// persisted legacy transaction that predates balance-engine guards.
type BalanceEngineGuardBootstrapper interface {
	EnsureTransactionGuard(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, nextToken string) error
}
