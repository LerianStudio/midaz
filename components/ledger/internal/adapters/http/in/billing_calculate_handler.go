// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the Huma transport of the billing-calculate resource: the response
// envelope and the shell that decodes a request, calls calculateBilling in
// billing_calculate_core.go, and renders the envelope.
//
// The shell names the ledger in its path and resolves it via parseFeeV2Path, and the
// body's own ledger must agree with the path — see requireBodyLedgerMatchesPath in
// fee_ledger_scope.go.
//
// 200 is intentional: this is a compute/RPC-style endpoint that persists nothing.
// Unlike the fee-estimate op (whose response embeds the transaction tree and forces a
// raw-[]byte escape hatch), BillingCalculateResponse is a flat Results+Summary struct
// with no time.Time-alias schema-gen landmine, so it serializes as a normal typed Body.
//
// AUTH is appName "midaz" (routes.go midazName), resource "billing-calculate". The
// Fiber guard chain — auth.Authorize("midaz","billing-calculate","post") + the
// fees-scoped tenant PostAuthMiddlewares +
// ParseUUIDPathParameters("billing-calculate") — is attached on the /v2 group BEFORE
// this terminal (see billing_calculate_routes.go), so the Security metadata is SPEC
// metadata only.

// CalculateBillingResponse carries the calculation envelope at 200.
type CalculateBillingResponse struct {
	Status int
	Body   *model.BillingCalculateResponse
}

// --- POST /ledgers/{ledger_id}/billing/calculate ---------------------------------

// CalculateBillingV2Request is the ledger-scoped calculate envelope (RawBody, see
// CreatePackageV2Request).
type CalculateBillingV2Request struct {
	FeeV2Path
	RawBody []byte `contentType:"application/json"`
}

// CalculateBillingV2 decodes+validates the raw body imperatively, refuses a body
// ledger that disagrees with the path, then delegates to the shared calculateBilling
// core.
func (handler *BillingCalculateHandler) CalculateBillingV2(ctx context.Context, in *CalculateBillingV2Request) (*CalculateBillingResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(model.BillingCalculateRequest)
	if err := decodeFeeBodyInSpan(ctx, in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	if err := requireBodyLedgerMatchesPath(payload.LedgerID, ledgerID); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	result, err := handler.calculateBilling(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CalculateBillingResponse{Status: http.StatusOK, Body: result}, nil
}
