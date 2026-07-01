// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// This file migrates the single ValidationHandler operation (Validate) to Huma,
// following the reference pattern established in rule_handler_huma.go (read that
// file's header for the full rationale). The conventions carried verbatim:
//
//   - The body is taken as RawBody []byte + SkipValidateBody:true so malformed
//     JSON and NormalizeAndValidate failures flow through the core's imperative
//     json.Unmarshal + NormalizeAndValidate and produce the canonical Midaz error
//     — same code/status as the Fiber path, never a native Huma 422. The
//     payload-size guard (>100KB) also stays imperative in the core (Huma has no
//     Fiber-style body limit).
//   - The handler func delegates to the transport-agnostic core validate() on
//     *ValidationHandler; the core returns the full *services.ValidateResult so
//     this shell can pick the success status.
//   - Errors flow through the package-level humaProblem (defined in
//     rule_handler_huma.go) — reused verbatim, NOT redefined here.
//
// DUAL-STATUS (DD-9): a NEW validation returns 201, a DUPLICATE (idempotent)
// returns 200. ValidateOutputHuma carries a Status int the shell sets from
// result.IsDuplicate; Huma's runtime writes that value, overriding DefaultStatus
// (huma.go: `if outStatusIndex != -1 { status = out.Status }`). Because a Body
// field is present, Huma auto-registers the 200 response in the spec; the 201 is
// declared explicitly in the Operation's Responses so both are advertised, with
// NO fixed DefaultStatus forcing one.

// ValidateInputHuma is the Huma request envelope for POST /v1/validations. The
// body is taken raw (see file header) so the core's imperative json.Unmarshal +
// NormalizeAndValidate remain the sole validators.
type ValidateInputHuma struct {
	RawBody []byte `contentType:"application/json"`
}

// ValidateOutputHuma is the Huma response envelope for POST /v1/validations. Body
// is the ValidationResponse serialized verbatim; Status is set by the shell to
// 200 (duplicate) or 201 (new) from result.IsDuplicate, and Huma's runtime emits
// that value rather than DefaultStatus.
type ValidateOutputHuma struct {
	Status int
	Body   *model.ValidationResponse
}

// ValidateHuma is the Huma handler for POST /v1/validations. It delegates to the
// shared core and maps result.IsDuplicate to the DD-9 dual status: 200 for a
// duplicate (idempotent) request, 201 for a new one.
func (h *ValidationHandler) ValidateHuma(ctx context.Context, in *ValidateInputHuma) (*ValidateOutputHuma, error) {
	result, err := h.validate(ctx, in.RawBody)
	if err != nil {
		return nil, humaProblem(err)
	}

	status := http.StatusCreated
	if result.IsDuplicate {
		status = http.StatusOK
	}

	return &ValidateOutputHuma{Status: status, Body: result.Response}, nil
}

// RegisterValidationRoutes registers the migrated validate operation on the shared
// Huma API. It is the per-file seam NewRoutes calls; the auth middleware for this
// route is attached in routes.go (Fiber-level), not here. Path is GROUP-RELATIVE
// to the /v1 Fiber group.
func RegisterValidationRoutes(api huma.API, h *ValidationHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "validateTransaction",
		Method:      http.MethodPost,
		Path:        "/validations",
		Summary:     "Validate a transaction",
		Tags:        []string{"Validations"},
		// APIKeyOnly: this op reflects cfg.APIKeyOnlyValidation — the Fiber guard
		// forces X-API-Key (no bearer) for the hot validation path. Spec metadata
		// only; runtime auth stays the guard.With(..., forceAPIKey=true) middleware.
		Security: secAPIKeyOnly,
		// SkipValidateBody: the body is taken as RawBody and validated imperatively
		// by NormalizeAndValidate inside the core, which produces the canonical
		// Midaz error codes. Without this, Huma validates the JSON body against the
		// RawBody schema and rejects it with a native 422 before the handler runs.
		SkipValidateBody: true,
		// Dual-status: Huma auto-registers 200 from the Body field; declare 201
		// explicitly so both are advertised. No DefaultStatus is set — the runtime
		// status comes from ValidateOutputHuma.Status (200 duplicate / 201 new).
		Responses: map[string]*huma.Response{
			"201": {Description: "New validation created"},
		},
	}, h.ValidateHuma)
}
