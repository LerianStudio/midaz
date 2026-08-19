// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import "context"

type transactionEconomicContextKey struct{}

// WithTransactionEconomicContext carries the caller's immutable parent and
// lifecycle status through the existing Redis repository boundary. Keeping it
// request-scoped avoids a second, weaker cleanup API and preserves all callers
// on the same economic preflight command.
func WithTransactionEconomicContext(ctx context.Context, proof TransactionEconomicContext) context.Context {
	return context.WithValue(ctx, transactionEconomicContextKey{}, proof)
}

// TransactionEconomicContextFromContext returns the caller proof. Every
// enrichment and terminal handoff requires this context; there is no weaker
// legacy write path that may omit parent, lifecycle status, or action.
func TransactionEconomicContextFromContext(ctx context.Context) (TransactionEconomicContext, bool) {
	proof, ok := ctx.Value(transactionEconomicContextKey{}).(TransactionEconomicContext)

	return proof, ok
}
