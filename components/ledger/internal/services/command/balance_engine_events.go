// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"sync"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// BalanceEngineEventPublisher dispatches the existing best-effort transaction,
// overdraft, and balance-change emitters after durable finalization.
type BalanceEngineEventPublisher interface {
	PublishBalanceEngineEvents(ctx context.Context, tran *transaction.Transaction, phase string)
}

var _ BalanceEngineEventPublisher = (*UseCase)(nil)

// PublishBalanceEngineEvents delegates to the same best-effort event dispatch
// used by the legacy transaction persistence path.
func (uc *UseCase) PublishBalanceEngineEvents(ctx context.Context, tran *transaction.Transaction, phase string) {
	uc.dispatchTransactionEvents(ctx, tran, phase)
}

func (uc *UseCase) dispatchTransactionEvents(ctx context.Context, tran *transaction.Transaction, phase string) {
	// Send events asynchronously with context that preserves trace but survives parent cancellation.
	// Each emitter gets its own timeout budget so a slow earlier emitter cannot starve later ones.
	go func() {
		base := context.WithoutCancel(ctx)

		runWithTimeout := func(fn func(context.Context)) {
			emitCtx, cancel := context.WithTimeout(base, asyncOperationTimeout)
			defer cancel()

			fn(emitCtx)
		}

		var wg sync.WaitGroup

		wg.Add(3)

		go func() {
			defer wg.Done()

			runWithTimeout(func(c context.Context) { uc.SendTransactionEvents(c, tran, phase) })
		}()
		go func() { defer wg.Done(); runWithTimeout(func(c context.Context) { uc.SendOverdraftEvents(c, tran) }) }()
		go func() {
			defer wg.Done()

			runWithTimeout(func(c context.Context) { uc.SendBalanceChangedEvents(c, tran) })
		}()

		wg.Wait()
	}()
}
