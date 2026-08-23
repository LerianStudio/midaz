// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"encoding/json"
	"net/http"
)

// problemBody is the subset of the RFC 9457 envelope the legacy renderer reads.
// It is declared here rather than reusing pkgHTTP.Detail so the renderer stays a
// pure function over bytes and does not couple the /v1 wire shape to the shared
// envelope type — the two are allowed to drift, which is the whole point of a
// per-version registry.
type problemBody struct {
	Code       string         `json:"code"`
	Title      string         `json:"title"`
	Detail     string         `json:"detail"`
	Status     int            `json:"status"`
	EntityType string         `json:"entityType"`
	Errors     []problemError `json:"errors"`
}

// problemError mirrors huma.ErrorDetail, the shape ProblemDetail's fields->errors[]
// remap emits (pkg/net/http/problem.go, fieldsToErrors).
type problemError struct {
	Message  string `json:"message"`
	Location string `json:"location"`
	Value    any    `json:"value"`
}

// legacyFlatBody is the v3 envelope for the classes WithError rendered through a
// fiber.Map: NotFound, Conflict, UnprocessableEntity, Unauthorized, Forbidden,
// InternalServerError, FailedPrecondition and ServiceUnavailable.
//
// Field ORDER is load-bearing. fiber.Map is a map, and encoding/json sorts map
// keys, so v3 emitted these three alphabetically. Nothing is omitempty: a map
// literal always carries all three keys, even when a value is empty.
type legacyFlatBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Title   string `json:"title"`
}

// legacyStructBody is the v3 envelope for the classes WithError rendered by
// serializing the error struct itself: ValidationError, ValidationKnownFieldsError,
// ValidationUnknownFieldsError and ResponseError.
//
// Field order and the omitempty set are copied from pkg/errors.go as it stood at
// 270bc45ef^ — declaration order is what encoding/json emits for a struct, so this
// ordering IS the v3 byte layout.
//
// The v3 structs also carried `err,omitempty`. It is deliberately not reproduced:
// it serialized a Go error value, which rendered as an empty object for the usual
// error types and carried nothing a client could branch on.
type legacyStructBody struct {
	EntityType string         `json:"entityType,omitempty"`
	Title      string         `json:"title,omitempty"`
	Message    string         `json:"message,omitempty"`
	Code       string         `json:"code,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
}

// renderLegacyV1 converts one serialized RFC 9457 body into the v3 envelope that
// /v1 served before the problem+json swap. It returns ok=false — leaving the
// caller to pass the original bytes through untouched — for anything it cannot
// confidently convert, so a shape this renderer does not recognize degrades to
// the current envelope rather than to a broken one.
//
// It never panics: that is a hard requirement, because the middleware calling it
// is registered outside the panic-recovery middleware.
func renderLegacyV1(body []byte, status int) ([]byte, bool) {
	var problem problemBody
	if err := json.Unmarshal(body, &problem); err != nil {
		return nil, false
	}

	// A problem document restates its HTTP status in the body. A payload that does
	// not, or that disagrees, is something else — refuse rather than guess.
	if problem.Status != status {
		return nil, false
	}

	if problem.Status == http.StatusBadRequest {
		return marshalOrPassThrough(legacyStructBody{
			EntityType: structClassEntityType(problem),
			Title:      problem.Title,
			Message:    problem.Detail,
			Code:       problem.Code,
			Fields:     errorsToFields(problem.Errors),
		})
	}

	return marshalOrPassThrough(legacyFlatBody{
		Code:    problem.Code,
		Message: problem.Detail,
		Title:   problem.Title,
	})
}

// structClassEntityType decides whether entityType belongs on a 400 body.
//
// v3 split the 400s in two. The field-bearing classes serialized their own
// EntityType, but WithError rendered a plain ValidationError by copying it into a
// ValidationKnownFieldsError and leaving EntityType zero — so a ValidationError
// reached the client WITHOUT it, even though v4 carries one. The presence of
// errors[] is what separates the two at this layer.
//
// KNOWN GAP: a ValidationKnownFieldsError built with an empty Fields map produces
// no errors[], so it is indistinguishable from a ValidationError here and loses
// entityType where v3 kept it. Closing it would require carrying the concrete
// error class on the wire or in a header, which costs a public schema change to
// fix a case no client is known to depend on.
func structClassEntityType(problem problemBody) string {
	if len(problem.Errors) == 0 {
		return ""
	}

	return problem.EntityType
}

// errorsToFields reverses the fields->errors[] remap. The known-fields shape put
// the violation text in Message; the unknown-fields shape put the offending value
// in Value and a fixed string in Message, so a non-nil Value is what marks it.
func errorsToFields(details []problemError) map[string]any {
	if len(details) == 0 {
		return nil
	}

	fields := make(map[string]any, len(details))

	for _, detail := range details {
		if detail.Location == "" {
			continue
		}

		if detail.Value != nil {
			fields[detail.Location] = detail.Value
			continue
		}

		fields[detail.Location] = detail.Message
	}

	if len(fields) == 0 {
		return nil
	}

	return fields
}

func marshalOrPassThrough(body any) ([]byte, bool) {
	out, err := json.Marshal(body)
	if err != nil {
		return nil, false
	}

	return out, true
}
