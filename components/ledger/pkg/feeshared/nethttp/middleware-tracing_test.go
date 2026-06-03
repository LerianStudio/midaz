// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithBodyTracing_ValidBody(t *testing.T) {
	app := fiber.New()

	handler := func(p any, c *fiber.Ctx) error {
		s := p.(*simpleTestStruct)
		assert.Equal(t, "test", s.Name)
		return c.SendStatus(fiber.StatusOK)
	}

	app.Post("/test", WithBodyTracing(&simpleTestStruct{}, handler))

	body := `{"name": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWithBodyTracing_InvalidBody(t *testing.T) {
	app := fiber.New()

	handler := func(p any, c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	}

	app.Post("/test", WithBodyTracing(&simpleTestStruct{}, handler))

	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestWithBodyTracing_EmptyBody(t *testing.T) {
	app := fiber.New()

	handler := func(p any, c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	}

	app.Post("/test", WithBodyTracing(&simpleTestStruct{}, handler))

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
