// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// defaultDispatchTimeout is the default timeout for fire-and-forget
// decision lifecycle event dispatches.
const defaultDispatchTimeout = 3 * time.Second

func (uc *UseCase) dispatchDecisionLifecycleEvent(
	ctx context.Context,
	tran *transaction.Transaction,
	decisionContract pkgTransaction.DecisionContract,
	action pkgTransaction.DecisionLifecycleAction,
	timeout time.Duration,
) {
	if tran == nil {
		return
	}

	if timeout <= 0 {
		timeout = defaultDispatchTimeout
	}

	tranCopy := *tran

	publish := func() {
		sideEffectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()

		uc.SendDecisionLifecycleEvent(sideEffectCtx, &tranCopy, decisionContract, action)
	}

	if uc != nil && uc.DecisionLifecycleSyncForTests {
		publish()
		return
	}

	go publish()
}
