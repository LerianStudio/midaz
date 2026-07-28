// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the v2 transaction terminal (ADR-006 filename-suffix versioning). It
// is transport-only: the handler decodes the flat single-leg v2 body, translates it to
// the canonical Transaction, and delegates to the SAME createTransaction funnel the v1
// create ops use (via createTransactionShell). The ~480-line create orchestration and
// its idempotency/fee/reserve seams are NOT touched — the only new code is the thin
// decode → translate boundary and the direct-vs-hold action encoded by Translate's
// pending flag. Conventions mirror the v1 Huma create shells (see
// transaction_handler_huma.go's header): path params are plain strings validated by the
// ParseUUIDPathParameters Fiber middleware, the body carries RawBody + SkipValidateBody
// so http.DecodeAndValidate is the sole body validator, and errors flow through the
// shared pkgHTTP.HumaProblem RFC 9457 envelope (business errors from Translate map to a
// 4xx with a green span; a malformed route UUID is caught by the input's uuid validate
// tag as a clean 400).

// CreateTransactionDirectV2InputHuma is the v2 direct-create request envelope. The
// org/ledger path params are plain strings (validated by the ParseUUIDPathParameters
// Fiber middleware attached before this terminal). RawBody keeps the body out of Huma's
// validator so the flat v2 model is decoded imperatively via http.DecodeAndValidate.
type CreateTransactionDirectV2InputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateTransactionDirectV2Huma decodes+validates the flat v2 body imperatively (the
// SAME http.DecodeAndValidate the v1 create ops run over their input types), translates
// it to the canonical single-leg Transaction with the direct (non-pending) action, and
// delegates to the shared createTransactionShell — reusing the v1 createTransaction
// funnel and its CreateTransactionOutputHuma success envelope (201 + X-Idempotency-
// Replayed). Translate business errors and the input's route-UUID validation surface as
// RFC 9457 4xx via pkgHTTP.HumaProblem.
func (handler *TransactionHandler) CreateTransactionDirectV2Huma(ctx context.Context, in *CreateTransactionDirectV2InputHuma) (*CreateTransactionOutputHuma, error) {
	payload := new(mtransaction.CreateTransactionV2Input)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput, err := payload.Translate(false)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return handler.createTransactionShell(ctx, in.OrganizationID, in.LedgerID, transactionInput, transactionInput.InitialStatus(), in.IdempotencyKey, in.IdempotencyTTL)
}
