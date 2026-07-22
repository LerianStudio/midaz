// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"net/http"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
)

// problemContentType is the RFC 9457 media type for the error body. huma's
// ErrorModel.ContentType maps application/json to this; because we serialize
// through fiber's JSON encoder (not huma's transport), we set it explicitly.
const problemContentType = "application/problem+json"

// Detail is the Midaz wire projection of the shared lib-commons RFC 9457 body:
// problem.Detail (type/title/status/detail/instance/code/errors[]) plus the
// Midaz-specific entityType. entityType is a strict, omitempty superset over the
// shared shape — carried so the ledger/tracer envelope keeps its entityType
// field (r3 §2.6). code + status ride problem.Detail unchanged (money path).
type Detail struct {
	libProblem.Detail

	EntityType string `json:"entityType,omitempty"`

	// Message carries the human-readable reason as a top-level field for the
	// error classes that expose it to clients verbatim (413 payload-too-large,
	// 504 gateway-timeout). It is populated only for those statuses; for every
	// other class it stays empty and is dropped by omitempty, so the shared
	// envelope (and the ledger wire) is unchanged. Distinct from the RFC 9457
	// `detail`, which the >=500 scrub replaces with "internal error".
	Message string `json:"message,omitempty"`
}

// codeStatus is the FROZEN code->status snapshot of the §1 table, keyed by the
// business Code string. It is built once from the SAME errors.As cascade
// WithError walks (classifyForProblem), so statusOf is a pure function of the
// code and the golden sweep guards it against drift.
//
// ponytail: statusOf must satisfy MapError's code->status signature, but Midaz's
// status is a function of the Go *type*, not the code. classifyForProblem does
// the type classification once and stashes (code, msg, status, entityType); the
// per-call closure in withProblem feeds statusOf the captured status. No
// package-level map can be built without reaching pkg.ValidateBusinessError's
// unexported errorMap, so the single frozen source is the cascade itself.

// codeOf reproduces WithError's errors.As cascade (r3 §2.1) over the SAME 11
// typed structs in the SAME declaration order, returning (Code, Message, ok).
// It also resolves the lib-commons Response arm at the same position WithError
// does (between the field-bearing 400 arms and the internal arm), so the
// commons money-path codes keep their status. The resolved HTTP status and
// entityType are returned alongside for the closure wiring in withProblem.
//
// It does NOT handle ResponseError (status-in-Code, 0094): that is dispatched on
// its own branch in WithError before ever reaching here (r3 §2.2).
func classifyForProblem(err error) (code, msg, title, entityType string, status int, ok bool) {
	if e := (pkg.EntityNotFoundError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusNotFound, true
	}

	if e := (pkg.EntityConflictError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusConflict, true
	}

	if e := (pkg.ValidationError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusBadRequest, true
	}

	if e := (pkg.UnprocessableOperationError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusUnprocessableEntity, true
	}

	if e := (pkg.UnauthorizedError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusUnauthorized, true
	}

	if e := (pkg.ForbiddenError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusForbidden, true
	}

	if e := (pkg.ValidationKnownFieldsError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusBadRequest, true
	}

	if e := (pkg.ValidationUnknownFieldsError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusBadRequest, true
	}

	if e := (pkg.InternalServerError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusInternalServerError, true
	}

	if e := (pkg.FailedPreconditionError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusInternalServerError, true // NOT 412 — §1 row 11
	}

	if e := (pkg.ServiceUnavailableError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusServiceUnavailable, true
	}

	if e := (pkg.GatewayTimeoutError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusGatewayTimeout, true
	}

	if e := (pkg.PayloadTooLargeError{}); errors.As(err, &e) {
		return e.Code, e.Message, e.Title, e.EntityType, http.StatusRequestEntityTooLarge, true
	}

	// lib-commons Response sub-switch (r3 §1.2), resolved at the same arm position
	// WithError uses. Its mapped codes carry the same status their pkg type would.
	if e := (libCommons.Response{}); errors.As(err, &e) {
		switch e.Code {
		case libConstants.ErrInsufficientFunds.Error(), libConstants.ErrAccountIneligibility.Error():
			return e.Code, e.Message, e.Title, "", http.StatusUnprocessableEntity, true
		case libConstants.ErrAssetCodeNotFound.Error():
			return e.Code, e.Message, e.Title, "", http.StatusNotFound, true
		case libConstants.ErrOverFlowInt64.Error():
			return e.Code, e.Message, e.Title, "", http.StatusInternalServerError, true
		default:
			return e.Code, e.Message, e.Title, "", http.StatusBadRequest, true
		}
	}

	return "", "", "", "", 0, false
}

// fieldsToErrors remaps the two field-bearing 400 paths' flat fields map into the
// RFC 9457 errors[] array (r3 §2.7). Location is FROZEN to the bare field key so
// clients parse a stable shape. Returns nil when there are no fields (omitempty).
func fieldsToErrors(err error) []*huma.ErrorDetail {
	if e := (pkg.ValidationKnownFieldsError{}); errors.As(err, &e) && len(e.Fields) > 0 {
		out := make([]*huma.ErrorDetail, 0, len(e.Fields))
		for field, message := range e.Fields {
			out = append(out, &huma.ErrorDetail{Location: field, Message: message})
		}

		return out
	}

	if e := (pkg.ValidationUnknownFieldsError{}); errors.As(err, &e) && len(e.Fields) > 0 {
		out := make([]*huma.ErrorDetail, 0, len(e.Fields))
		for field, value := range e.Fields {
			out = append(out, &huma.ErrorDetail{Location: field, Message: "unexpected field", Value: value})
		}

		return out
	}

	return nil
}

