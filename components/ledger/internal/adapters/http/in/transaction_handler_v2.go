// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the v2 transaction terminal.
// Transport-only: decode the flat v2 body, translate it to the canonical
// Transaction, and delegate to the shared createTransactionShell funnel. The v2 create
// actions (direct, hold, ...) differ only in the pending flag Translate applies and an
// optional Operation.Type label override, so they share the createTransactionV2 helper.
//
// A v2 create names its organization and ledger on every leg of the body, and Translate
// resolves the single pair they agree on. That pair is what the funnel is entered with, so
// the create endpoints carry no organization or ledger segment: one request has one scope,
// stated once.
//
// Idempotency keys off the raw v2 body as submitted (pre-translation), but the v2 action
// is carried by the ENDPOINT, not the body — so the action identity is folded into the
// hash source (see v2IdempotencyHashSource) to keep byte-identical bodies posted to
// different actions in distinct idempotency slots. Conventions mirror the v1 Huma create
// shells (see transaction_handler_huma.go's header): the body carries RawBody so
// http.DecodeAndValidate is the sole body validator, and errors flow through the shared
// pkgHTTP.HumaProblem RFC 9457 envelope (business errors from Translate map to a 4xx with
// a green span; a malformed route UUID, organization or ledger is caught by the input's
// uuid validate tags as a clean 400).

