// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file holds the /v2 account shells. They reuse the request envelopes and the
// transport-agnostic cores of the /v1 shells in account_handler.go and differ in
// exactly two ways, both of which are the holder seam:
//
//  1. CREATE passes command.HolderOnV2, so the requireHolder gate, the two-key
//     per-call skip, and the self-holder default all apply.
//  2. Every account-bearing response carries the canonical mmodel.Account, holderId
//     and holderCheckSkipped included, instead of the AccountV1 projection.
//
// The two contracts share one Huma document and one component registry, so the v2
// response types are named apart from their v1 twins — a schema-name collision
// between distinct wire shapes is a deliberate boot panic in mapRegistry.Schema.
//
// The three ops with no account in the body — DELETE (bodiless 204) and the HEAD
// count (headers only) — have no holder surface, so they have no v2 twin here and
// both version groups register the same handler method.

// --- POST /accounts -----------------------------------------------------------

// CreateAccountV2Response pins 201 and carries the canonical account.
type CreateAccountV2Response struct {
	Status int
	Body   *mmodel.Account
}

// CreateAccountV2 decodes+validates the raw body imperatively then delegates to the
// shared createAccount core under command.HolderOnV2.
func (handler *AccountHandler) CreateAccountV2(ctx context.Context, in *CreateAccountRequest) (*CreateAccountV2Response, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.CreateAccountInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.createAccount(ctx, orgID, ledgerID, payload, in.Authorization, command.HolderOnV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateAccountV2Response{Status: http.StatusCreated, Body: account}, nil
}

// --- GET /accounts (list) -----------------------------------------------------

// ListAccountsV2Response carries the pagination envelope verbatim: the items keep
// the canonical account shape.
type ListAccountsV2Response struct {
	Status int
	Body   pkgHTTP.Pagination
}

// ListAccountsV2 binds the query imperatively then delegates to getAllAccounts.
func (handler *AccountHandler) ListAccountsV2(ctx context.Context, in *ListAccountsRequest) (*ListAccountsV2Response, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	pagination, err := handler.getAllAccounts(ctx, orgID, ledgerID, in.queries(), command.HolderOnV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListAccountsV2Response{Status: http.StatusOK, Body: pagination}, nil
}

// --- GET /accounts/{id} + the two by-key reads --------------------------------

// GetAccountV2Response carries the canonical account.
type GetAccountV2Response struct {
	Status int
	Body   *mmodel.Account
}

// GetAccountByIDV2 delegates to getAccountByID.
func (handler *AccountHandler) GetAccountByIDV2(ctx context.Context, in *GetAccountRequest) (*GetAccountV2Response, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.getAccountByID(ctx, orgID, ledgerID, id, command.HolderOnV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountV2Response{Status: http.StatusOK, Body: account}, nil
}

// GetAccountByAliasV2 delegates to the shared getAccountByAlias core.
func (handler *AccountHandler) GetAccountByAliasV2(ctx context.Context, in *GetAccountByAliasRequest) (*GetAccountV2Response, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.getAccountByAlias(ctx, "handler.get_account_by_alias", orgID, ledgerID, in.Alias, command.HolderOnV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountV2Response{Status: http.StatusOK, Body: account}, nil
}

// GetAccountExternalByCodeV2 resolves the external alias then delegates to the
// shared getAccountByAlias core.
func (handler *AccountHandler) GetAccountExternalByCodeV2(ctx context.Context, in *GetAccountExternalByCodeRequest) (*GetAccountV2Response, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	alias := constant.DefaultExternalAccountAliasPrefix + in.Code

	account, err := handler.getAccountByAlias(ctx, "handler.get_account_external_by_code", orgID, ledgerID, alias, command.HolderOnV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetAccountV2Response{Status: http.StatusOK, Body: account}, nil
}

// --- PATCH /accounts/{id} -----------------------------------------------------

// UpdateAccountV2Response carries the updated account (200, matching http.OK).
type UpdateAccountV2Response struct {
	Status int
	Body   *mmodel.Account
}

// UpdateAccountV2 decodes+validates the raw body imperatively then delegates to the
// shared updateAccount core.
func (handler *AccountHandler) UpdateAccountV2(ctx context.Context, in *UpdateAccountRequest) (*UpdateAccountV2Response, error) {
	orgID, ledgerID, err := parseOrgLedger(in.OrganizationID, in.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	id, err := parsePathUUID(in.ID, "id")
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.UpdateAccountInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	account, err := handler.updateAccount(ctx, orgID, ledgerID, id, payload, command.HolderOnV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &UpdateAccountV2Response{Status: http.StatusOK, Body: account}, nil
}
