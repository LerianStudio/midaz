// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file holds the DEDICATED account block/unblock transport: two POST terminals
// per version group that flip an account's block state and answer with the updated
// account in the same shape the GET/PATCH ops use.
//
// Three things make this surface its own file rather than two more shells in
// account_handler.go:
//
//  1. It has an authz resource of its OWN — ("account-blocks","post") governs both
//     directions — so the routes that carry it are worth reading in one place next to
//     the handlers they guard. One permission covers block and unblock deliberately:
//     an operator who can freeze an account must be able to release it.
//  2. It takes NO body. The reason a block happened is the integrator's to record, in
//     the account's metadata via PATCH; encoding it here would mint a second, weaker
//     write path into the same account row.
//  3. Its state transition is a convergence sequence in the command layer (account
//     row -> balance projection -> cache eviction -> audit event), so the shells stay
//     as thin as every other account shell and add nothing but path parsing.
//
// The block ops are NOT holds of funds. /transactions/block and /transactions/unblock
// are a distinct, pre-existing concept on the transaction surface and are untouched.
//
// The /v1 shells pass command.HolderOffV1 and project onto AccountV1 (holderId +
// holderCheckSkipped withheld); the /v2 shells pass command.HolderOnV2 and carry the
// canonical mmodel.Account. That is the same holder split every account-bearing op
// carries — block/unblock reuse the projection rather than minting a body shape of
// their own.

// --- POST /accounts/{id}/block and /accounts/{id}/unblock ----------------------

// AccountBlockRequest is the request envelope shared by both directions on both
// contracts. There is no body: the four ops differ only in the state they request and
// in the holder policy their contract carries. The path params carry no format tag —
// ParseUUIDPathParameters, attached as Fiber middleware by attachAccountRouteChain, is
// the sole UUID validator (see account_handler.go).
type AccountBlockRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	ID             string `path:"id" doc:"Account ID (UUID)"`
}

// AccountBlockResponse carries the updated account as the /v1 projection (200: the
// endpoint performs an action on an existing account, it creates nothing).
type AccountBlockResponse struct {
	Status int
	Body   *AccountV1
}

// AccountBlockV2Response carries the updated account as the canonical /v2 body.
type AccountBlockV2Response struct {
	Status int
	Body   *mmodel.Account
}

// BlockAccount blocks an account on the /v1 contract.
func (handler *AccountHandler) BlockAccount(ctx context.Context, in *AccountBlockRequest) (*AccountBlockResponse, error) {
	account, err := handler.changeAccountBlockStateFromRequest(ctx, in, true, command.HolderOffV1)
	if err != nil {
		return nil, err
	}

	return &AccountBlockResponse{Status: http.StatusOK, Body: newAccountV1(account)}, nil
}

// UnblockAccount releases an account on the /v1 contract.
func (handler *AccountHandler) UnblockAccount(ctx context.Context, in *AccountBlockRequest) (*AccountBlockResponse, error) {
	account, err := handler.changeAccountBlockStateFromRequest(ctx, in, false, command.HolderOffV1)
	if err != nil {
		return nil, err
	}

	return &AccountBlockResponse{Status: http.StatusOK, Body: newAccountV1(account)}, nil
}

// BlockAccountV2 blocks an account on the /v2 contract.
func (handler *AccountHandler) BlockAccountV2(ctx context.Context, in *AccountBlockRequest) (*AccountBlockV2Response, error) {
	account, err := handler.changeAccountBlockStateFromRequest(ctx, in, true, command.HolderOnV2)
	if err != nil {
		return nil, err
	}

	return &AccountBlockV2Response{Status: http.StatusOK, Body: account}, nil
}

// UnblockAccountV2 releases an account on the /v2 contract.
func (handler *AccountHandler) UnblockAccountV2(ctx context.Context, in *AccountBlockRequest) (*AccountBlockV2Response, error) {
	account, err := handler.changeAccountBlockStateFromRequest(ctx, in, false, command.HolderOnV2)
	if err != nil {
		return nil, err
	}

	return &AccountBlockV2Response{Status: http.StatusOK, Body: account}, nil
}

// changeAccountBlockStateFromRequest pulls the three path params out of the request
// envelope and hands primitives to the transport-agnostic core, so all four shells
// above reduce to "pick a direction, pick a holder policy, project the result".
// Every canonical Midaz error it returns is already wrapped by HumaProblem, which
// fixes the code and the HTTP status (0052 -> 404, 0074 -> 403).
func (handler *AccountHandler) changeAccountBlockStateFromRequest(ctx context.Context, in *AccountBlockRequest, blocked bool, holderPolicy command.RouteHolderPolicy) (*mmodel.Account, error) {
	organizationID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.changeAccountBlockState(ctx, organizationID, ledgerID, id, blocked, holderPolicy)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return account, nil
}

// changeAccountBlockState is the transport-agnostic core: it owns the handler span and
// the service call, taking primitives only (see the cores in account_core.go).
//
// The two directions share one core because they are one state transition with one
// authz resource; the command layer likewise funnels both into a single sequence. The
// requested state is recorded as a span attribute so the two directions stay
// distinguishable in traces without a second span name per direction.
func (handler *AccountHandler) changeAccountBlockState(ctx context.Context, organizationID, ledgerID, id uuid.UUID, blocked bool, holderPolicy command.RouteHolderPolicy) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	spanName := "handler.unblock_account"
	if blocked {
		spanName = "handler.block_account"
	}

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.account_id", id.String()),
		attribute.Bool("app.request.blocked", blocked),
	)

	change := handler.Command.UnblockAccount
	if blocked {
		change = handler.Command.BlockAccount
	}

	account, err := change(ctx, organizationID, ledgerID, id, holderPolicy)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to change Account block state on command", err)

		return nil, err
	}

	return account, nil
}
