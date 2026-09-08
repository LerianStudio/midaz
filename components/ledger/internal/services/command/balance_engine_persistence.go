// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// ErrBalanceEnginePersistenceConflict means the persisted state cannot be
// reconciled with the frozen execution without replacing existing information.
var ErrBalanceEnginePersistenceConflict = errors.New("balance engine persistence conflict")

// BalanceEnginePersistenceRecord carries already projected, deterministic rows.
// ExpectedStatus is empty for creation and PENDING for commit or cancellation.
type BalanceEnginePersistenceRecord struct {
	Transaction    *transaction.Transaction
	Action         string
	ExpectedStatus string
}

// BalanceEngineRecoveryOutcome reports the transaction status confirmed by the
// durable SQL store. It is not derived from the frozen recovery payload.
type BalanceEngineRecoveryOutcome struct {
	TransactionStatus string
	// LifecyclePhase identifies creation, transition, or a verified replay in
	// this committed SQL attempt. It does not confirm event publication.
	LifecyclePhase string
}

// BalanceEngineRecoveryStore confirms only durable SQL transaction and operation
// persistence. Success does not confirm Mongo metadata or authorize backup removal.
type BalanceEngineRecoveryStore interface {
	Persist(context.Context, BalanceEnginePersistenceRecord) error
}

// BalanceEngineRecoveryStoreWithOutcome is the additive recovery-store
// capability used when callers need the status actually observed in durable
// SQL. PersistWithOutcome returns successfully only after the SQL transaction
// commits.
type BalanceEngineRecoveryStoreWithOutcome interface {
	PersistWithOutcome(context.Context, BalanceEnginePersistenceRecord) (BalanceEngineRecoveryOutcome, error)
}
