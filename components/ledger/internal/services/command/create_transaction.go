// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/skip"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// IdempotencyDiscriminatorSep joins an action discriminator to the rest of an idempotency
// hash preimage (v2IdempotencyHashSource). A NUL byte can appear in neither an action label
// nor a JSON body, so two preimages built with it can never collide by concatenation.
const IdempotencyDiscriminatorSep = "\x00"

// resolveIdempotencyHashSource returns the string the idempotency hash is computed over:
// the non-empty override when supplied, else the canonical serialized transaction. Keying
// off a raw pre-translation body via the override is the v2 idempotency contract.
func resolveIdempotencyHashSource(transactionInput mtransaction.Transaction, override ...string) (string, error) {
	if len(override) > 0 && override[0] != "" {
		return override[0], nil
	}

	return libCommons.StructToJSONString(transactionInput)
}

// resolveTransactionSkips resolves the two per-call control skips (fees, tracer)
// off the already-read ledger settings, with no extra I/O. Each skip is honored
// only when the request asks for it AND the ledger opts in via its override; a
// skip requested without the matching opt-in returns the 422 business error plus
// the log/span label naming the rejected control, so the caller emits a single
// error branch for both controls.
func resolveTransactionSkips(input mtransaction.Transaction, settings mmodel.LedgerSettings) (feeSkip, tracerSkip bool, rejectLabel string, err error) {
	feeSkip, err = skip.ResolveSkipFor("fees", input.Skip != nil && input.Skip.Fees, settings.Overrides.AllowFeeSkip)
	if err != nil {
		return false, false, "Fee skip not permitted", err
	}

	tracerSkip, err = skip.ResolveSkipFor("tracer", input.Skip != nil && input.Skip.Tracer, settings.Overrides.AllowTracerSkip)
	if err != nil {
		return false, false, "Tracer skip not permitted", err
	}

	return feeSkip, tracerSkip, "", nil
}
