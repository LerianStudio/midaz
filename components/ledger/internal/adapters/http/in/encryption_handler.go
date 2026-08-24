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

// This file is the ledger's Huma adoption of the CRM envelope-encryption
// provisioning resource (provision + status). It mirrors the asset exemplar
// (asset_handler.go); see that file's header for the full conventions.
// Encryption-specific notes:
//
//  1. AUTH is appName "midaz" (routes.go midazName), resource
//     "encryption". The Fiber guard chain is Bearer-only, so the per-op Security
//     metadata here is Bearer-only too — SPEC metadata only;
//     runtime auth stays the Fiber guard chain (auth.Authorize("midaz","encryption",
//     verb) + tenant + ParseUUIDPathParameters("organization")) attached BEFORE the
//     Huma terminal.
//  2. These ops are ORG-SCOPED (no ledger in the path), so the shells resolve only
//     organization_id via the shared parseOrg helper (defined in ledger_handler.go).
//  3. TENANT: the provision core reads the tenant id from ctx via
//     encryption.ResolveProvisionTenantID. The Fiber tenant PostAuthMiddlewares run
//     BEFORE the Huma terminal, so ctx already carries the tenant id — the shell
//     forwards ctx untouched and the core is transport-neutral.
//  4. POST carries RawBody + SkipValidateBody so http.DecodeAndValidate is the sole
//     body validator (never a native Huma 422). Errors go through pkgHTTP.HumaProblem.

// secEncryptionBearer advertises that each encryption operation accepts a JWT
// bearer token (Bearer-only, matching the Fiber guard chain). SPEC
// metadata only; runtime auth is the Fiber guard chain.
var secEncryptionBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /encryption/provision -----------------------------------------------

// ProvisionEncryptionRequest is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator (see file header); organization_id is validated
// by the Fiber middleware, not by a format tag.
type ProvisionEncryptionRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token; only required when the auth plugin is enabled"`
	RawBody        []byte `contentType:"application/json"`
}

// ProvisionEncryptionResponse pins 201 (matching http.Created).
type ProvisionEncryptionResponse struct {
	Status int
	Body   *mmodel.ProvisionEncryptionResponse
}

// Provision decodes+validates the raw body imperatively then delegates to the
// shared provision core.
func (handler *EncryptionHandler) Provision(ctx context.Context, in *ProvisionEncryptionRequest) (*ProvisionEncryptionResponse, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	payload := new(mmodel.ProvisionEncryptionInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	response, err := handler.provision(ctx, orgID, payload)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ProvisionEncryptionResponse{Status: http.StatusCreated, Body: response}, nil
}

// --- GET /encryption/status ---------------------------------------------------

// GetProvisioningStatusRequest is the status request envelope (org only).
type GetProvisioningStatusRequest struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token; only required when the auth plugin is enabled"`
}

// GetProvisioningStatusResponse carries the status verbatim (200, matching http.OK).
type GetProvisioningStatusResponse struct {
	Status int
	Body   *mmodel.ProvisioningStatusResponse
}

// GetProvisioningStatus delegates to the shared getProvisioningStatus core.
func (handler *EncryptionHandler) GetProvisioningStatus(ctx context.Context, in *GetProvisioningStatusRequest) (*GetProvisioningStatusResponse, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	response, err := handler.getProvisioningStatus(ctx, orgID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetProvisioningStatusResponse{Status: http.StatusOK, Body: response}, nil
}