// CreateTransactionInputV2 is the request envelope every v2 create action shares: the
// body shape is identical across them because the action intent is carried by the
// endpoint, not the body. It declares no path parameter — the organization and ledger a
// create posts against are body fields. RawBody keeps the body out of Huma's validator so
// the flat v2 model is decoded imperatively via http.DecodeAndValidate.
type CreateTransactionInputV2 struct {
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the create; an identical retry returns the original transaction"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// createTransactionV2 is the shared body of the v2 create actions. It guards the request
// context, builds the canonical Transaction and the request's scope from the flat v2 body
// (decodeAndBuildV2Transaction), delegates to command.CreateTransactionV2 keyed by the
// action-discriminated raw body (v2IdempotencyHashSource) — the /v2 contract is the one
// that includes the fee engine and the tracer reservation — and projects the result onto
// the /v2 wire shape (newTransactionV2). Translate business errors and the input's UUID
// validation surface as RFC 9457 4xx via pkgHTTP.HumaProblem.
func (handler *TransactionHandler) createTransactionV2(ctx context.Context, rawBody []byte, idempotencyKey, idempotencyTTL string, pending bool, operationTypeOverride string) (*CreateTransactionOutputV2, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	transactionInput, scope, err := decodeAndBuildV2Transaction(rawBody, pending, operationTypeOverride)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, err := parseOrgLedger(scope.OrganizationID, scope.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, replayed, err := handler.Command.CreateTransactionV2(ctx, command.CreateTransactionV2Input{
		OrganizationID:        orgID,
		LedgerID:              ledgerID,
		Transaction:           transactionInput,
		TransactionStatus:     transactionInput.InitialStatus(),
		IdempotencyKey:        idempotencyKey,
		IdempotencyTTL:        pkgHTTP.ParseIdempotencyTTL(idempotencyTTL),
		IdempotencyHashSource: v2IdempotencyHashSource(rawBody, pending, operationTypeOverride),
	})
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateTransactionOutputV2{
		Status:              http.StatusCreated,
		IdempotencyReplayed: replayedHeader(replayed),
		Body:                newTransactionV2(tran),
	}, nil
}

// decodeAndBuildV2Transaction decodes+validates the flat v2 body imperatively (the SAME
// http.DecodeAndValidate the v1 create ops run), translates it to the canonical
// Transaction with the caller's pending intent, and stamps the optional Operation.Type
// override. It returns the transaction alongside the scope Translate resolved from the
// legs, which together are exactly what createTransactionV2 hands to the funnel — so this
// is the unit seam for asserting both the translate+stamp result and the resolved scope.
func decodeAndBuildV2Transaction(rawBody []byte, pending bool, operationTypeOverride string) (mtransaction.Transaction, mtransaction.V2Scope, error) {
	payload := new(mtransaction.CreateTransactionV2Input)
	if _, err := pkgHTTP.DecodeAndValidate(rawBody, payload); err != nil {
		return mtransaction.Transaction{}, mtransaction.V2Scope{}, err
	}

	transactionInput, scope, err := payload.Translate(pending)
	if err != nil {
		return mtransaction.Transaction{}, mtransaction.V2Scope{}, err
	}

	if operationTypeOverride != "" {
		transactionInput.OperationTypeOverride = operationTypeOverride
	}

	return transactionInput, scope, nil
}

// idempotencyActionDiscriminator returns the action-identity label folded into the v2
// idempotency hash source, derived from the same (pending, operationTypeOverride) the action
// passes. An explicit Operation.Type override wins (block/unblock); else a pending create is
// HOLD; else direct carries no discriminator. Direct's empty label is deliberate so its hash
// source stays the bare body, byte-identical to the Phase 1 direct idempotency contract.
func idempotencyActionDiscriminator(pending bool, operationTypeOverride string) string {
	switch {
	case operationTypeOverride != "":
		return operationTypeOverride
	case pending:
		return "HOLD"
	default:
		return ""
	}
}

// v2IdempotencyHashSource builds the idempotency hash source for a v2 create action. The v2
// action is carried by the ENDPOINT, not the body, so two byte-identical flat bodies posted
// to /direct and /hold would otherwise share one no-key idempotency slot and cross-replay
// (a hold could return a settled direct, or vice versa). Folding the action discriminator in
// gives each action a distinct no-key identity: direct keeps the bare body; every other
// action prefixes its discriminator joined by command.IdempotencyDiscriminatorSep so the two sources
// can never collide.
func v2IdempotencyHashSource(rawBody []byte, pending bool, operationTypeOverride string) string {
	disc := idempotencyActionDiscriminator(pending, operationTypeOverride)
	if disc == "" {
		return string(rawBody)
	}

	return disc + command.IdempotencyDiscriminatorSep + string(rawBody)
}

// CreateTransactionDirectV2 creates a v2 transaction with the direct (non-pending)
// action: it delegates to createTransactionV2 with pending=false and no Operation.Type
// override, answering with the /v2 CreateTransactionOutputV2 success envelope
// (201 + X-Idempotency-Replayed).
func (handler *TransactionHandler) CreateTransactionDirectV2(ctx context.Context, in *CreateTransactionInputV2) (*CreateTransactionOutputV2, error) {
	return handler.createTransactionV2(ctx, in.RawBody, in.IdempotencyKey, in.IdempotencyTTL, false, "")
}

// CreateTransactionHoldV2 creates a v2 transaction with the hold action: it delegates
// to createTransactionV2 with pending=true so the use case opens the transaction as PENDING
// (held for later commit/cancel). It reuses the same flat input envelope and success
// envelope as the direct action.
func (handler *TransactionHandler) CreateTransactionHoldV2(ctx context.Context, in *CreateTransactionInputV2) (*CreateTransactionOutputV2, error) {
	return handler.createTransactionV2(ctx, in.RawBody, in.IdempotencyKey, in.IdempotencyTTL, true, "")
}

// CreateTransactionBlockV2 creates a v2 transaction with the block action: it delegates
// to createTransactionV2 with pending=false and the constant.BLOCK Operation.Type override.
// The override stays transaction-level: it labels the operation type and never touches
// accounting direction, value, or balance flags.
// pending=false gives InitialStatus()=CREATED, matching the v1 block action which forces
// Pending=false. The reason travels as a metadata key copied by Translate. It reuses the same
// flat input envelope and success envelope as the direct action.
func (handler *TransactionHandler) CreateTransactionBlockV2(ctx context.Context, in *CreateTransactionInputV2) (*CreateTransactionOutputV2, error) {
	return handler.createTransactionV2(ctx, in.RawBody, in.IdempotencyKey, in.IdempotencyTTL, false, constant.BLOCK)
}

// CreateTransactionUnblockV2 creates a v2 transaction with the unblock action: it delegates
// to createTransactionV2 with pending=false and the constant.UNBLOCK Operation.Type override,
// mirroring the block action's contract with the opposing override label.
func (handler *TransactionHandler) CreateTransactionUnblockV2(ctx context.Context, in *CreateTransactionInputV2) (*CreateTransactionOutputV2, error) {
	return handler.createTransactionV2(ctx, in.RawBody, in.IdempotencyKey, in.IdempotencyTTL, false, constant.UNBLOCK)
}

// --- POST /organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/{commit,cancel,revert} ---

// CommitTransactionV2 is the /v2 shell over the SAME commitTransaction core the v1
// CommitTransaction shell calls (fetch write-behind/DB, then commitOrCancelTransaction with
// APPROVED). It differs from the v1 shell only in the response envelope: the /v2 wire shape
// (TransactionV2) instead of the canonical transaction.Transaction. Returns 201.
func (handler *TransactionHandler) CommitTransactionV2(ctx context.Context, in *StateTransactionRequest) (*StateTransactionOutputV2, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, err := handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.APPROVED, command.RouteV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &StateTransactionOutputV2{Status: http.StatusCreated, Body: newTransactionV2(tran)}, nil
}

// CancelTransactionV2 is the /v2 shell over the SAME commitTransaction core the v1
// CancelTransaction shell calls (CANCELED, which runs the tracer release-by-transaction
// two-phase), differing only in the /v2 response envelope. Returns 201.
func (handler *TransactionHandler) CancelTransactionV2(ctx context.Context, in *StateTransactionRequest) (*StateTransactionOutputV2, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, err := handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.CANCELED, command.RouteV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &StateTransactionOutputV2{Status: http.StatusCreated, Body: newTransactionV2(tran)}, nil
}

// RevertTransactionV2 is the /v2 shell over the SAME revertTransaction core the v1
// RevertTransaction shell calls (parent/revert eligibility + bidirectional-route checks,
// then command.CreateRevertTransaction), differing only in the /v2 response envelope
// (CreateTransactionOutputV2 instead of CreateTransactionResponse) — a revert IS a
// create, so it carries the same 201 + X-Idempotency-Replayed shape as the v2 create actions.
func (handler *TransactionHandler) RevertTransactionV2(ctx context.Context, in *StateTransactionRequest) (*CreateTransactionOutputV2, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	orgID, ledgerID, txID, err := parseOrgLedgerTx(in)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	tran, replayed, err := handler.revertTransaction(ctx, orgID, ledgerID, txID, command.RouteV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateTransactionOutputV2{
		Status:              http.StatusCreated,
		IdempotencyReplayed: replayedHeader(replayed),
		Body:                newTransactionV2(tran),
	}, nil
}
