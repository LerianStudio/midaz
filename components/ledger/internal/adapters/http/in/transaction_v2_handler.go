// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the v2 transaction terminal.
// Transport-only: decode the flat single-leg v2 body, translate it to the canonical
// Transaction, and delegate to the shared createTransactionShell funnel. The v2 create
// actions (direct, hold, ...) differ only in the pending flag Translate applies and an
// optional Operation.Type label override, so they share the createTransactionV2 helper.
// Idempotency keys off the raw v2 body as submitted (pre-translation), passed to the
// funnel as the hash-source override so identical v2 submissions dedup by the body the
// client actually sent. Conventions mirror the v1 Huma create shells (see
// transaction_handler_huma.go's header): path params are plain strings validated by the
// ParseUUIDPathParameters Fiber middleware, the body carries RawBody so
// http.DecodeAndValidate is the sole body validator, and errors flow through the shared
// pkgHTTP.HumaProblem RFC 9457 envelope (business errors from Translate map to a 4xx with
// a green span; a malformed route UUID is caught by the input's uuid validate tag as a
// clean 400).

// CreateTransactionDirectV2InputHuma is the v2 create request envelope shared by the
// direct and hold actions (identical flat single-leg shape; the action intent is carried
// by the endpoint, not the body). The org/ledger path params are plain strings (validated
// by the ParseUUIDPathParameters Fiber middleware attached before this terminal). RawBody
// keeps the body out of Huma's validator so the flat v2 model is decoded imperatively via
// http.DecodeAndValidate.
type CreateTransactionDirectV2InputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// createTransactionV2 is the shared body of the v2 create actions. It guards the request
// context, decodes+validates the flat v2 body imperatively (the SAME http.DecodeAndValidate
// the v1 create ops run), translates it to the canonical single-leg Transaction with the
// caller's pending intent, stamps the optional Operation.Type override, and delegates to
// the shared createTransactionShell — always passing the raw v2 body as the idempotency
// hash source so every action dedups by the body as submitted. Translate business errors
// and the input's route-UUID validation surface as RFC 9457 4xx via pkgHTTP.HumaProblem.
func (handler *TransactionHandler) createTransactionV2(ctx context.Context, orgStr, ledgerStr string, rawBody []byte, idempotencyKey, idempotencyTTL string, pending bool, operationTypeOverride string) (*CreateTransactionOutputHuma, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mtransaction.CreateTransactionV2Input)
	if _, err := pkgHTTP.DecodeAndValidate(rawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput, err := payload.Translate(pending)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if operationTypeOverride != "" {
		transactionInput.OperationTypeOverride = operationTypeOverride
	}

	return handler.createTransactionShell(ctx, orgStr, ledgerStr, transactionInput, transactionInput.InitialStatus(), idempotencyKey, idempotencyTTL, string(rawBody))
}

// CreateTransactionDirectV2Huma creates a v2 transaction with the direct (non-pending)
// action: it delegates to createTransactionV2 with pending=false and no Operation.Type
// override, reusing the v1 createTransaction funnel and its CreateTransactionOutputHuma
// success envelope (201 + X-Idempotency-Replayed).
func (handler *TransactionHandler) CreateTransactionDirectV2Huma(ctx context.Context, in *CreateTransactionDirectV2InputHuma) (*CreateTransactionOutputHuma, error) {
	return handler.createTransactionV2(ctx, in.OrganizationID, in.LedgerID, in.RawBody, in.IdempotencyKey, in.IdempotencyTTL, false, "")
}

// CreateTransactionHoldV2Huma creates a v2 transaction with the hold action: it delegates
// to createTransactionV2 with pending=true so the funnel opens the transaction as PENDING
// (held for later commit/cancel). It reuses the same flat input envelope and success
// envelope as the direct action.
func (handler *TransactionHandler) CreateTransactionHoldV2Huma(ctx context.Context, in *CreateTransactionDirectV2InputHuma) (*CreateTransactionOutputHuma, error) {
	return handler.createTransactionV2(ctx, in.OrganizationID, in.LedgerID, in.RawBody, in.IdempotencyKey, in.IdempotencyTTL, true, "")
}
