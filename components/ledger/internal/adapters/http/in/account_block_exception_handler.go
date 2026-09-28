// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the Huma transport of the account block-exception resource. It
// follows the account-type exemplar (account_type_handler.go): RawBody keeps the
// body out of Huma's validator so the canonical Midaz envelope owns every
// rejection, and the org+ledger path params are validated by the Fiber
// ParseUUIDPathParameters middleware rather than by a format tag.
//
// AUTH NOTE: the resource authorizes under the "midaz" appName with its OWN
// resource name — the (midaz, account-block-exceptions, post) tuple is attached
// by the Fiber guard chain in registerAccountBlockExceptionRoutesToApp. The
// Security metadata below is SPEC-ONLY.

// secAccountBlockExceptionBearer advertises that the op accepts a JWT bearer
// token. SPEC metadata only; runtime auth is the Fiber guard chain. The scheme
// name is declared once on the shared Huma API.
var secAccountBlockExceptionBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /accounts/block-exceptions ------------------------------------------

// CreateAccountBlockExceptionsRequest is the Huma request envelope for POST.
// Tenancy is NOT part of it: the organization and ledger come from the path and
// the tenant from the validated JWT the guard chain resolves, never from the
// payload.
type CreateAccountBlockExceptionsRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	LedgerID       string `path:"ledger_id" doc:"Ledger ID (UUID)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAccountBlockExceptionsResponse pins 201 (matching http.Created).
type CreateAccountBlockExceptionsResponse struct {
	Status int
	Body   *mmodel.AccountBlockExceptions
}

// CreateAccountBlockExceptions decodes+validates the raw body imperatively then
// delegates to the shared createAccountBlockExceptions core. The batch-size cap
// and the per-item amount/ttl/alias rules live in the command, so a rejection
// carries the offending item's index rather than a generic schema complaint.
func (handler *AccountBlockExceptionHandler) CreateAccountBlockExceptions(ctx context.Context, in *CreateAccountBlockExceptionsRequest) (*CreateAccountBlockExceptionsResponse, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateAccountBlockExceptionsInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	exceptions, err := handler.createAccountBlockExceptions(ctx, orgID, ledgerID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateAccountBlockExceptionsResponse{Status: http.StatusCreated, Body: exceptions}, nil
}