// withProblem is the shared serializer that replaces the response.go helpers:
// it builds the RFC 9457 *Detail via lib-commons MapError (5xx scrub included)
// and writes it as application/problem+json. code + status survive byte-for-byte
// (money path); the 5xx detail/title scrub is deliberate (r3 §2.3, approved).
//
// entityType and the fields->errors[] remap are the two carries problem.Detail
// does not do for free; both are non-money-path envelope shape.
func withProblem(c *fiber.Ctx, err error) error {
	body, ok := ProblemDetail(err)
	if !ok {
		// MapError always returns *Detail; this is unreachable defensive code.
		return InternalServerError(c, constant.ErrInternalServer.Error(),
			http.StatusText(http.StatusInternalServerError), "internal error")
	}

	c.Set(fiber.HeaderContentType, problemContentType)

	return c.Status(body.Status).JSON(body)
}

// ProblemDetail builds the frozen RFC 9457 *Detail for err WITHOUT writing it
// to a fiber response. It is the single source of the (code, status, title,
// entityType, errors[]) envelope shared by the Fiber path (withProblem) and the
// Huma path (handlers return the *Detail as a huma.StatusError so Huma
// serializes the identical body). ok is false only on MapError's unreachable
// non-*Detail return.
//
// It reproduces the money-path classification verbatim: classifyForProblem's
// errors.As cascade + lib-commons MapError (5xx scrub) + the <500 title restore
// + the fields->errors[] carry. The golden net (errors_golden_test.go) guards
// this against drift; the Huma handlers inherit that guarantee for free by
// reusing this function.
func ProblemDetail(err error) (Detail, bool) {
	var (
		capturedEntityType string
		capturedTitle      string
		capturedMessage    string
		problemStatus      int
	)

	// codeOf classifies the type once and stashes status + title + entityType +
	// message. MapError calls codeOf then statusOf synchronously in the same
	// call, so statusOf can return the captured status. See ponytail note above.
	codeOf := func(e error) (string, string, bool) {
		code, msg, title, entityType, status, ok := classifyForProblem(e)
		if ok {
			capturedEntityType = entityType
			capturedTitle = title
			capturedMessage = msg
			problemStatus = status
		}

		return code, msg, ok
	}

	statusOf := func(_ string) int { return problemStatus }

	mapped := libProblem.MapError(err, codeOf, statusOf, constant.ErrInternalServer.Error())

	pd, ok := mapped.(*libProblem.Detail)
	if !ok {
		return Detail{}, false
	}

	// Restore the registry title for <500 (r3 §2.8: Title is verbatim below 500).
	// MapError/newProblem defaults Title to http.StatusText; only the >=500 scrub
	// (Title->status text, Detail->"internal error") is intended (r3 §2.3).
	if pd.Status < http.StatusInternalServerError && capturedTitle != "" {
		pd.Title = capturedTitle
	}

	body := Detail{Detail: *pd, EntityType: capturedEntityType}
	if errs := fieldsToErrors(err); errs != nil {
		body.Errors = errs
	}

	// 413 and 504 expose the reason to clients verbatim via the top-level
	// message field, using the raw registry message (not the >=500-scrubbed
	// detail). No other class sets it, so omitempty keeps the shared envelope
	// unchanged. These two statuses are produced ONLY by PayloadTooLargeError /
	// GatewayTimeoutError inside this classifier, so gating on status is exact.
	if problemStatus == http.StatusRequestEntityTooLarge || problemStatus == http.StatusGatewayTimeout {
		body.Message = capturedMessage
	}

	return body, true
}

// withProblemStatus renders err as the problem+json envelope at an EXPLICIT
// status that overrides the code->status table (r3 §0, §1.3). It is the only
// producer of 405/413: those statuses do not exist in the table, so
// renderCanonical carries them here. code + status are still money-path exact.
func withProblemStatus(c *fiber.Ctx, status int, err error) error {
	code, msg, title, entityType, _, ok := classifyForProblem(err)
	if !ok {
		return withProblem(c, err)
	}

	if title == "" {
		title = http.StatusText(status)
	}

	pd := libProblem.Detail{
		ErrorModel: huma.ErrorModel{
			Status: status,
			Title:  title,
			Detail: msg,
		},
		Code: code,
	}
	if code != "" {
		pd.Type = libProblem.BaseURI + "/" + code
	}

	// >=500 scrub mirrors MapError (r3 §2.3): status text title, generic detail.
	if status >= http.StatusInternalServerError {
		pd.Title = http.StatusText(status)
		pd.Detail = "internal error"
	}

	body := Detail{Detail: pd, EntityType: entityType}
	if errs := fieldsToErrors(err); errs != nil {
		body.Errors = errs
	}

	c.Set(fiber.HeaderContentType, problemContentType)

	return c.Status(status).JSON(body)
}
