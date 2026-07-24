// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterRoute_PreservesAuthFirstChainOrder locks the money-path invariant that
// registerRoute invokes chain[0] (auth) first and then the remaining handlers in
// chain[1]→chain[n] order. Each marker handler appends its name to a shared slice and
// calls c.Next(); the terminal returns 200. The observed order proves auth always runs
// before tenant/parse/terminal after the Fiber v3 (handler any, handlers ...any) split.
func TestRegisterRoute_PreservesAuthFirstChainOrder(t *testing.T) {
	app := fiber.New()

	var order []string
	marker := func(name string) fiber.Handler {
		return func(c fiber.Ctx) error {
			order = append(order, name)
			return c.Next()
		}
	}

	chain := []fiber.Handler{
		marker("auth"),
		marker("tenant"),
		marker("parse"),
		func(c fiber.Ctx) error {
			order = append(order, "terminal")
			return c.SendStatus(fiber.StatusOK)
		},
	}

	registerRoute(app, fiber.MethodGet, "/register-route-order", chain)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/register-route-order", nil), fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, []string{"auth", "tenant", "parse", "terminal"}, order)
}

// TestRegisterRoute_SingleHandlerChain covers the append([]any{handler}, tail...)
// boundary where tail is empty — the ProtectedRouteChain len==1 case (auth handler only
// acting as terminal). The route must register and respond without panicking.
func TestRegisterRoute_SingleHandlerChain(t *testing.T) {
	app := fiber.New()

	terminalCalled := false
	chain := []fiber.Handler{
		func(c fiber.Ctx) error {
			terminalCalled = true
			return c.SendStatus(fiber.StatusOK)
		},
	}

	require.NotPanics(t, func() {
		registerRoute(app, fiber.MethodGet, "/register-route-single", chain)
	})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/register-route-single", nil), fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.True(t, terminalCalled, "single-handler chain terminal should fire")
}
