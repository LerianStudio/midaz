// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRecoverLogger(t *testing.T) {
	logger := &libLog.GoLogger{}
	opt := WithRecoverLogger(logger)

	mid := &recoverMiddleware{}
	opt(mid)

	assert.Equal(t, logger, mid.Logger)
}

func TestBuildRecoverOpts_DefaultLogger(t *testing.T) {
	mid := buildRecoverOpts()

	assert.NotNil(t, mid)
	assert.IsType(t, &libLog.GoLogger{}, mid.Logger)
}

func TestBuildRecoverOpts_WithCustomLogger(t *testing.T) {
	logger := &libLog.GoLogger{}
	mid := buildRecoverOpts(WithRecoverLogger(logger))

	assert.NotNil(t, mid)
	assert.Equal(t, logger, mid.Logger)
}

func TestWithRecover_NoPanic(t *testing.T) {
	app := fiber.New()

	app.Use(WithRecover())
	app.Get("/ok", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWithRecover_PanicReturns500(t *testing.T) {
	app := fiber.New()

	app.Use(WithRecover())
	app.Get("/panic", func(c *fiber.Ctx) error {
		panic("something went wrong")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()

	var internalErr pkg.InternalServerError
	err = json.Unmarshal(body, &internalErr)
	require.NoError(t, err)

	assert.Equal(t, constant.ErrInternalServer.Error(), internalErr.Code)
	assert.Equal(t, "Internal Server Error", internalErr.Title)
	assert.Equal(t, "The server encountered an unexpected error. Please try again later or contact support.", internalErr.Message)
}

func TestWithRecover_PanicWithCustomLogger(t *testing.T) {
	app := fiber.New()

	logger := &libLog.GoLogger{}
	app.Use(WithRecover(WithRecoverLogger(logger)))
	app.Get("/panic", func(c *fiber.Ctx) error {
		panic("custom logger panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestWithRecover_PanicWithErrorValue(t *testing.T) {
	app := fiber.New()

	app.Use(WithRecover())
	app.Get("/panic-err", func(c *fiber.Ctx) error {
		panic(42)
	})

	req := httptest.NewRequest(http.MethodGet, "/panic-err", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestBuildRecoverOpts_MultipleOptions(t *testing.T) {
	logger1 := &libLog.GoLogger{}
	logger2 := &libLog.GoLogger{}

	mid := buildRecoverOpts(
		WithRecoverLogger(logger1),
		WithRecoverLogger(logger2),
	)

	assert.Equal(t, logger2, mid.Logger, "last option should win")
}
