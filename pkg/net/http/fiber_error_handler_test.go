// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

// canonicalEnvelope is the only client-facing error shape per E13.
type canonicalEnvelope struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

func newAppWithCanonicalHandler() *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          CanonicalFiberErrorHandler,
	})
	app.Get("/ok", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Get("/unauthorized", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized")
	})
	app.Get("/boom", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "kaboom")
	})

	return app
}

func decodeEnvelope(t *testing.T, app *fiber.App, method, path string) (int, canonicalEnvelope) {
	t.Helper()

	resp, err := app.Test(httptest.NewRequest(method, path, nil))
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var env canonicalEnvelope
	require.NoError(t, json.Unmarshal(body, &env), "body must be the canonical {code,title,message} envelope, got: %s", string(body))

	// E13: the only client-facing shape is {code,title,message}, never {"error":...}.
	var keys map[string]any
	require.NoError(t, json.Unmarshal(body, &keys))
	require.NotContains(t, keys, "error", "legacy {\"error\":...} envelope is banned (E13)")

	return resp.StatusCode, env
}

func TestCanonicalFiberErrorHandler_RouteNotFound(t *testing.T) {
	t.Parallel()

	app := newAppWithCanonicalHandler()

	status, env := decodeEnvelope(t, app, fiber.MethodGet, "/does-not-exist")

	require.Equal(t, fiber.StatusNotFound, status)
	require.Equal(t, constant.ErrRouteNotFound.Error(), env.Code, "code must be the canonical string sentinel, not the integer status")
	require.NotEmpty(t, env.Title)
	require.NotEmpty(t, env.Message)
}

func TestCanonicalFiberErrorHandler_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	app := newAppWithCanonicalHandler()

	// /ok only registers GET; a POST yields fiber's 405.
	status, env := decodeEnvelope(t, app, fiber.MethodPost, "/ok")

	require.Equal(t, fiber.StatusMethodNotAllowed, status)
	require.Equal(t, constant.ErrMethodNotAllowed.Error(), env.Code)
	require.NotEmpty(t, env.Title)
	require.NotEmpty(t, env.Message)
}

func TestCanonicalFiberErrorHandler_Unauthorized(t *testing.T) {
	t.Parallel()

	app := newAppWithCanonicalHandler()

	status, env := decodeEnvelope(t, app, fiber.MethodGet, "/unauthorized")

	require.Equal(t, fiber.StatusUnauthorized, status)
	require.Equal(t, constant.ErrInvalidToken.Error(), env.Code)
	require.NotEmpty(t, env.Title)
	require.NotEmpty(t, env.Message)
	// E9: never leak the raw fiber message verbatim as the code/title.
	require.NotEqual(t, "Unauthorized", env.Code)
}

func TestCanonicalFiberErrorHandler_PayloadTooLarge(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          CanonicalFiberErrorHandler,
	})
	app.Get("/413", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, "Request Entity Too Large")
	})

	status, env := decodeEnvelope(t, app, fiber.MethodGet, "/413")

	require.Equal(t, fiber.StatusRequestEntityTooLarge, status)
	require.Equal(t, constant.ErrPayloadTooLarge.Error(), env.Code)
	require.NotEmpty(t, env.Title)
	require.NotEmpty(t, env.Message)
}

func TestCanonicalFiberErrorHandler_GenericInternal(t *testing.T) {
	t.Parallel()

	app := newAppWithCanonicalHandler()

	status, env := decodeEnvelope(t, app, fiber.MethodGet, "/boom")

	require.Equal(t, fiber.StatusInternalServerError, status)
	require.NotEmpty(t, env.Code)
	require.NotEmpty(t, env.Title)
	require.NotEmpty(t, env.Message)
	// E9: the raw handler message must not leak into the client envelope.
	require.NotContains(t, env.Message, "kaboom")
	require.NotEqual(t, "kaboom", env.Message)
}
