// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import "context"

// TransactionCompleter durably projects an accounting result already applied by
// BalanceEngine and reports the transaction status confirmed by SQL. Completion
// and recovery must never invoke BalanceEngine or mutate balances again.
type TransactionCompleter interface {
	Complete(context.Context, *TransactionCompletionRecord) (TransactionCompletionResult, error)
}
