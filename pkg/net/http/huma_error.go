// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"net/http"

	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/danielgtaylor/huma/v2"
)

// HumaProblem converts a canonical Midaz error into the frozen RFC 9457 *Detail,
// returned as the error a Huma handler yields so Huma serializes the problem+json
// body. It is the Huma-transport analogue of WithError: both go through the same
// ProblemDetail classifier, so the two transports emit field/status/code/type/
// entityType-identical envelopes (the decoded bodies match exactly, guarded by
// the money-path golden net; raw bytes differ only by Huma's encoder trailing
// '\n' + HTML-escaping, invisible to any JSON parser).
//
// *Detail satisfies huma.StatusError (GetStatus/Error) and ContentTypeFilter
// (application/problem+json) via its embedded huma.ErrorModel, so returning it as
// the error is all Huma needs to render the frozen body + correct status. This is
// the single shared seam every Huma-migrated handler reuses — do NOT redefine it
// per package.
func HumaProblem(err error) error {
	// ResponseError is the malformed-request-body shape (ValidateUnmarshallingError
	// -> code 0094). On the Fiber path it is written by BadRequest at a HARD-CODED
	// HTTP 400 (response.go), NOT via WithError's status-in-Code JSONResponseError
	// branch — its Code ("0094") is a business code, not an HTTP status. So the
	// faithful Huma projection pins 400 and carries the same Code/Title/Detail,
	// now as problem+json (the sanctioned envelope change for the body-decode path;
	// code + status stay identical). classifyForProblem does not handle
	// ResponseError, so this must be mapped explicitly here.
	var responseErr pkg.ResponseError
	if errors.As(err, &responseErr) {
		detail := Detail{
			Detail: libProblem.Detail{
				ErrorModel: huma.ErrorModel{
					Status: http.StatusBadRequest,
					Title:  responseErr.Title,
					Detail: responseErr.Message,
				},
				Code: responseErr.Code,
			},
			EntityType: responseErr.EntityType,
		}
		if responseErr.Code != "" {
			detail.Type = libProblem.BaseURI + "/" + responseErr.Code
		}

		return &detail
	}

	// ValidateStruct returns a *ValidationKnownFieldsError (POINTER) for a failed
	// go-playground rule (required/max/etc.) — the one business error in the codebase
	// wrapped in a pointer (locked in by validation_test.go). ProblemDetail's
	// classifier matches only VALUE targets (errors.As can't assign *T to a T
	// target), so dereference to the value here; every other producer already
	// returns a value. Without this the body-validation path 500s instead of 400.
	var knownFieldsPtr *pkg.ValidationKnownFieldsError
	if errors.As(err, &knownFieldsPtr) {
		err = *knownFieldsPtr
	}

	detail, ok := ProblemDetail(err)
	if !ok {
		// Unreachable: ProblemDetail only fails on MapError's non-*Detail return.
		// Fall back to the canonical sanitized 500 shape.
		detail, _ = ProblemDetail(nil)
	}

	return &detail
}
