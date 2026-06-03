// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// bodyParsingHandler holds the struct source for body parsing without coupling to a handler.
type bodyParsingHandler struct {
	structSource any
}

// parseBody decodes, validates and sanitizes the request body, returning the parsed struct.
func (b *bodyParsingHandler) parseBody(c *fiber.Ctx) (any, error) {
	s := newOfType(b.structSource)

	bodyBytes := c.Body()

	if err := json.Unmarshal(bodyBytes, s); err != nil {
		validationErr := pkg.ValidateUnmarshallingError(err)
		_ = commonsHttp.Respond(c, fiber.StatusBadRequest, validationErr)

		return nil, validationErr
	}

	marshaled, err := json.Marshal(s)
	if err != nil {
		validationErr := pkg.ValidateUnmarshallingError(err)
		_ = commonsHttp.Respond(c, fiber.StatusBadRequest, validationErr)

		return nil, validationErr
	}

	var originalMap, marshaledMap map[string]any

	if err := json.Unmarshal(bodyBytes, &originalMap); err != nil {
		validationErr := pkg.ValidateUnmarshallingError(err)
		_ = commonsHttp.Respond(c, fiber.StatusBadRequest, validationErr)

		return nil, validationErr
	}

	if err := json.Unmarshal(marshaled, &marshaledMap); err != nil {
		validationErr := pkg.ValidateUnmarshallingError(err)
		_ = commonsHttp.Respond(c, fiber.StatusBadRequest, validationErr)

		return nil, validationErr
	}

	diffFields := findUnknownFields(originalMap, marshaledMap)

	if len(diffFields) > 0 {
		validationErr := pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, pkg.FieldValidations{}, "", diffFields)
		_ = commonsHttp.Respond(c, fiber.StatusBadRequest, validationErr)

		return nil, validationErr
	}

	sanitizeStruct(s)

	if err := ValidateStruct(s); err != nil {
		_ = commonsHttp.Respond(c, fiber.StatusBadRequest, err)

		return nil, err
	}

	c.Locals("fields", diffFields)

	parseMetadata(s, originalMap)

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
