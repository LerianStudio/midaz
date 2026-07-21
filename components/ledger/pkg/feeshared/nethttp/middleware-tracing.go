// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// bodyParsingHandler holds the struct source for body parsing without coupling to a handler.
type bodyParsingHandler struct {
	structSource any
}

// DecodeValidateBody is the transport-agnostic decode + unknown-field +
// sanitize + validate + metadata sequence for a fee-package body. It is the SINGLE
// source of that sequence, shared by the Fiber body-parsing handler (parseBody) and
// the Huma handler cores, so both transports decode+validate identically with no
// drift. It decodes into the caller-provided struct pointer and returns the parsed
// originalMap plus the raw canonical Midaz error (ResponseError for malformed JSON,
// Validation* for unknown/missing/invalid fields) WITHOUT writing a response — the
// caller renders it (Fiber: BadRequest flat envelope; Huma: HumaProblem problem+json).
//
// The fee package keeps its OWN validator (this package's ValidateStruct, registered
// separately from pkg/net/http's) and its OWN unknown-field/sanitize/metadata
// helpers, so this must NOT be swapped for pkgHTTP.DecodeAndValidate — the two are
// distinct validator instances with different registered rules.
//
// NOTE: findUnknownFields short-circuits BEFORE ValidateStruct, exactly as the
// pre-refactor parseBody did — an unexpected field wins over a missing required one.
func DecodeValidateBody(bodyBytes []byte, s any) (map[string]any, error) {
	if err := json.Unmarshal(bodyBytes, s); err != nil {
		return nil, pkg.ValidateUnmarshallingError(err)
	}

	marshaled, err := json.Marshal(s)
	if err != nil {
		return nil, pkg.ValidateUnmarshallingError(err)
	}

	var originalMap, marshaledMap map[string]any

	if err := json.Unmarshal(bodyBytes, &originalMap); err != nil {
		return nil, pkg.ValidateUnmarshallingError(err)
	}

	if err := json.Unmarshal(marshaled, &marshaledMap); err != nil {
		return nil, pkg.ValidateUnmarshallingError(err)
	}

	diffFields := findUnknownFields(originalMap, marshaledMap)
	if len(diffFields) > 0 {
		return nil, pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, pkg.FieldValidations{}, "", diffFields)
	}

	sanitizeStruct(s)

	if err := ValidateStruct(s); err != nil {
		return nil, err
	}

	parseMetadata(s, originalMap)

	return originalMap, nil
}

// parseBody decodes, validates and sanitizes the request body, returning the parsed
// struct. It delegates the decode+validate sequence to DecodeValidateBody (the shared
// transport-agnostic core) and, on error, renders the canonical BadRequest envelope,
// keeping the Fiber and Huma paths byte-identical.
func (b *bodyParsingHandler) parseBody(c *fiber.Ctx) (any, error) {
	s := newOfType(b.structSource)

	if _, err := DecodeValidateBody(c.Body(), s); err != nil {
		_ = commonsHttp.Respond(c, fiber.StatusBadRequest, err)

		return nil, err
	}

	// parseBody only reaches here with zero unknown fields, so the stored diff is
	// always empty; kept for the existing c.Locals("fields") contract.
	c.Locals("fields", make(map[string]any))

	return s, nil
}

// WithBodyTracing wraps body parsing with OpenTelemetry tracing to measure body parsing time.
func WithBodyTracing(s any, h DecodeHandlerFunc) fiber.Handler {
	parsingHandler := &bodyParsingHandler{structSource: s}

	return func(c *fiber.Ctx) error {
		parentCtx := c.UserContext()

		_, tracer, reqID, _ := libObservability.NewTrackingFromContext(parentCtx)

		spanCtx, span := tracer.Start(parentCtx, "middleware.body_parsing")

		span.SetAttributes(
			attribute.String("app.request.request_id", reqID),
			attribute.String("url.path", c.Path()),
			attribute.String("http.request.method", c.Method()),
			attribute.String("http.route", c.Route().Path),
			attribute.Int("http.request.body.size", len(c.Body())),
		)

		c.SetUserContext(spanCtx)

		parsed, err := parsingHandler.parseBody(c)
		if err != nil {
			span.SetAttributes(attribute.String("error.message", err.Error()))
			span.End()

			return nil
		}

		span.End()

		c.SetUserContext(parentCtx)

		return h(parsed, c)
	}
}
