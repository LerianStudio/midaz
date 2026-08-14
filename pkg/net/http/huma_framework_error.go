// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"net/http"
	"sync"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg"
)

// humaEmptyBodyMessage is the message Huma passes to WriteErr when a request
// carrying a required body arrives with zero bytes. Pinned to huma/v2@v2.39.0
// huma.go:2197; Huma exports no constant for it, so the coupling is guarded by
// TestHuma_CreateOrganization_EmptyBody_Canonical400, which drives a real
// Fiber+Huma request and fails if a Huma upgrade rewords this string.
const humaEmptyBodyMessage = "request body is required"

// errEmptyJSONBody mirrors the error encoding/json yields for zero-length input,
// so an empty body reports the same detail the pre-Huma Fiber path reported when
// it reached json.Unmarshal([]byte{}, s) in DecodeAndValidate.
var errEmptyJSONBody = errors.New("unexpected end of JSON input")

// InstallHumaFrameworkErrors maps Huma's own framework errors onto the canonical
// Midaz envelope. It must run alongside lib-commons' problem.Install (order
// between the two does not matter) and is safe to call once per mounted group.
//
// Huma rejects a zero-length body from its request-body precondition, BEFORE it
// honours SkipValidateBody, so DecodeAndValidate — the sole body validator for
// every RawBody operation — never sees an empty body and the rejection never
// carries a business code. lib-commons' huma.NewError override renders framework
// errors as problem+json but deliberately leaves Code empty, which would emit a
// 400 with no `code` for a case the Fiber path answered with 0094. Mapping it
// here restores that answer for every body-bearing operation at once.
//
// Operations without a body (RawBody absent => nil RequestBody) never trigger the
// precondition, so bodiless mutations such as transaction commit/cancel/revert are
// untouched.
//
// installHumaFrameworkErrorsMu serializes install-vs-install, mirroring
// lib-commons problem.Install: huma.NewErrorWithContext is a plain package var, so
// two goroutines assigning it concurrently is a data race. Huma READS it
// unsynchronized on every error-constructing request, so this still MUST run during
// bootstrap before the server serves — the mutex covers installs against each
// other, not installs against those reads.
var installHumaFrameworkErrorsMu sync.Mutex

func InstallHumaFrameworkErrors() {
	installHumaFrameworkErrorsMu.Lock()
	defer installHumaFrameworkErrorsMu.Unlock()

	huma.NewErrorWithContext = func(_ huma.Context, status int, msg string, errs ...error) huma.StatusError {
		if status == http.StatusBadRequest && msg == humaEmptyBodyMessage {
			if detail, ok := HumaProblem(pkg.ValidateUnmarshallingError(errEmptyJSONBody)).(huma.StatusError); ok {
				return detail
			}
		}

		// Read huma.NewError at call time so lib-commons' override applies
		// regardless of install order, and so repeated installs stay idempotent
		// instead of chaining onto a captured predecessor.
		return huma.NewError(status, msg, errs...)
	}
}
