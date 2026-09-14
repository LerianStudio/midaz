// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// ErrTransactionCompletionConflict means the persisted state cannot be
// reconciled with the frozen execution without replacing existing information.
var ErrTransactionCompletionConflict = errors.New("transaction completion conflict")

// TransactionWriteSet carries already projected, deterministic rows.
// ExpectedStatus is empty for creation and PENDING for commit or cancellation.
type TransactionWriteSet struct {
	Transaction    *transaction.Transaction
	Action         string
	ExpectedStatus string
}

// TransactionPersistenceOutcome reports the transaction status confirmed by the
// durable SQL store. It is not derived from the completion plan.
type TransactionPersistenceOutcome struct {
	TransactionStatus string
	// LifecyclePhase identifies creation, transition, or a verified replay in
	// this committed SQL attempt. It does not confirm event publication.
	LifecyclePhase string
}

// TransactionCompletionResult returns the durable outcome together with a
// caller-owned copy of the exact transaction and operation rows that were
// projected and persisted.
type TransactionCompletionResult struct {
	Record  TransactionWriteSet
	Outcome TransactionPersistenceOutcome
}

// TransactionWriteStore confirms only durable SQL transaction and operation
// persistence. Success does not confirm Mongo metadata or authorize backup removal.
type TransactionWriteStore interface {
	Persist(context.Context, TransactionWriteSet) error
}

// TransactionWriteStoreWithOutcome is the additive recovery-store
// capability used when callers need the status actually observed in durable
// SQL. PersistWithOutcome returns successfully only after the SQL transaction
// commits.
type TransactionWriteStoreWithOutcome interface {
	PersistWithOutcome(context.Context, TransactionWriteSet) (TransactionPersistenceOutcome, error)
}
