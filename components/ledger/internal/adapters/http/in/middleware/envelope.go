// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// envelopeRenderer converts one serialized problem+json body into another wire
// shape. status is the response's HTTP status, which the renderer cross-checks
// against the body so a non-problem payload is never reshaped.
//
// ok=false means "leave the original bytes alone" and is the ONLY failure signal:
// a renderer must never panic, because this middleware is registered outside the
// panic-recovery middleware.
type envelopeRenderer func(body []byte, status int) ([]byte, bool)

// versionEnvelope binds a route-version prefix to the envelope that version
// serves.
type versionEnvelope struct {
	prefix string
	render envelopeRenderer
}

// versionEnvelopes lists the route versions that do NOT serve the current
// envelope. A version absent from this list inherits RFC 9457, so a new /vN
// needs no entry and no file — only a version that deliberately diverges does.
//
// /v1 is the one diverging version: it shipped in midaz v3 with a
// {code,title,message} body, and clients in production parse it. /v2 and
// everything after start on RFC 9457 and stay there.
var versionEnvelopes = []versionEnvelope{
	{prefix: "/v1/", render: renderLegacyV1},
}

// rendererFor resolves the renderer for a request path, or nil when the path's
// version keeps the current envelope.
func rendererFor(path string) envelopeRenderer {
	for _, envelope := range versionEnvelopes {
		if strings.HasPrefix(path, envelope.prefix) {
			return envelope.render
		}
	}

	return nil
}

// ErrorEnvelope rewrites error responses to the envelope their route version
// serves. A version the registry does not name keeps the RFC 9457 body untouched.
//
// It MUST be registered outermost — ahead of the panic-recovery middleware — so
// that a recovered panic's 500 body, written while unwinding c.Next(), is
// rewritten too. That ordering is only safe because every path through this
// middleware either rewrites the body wholesale or leaves it exactly as it was;
// there is no partial write and nothing here can panic.
func ErrorEnvelope() fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()

		rewriteErrorEnvelope(c)

		return err
	}
}

// WrapErrorHandler reshapes responses written by the Fiber error handler.
//
// ErrorEnvelope alone is not enough. A handler that RETURNS an error rather than
// writing one — Fiber's router on an unmatched route (404) or a rejected method
// (405), the auth chain's 401, the body-limit 413, the header-limit 431, and
// WithError's own fallthrough arm — leaves the response empty while the middleware
// chain unwinds. The error handler runs afterwards, at which point ErrorEnvelope
// has already been and gone, so those responses would keep the /v2 envelope on a
// /v1 route.
//
// Wrapping the handler covers them. The two seams are safe together because a
// reshape is idempotent: the renderer refuses any body whose status member does
// not match the response status, and a body it already rewrote has no status
// member at all.
func WrapErrorHandler(next fiber.ErrorHandler) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		handlerErr := next(c, err)

		rewriteErrorEnvelope(c)

		return handlerErr
	}
}

func rewriteErrorEnvelope(c fiber.Ctx) {
	response := c.Response()

	// Only error responses carry an envelope. Gating on status rather than on the
	// Content-Type is deliberate: fiber's c.JSON overwrites the media type
	// withProblem sets, so a problem body written through the Fiber path arrives
	// here labelled application/json and a media-type gate would miss it. The
	// renderer cross-checks the body's own status member against this one, so a
	// 4xx payload that is not a problem document is refused rather than mangled.
	status := response.StatusCode()
	if status < http.StatusBadRequest {
		return
	}

	body := response.Body()
	if len(body) == 0 {
		return
	}

	render := rendererFor(c.Path())
	if render == nil {
		return
	}

	if rewritten, ok := render(body, status); ok {
		response.Header.SetContentType(fiber.MIMEApplicationJSON)
		response.SetBody(rewritten)
	}
}
