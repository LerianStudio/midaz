// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shipped registry claims exactly one version. A future /vN inheriting the
// current envelope by DEFAULT is the property that keeps this design cheap, so it
// is asserted rather than assumed.
func TestRendererFor_ClaimsV1AndNothingElse(t *testing.T) {
	require.Len(t, versionEnvelopes, 1, "only /v1 diverges from the current envelope")

	assert.NotNil(t, rendererFor("/v1/organizations/x"))

	// The bare version path is that version too: "/v1" matches no route, so the
	// Fiber error handler answers it, and without this it would be a /v1 URL
	// carrying the /v2 envelope.
	assert.NotNil(t, rendererFor("/v1"), "the bare version path belongs to /v1")
	assert.NotNil(t, rendererFor("/v1/"))

	for _, path := range []string{
		"/v2/organizations/x",
		"/v3/anything",
		"/health",
		"/",
		// Neither a prefix match nor the bare version path.
		"/v10/organizations/x",
		"/v1x",
	} {
		assert.Nil(t, rendererFor(path), "path %q must keep the current envelope", path)
	}
}

func TestErrorEnvelope_Middleware(t *testing.T) {
	problem := `{"type":"https://errors.lerian.studio/v1/0065","title":"Invalid Path Parameter",` +
		`"status":400,"detail":"bad path","code":"0065","entityType":"Ledger"}`

	t.Run("success responses are never touched", func(t *testing.T) {
		body, contentType := driveEnvelope(t, "/health", fiber.StatusOK, fiber.MIMEApplicationJSON, `{"status":"ok"}`)

		assert.JSONEq(t, `{"status":"ok"}`, body)
		assert.Contains(t, contentType, fiber.MIMEApplicationJSON)
	})

	t.Run("unclaimed versions keep the problem body and media type", func(t *testing.T) {
		// /v2 is genuinely unclaimed in the shipped registry, so no swap is needed.
		body, contentType := driveEnvelope(t, "/v2/organizations/x", fiber.StatusBadRequest, "application/problem+json", problem)

		assert.JSONEq(t, problem, body)
		assert.Contains(t, contentType, "problem+json")
	})

	t.Run("a claimed version is rewritten and re-typed", func(t *testing.T) {
		body, contentType := driveEnvelope(t, "/v1/organizations/x", fiber.StatusBadRequest, "application/problem+json", problem)

		assert.Equal(t, `{"title":"Invalid Path Parameter","message":"bad path","code":"0065"}`, body)
		assert.Contains(t, contentType, fiber.MIMEApplicationJSON)
		assert.NotContains(t, contentType, "problem+json")
	})

	t.Run("a body the renderer refuses passes through untouched", func(t *testing.T) {
		unconvertible := `{"title":"no status member","code":"0065"}`

		body, contentType := driveEnvelope(t, "/v1/organizations/x", fiber.StatusBadRequest, "application/problem+json", unconvertible)

		assert.JSONEq(t, unconvertible, body)
		assert.Contains(t, contentType, "problem+json", "a refused rewrite must not change the media type either")
	})

	t.Run("an empty body is left alone", func(t *testing.T) {
		body, _ := driveEnvelope(t, "/v2/organizations/x", fiber.StatusBadRequest, "application/problem+json", "")

		assert.Empty(t, body)
	})
}

// driveEnvelope serves one canned response through the middleware and returns
// what a client would receive.
func driveEnvelope(t *testing.T, path string, status int, contentType, body string) (string, string) {
	t.Helper()

	app := fiber.New()
	app.Use(ErrorEnvelope())
	app.Get("/*", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, contentType)
		c.Status(status)

		return c.SendString(body)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(raw), resp.Header.Get(fiber.HeaderContentType)
}
