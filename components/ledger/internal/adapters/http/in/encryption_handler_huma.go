// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the ledger's Huma adoption of the CRM envelope-encryption
// provisioning resource (provision + status). It mirrors the asset exemplar
// (asset_handler_huma.go); see that file's header for the full conventions.
// Encryption-specific notes:
//
//  1. AUTH is appName "midaz" (crm_routes.go ApplicationName), resource
//     "encryption". The swaggo @Security on the Fiber wrappers is BearerAuth ONLY,
//     so the per-op Security metadata here is Bearer-only too — SPEC metadata only;
//     runtime auth stays the Fiber guard chain (auth.Authorize("midaz","encryption",
//     verb) + tenant + ParseUUIDPathParameters("organization")) attached BEFORE the
//     Huma terminal.
//  2. These ops are ORG-SCOPED (no ledger in the path), so the shells resolve only
//     organization_id via the shared parseOrg helper (defined in ledger_handler_huma.go).
//  3. TENANT: the provision core reads the tenant id from ctx via
//     encryption.ResolveProvisionTenantID. The Fiber tenant PostAuthMiddlewares run
//     BEFORE the Huma terminal (same chain both transports), so ctx already carries
//     the tenant id — the shell forwards ctx untouched and the core is transport-neutral.
//  4. POST carries RawBody + SkipValidateBody so http.DecodeAndValidate is the sole
//     body validator (never a native Huma 422). Errors go through pkgHTTP.HumaProblem.

// secEncryptionBearer advertises that each encryption operation accepts a JWT
// bearer token (Bearer-only, matching the Fiber swaggo @Security BearerAuth). SPEC
// metadata only; runtime auth is the Fiber guard chain.
var secEncryptionBearer = []map[string][]string{
	{"BearerAuth": {}},
}

// --- POST /encryption/provision -----------------------------------------------

// ProvisionEncryptionInputHuma is the Huma request envelope for POST. RawBody keeps
// the body out of Huma's validator (see file header); organization_id is validated
// by the Fiber middleware, not by a format tag.
type ProvisionEncryptionInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token; only required when the auth plugin is enabled"`
	RawBody        []byte `contentType:"application/json"`
}

// ProvisionEncryptionOutputHuma pins 201 (matching http.Created).
type ProvisionEncryptionOutputHuma struct {
	Status int
	Body   *mmodel.ProvisionEncryptionResponse
}

// ProvisionHuma decodes+validates the raw body imperatively then delegates to the
// shared provision core.
func (handler *EncryptionHandler) ProvisionHuma(ctx context.Context, in *ProvisionEncryptionInputHuma) (*ProvisionEncryptionOutputHuma, error) {
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

	return &ProvisionEncryptionOutputHuma{Status: http.StatusCreated, Body: response}, nil
}

// --- GET /encryption/status ---------------------------------------------------

// GetProvisioningStatusInputHuma is the status request envelope (org only).
type GetProvisioningStatusInputHuma struct {
	OrganizationID string `path:"organization_id" doc:"Organization ID (UUID)"`
	Authorization  string `header:"Authorization" doc:"Bearer token; only required when the auth plugin is enabled"`
}

// GetProvisioningStatusOutputHuma carries the status verbatim (200, matching http.OK).
type GetProvisioningStatusOutputHuma struct {
	Status int
	Body   *mmodel.ProvisioningStatusResponse
}

// GetProvisioningStatusHuma delegates to the shared getProvisioningStatus core.
func (handler *EncryptionHandler) GetProvisioningStatusHuma(ctx context.Context, in *GetProvisioningStatusInputHuma) (*GetProvisioningStatusOutputHuma, error) {
	orgID, err := parseOrg(in.OrganizationID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	response, err := handler.getProvisioningStatus(ctx, orgID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetProvisioningStatusOutputHuma{Status: http.StatusOK, Body: response}, nil
}

// RegisterEncryptionRoutes registers the two migrated encryption operations on the
// shared Huma API. It is the per-file seam the unified server calls (conditionally,
// only in envelope encryption mode — mirroring the Fiber `if eh != nil` guard in
// crm_routes.go); the auth ("midaz","encryption",verb) + tenant +
// ParseUUIDPathParameters("organization") middleware chain is attached on the /v1
// group (Fiber-level) BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE
// (see asset_handler_huma.go's RegisterAssetRoutes header for the /v1 rationale).
func RegisterEncryptionRoutes(api huma.API, h *EncryptionHandler) {
	const (
		provisionPath = "/organizations/{organization_id}/encryption/provision"
		statusPath    = "/organizations/{organization_id}/encryption/status"
		tag           = "Encryption"
	)

	huma.Register(api, huma.Operation{
		OperationID: "provisionEncryption",
		Method:      http.MethodPost,
		Path:        provisionPath,
		Summary:     "Provision an Organization for Envelope Encryption",
		Tags:        []string{tag},
		Security:    secEncryptionBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
	}, h.ProvisionHuma)

	huma.Register(api, huma.Operation{
		OperationID: "getProvisioningStatus",
		Method:      http.MethodGet,
		Path:        statusPath,
		Summary:     "Get Provisioning Status",
		Tags:        []string{tag},
		Security:    secEncryptionBearer,
	}, h.GetProvisioningStatusHuma)
}
