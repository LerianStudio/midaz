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

// BalanceEngineRecoveryStore confirms only durable SQL transaction and operation
// persistence. Success does not confirm Mongo metadata or authorize backup removal.
type BalanceEngineRecoveryStore interface {
	Persist(context.Context, BalanceEnginePersistenceRecord) error
}
