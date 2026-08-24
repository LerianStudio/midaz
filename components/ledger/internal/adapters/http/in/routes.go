// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file holds the primitives every per-resource <resource>_routes.go file
// builds its Fiber guard chain from: the "midaz" authz appName, the
// protectedMidaz chain constructor, and the routeXxx helpers that work around
// Fiber v3's variadic handler signature. The route surfaces themselves live one
// file per resource, next to that resource's Huma registrar.

const midazName = "midaz"

// Operation-ID version suffixes. The ledger serves every family it hosts — onboarding,
// transaction, CRM and composition — on the /v1 and /v2 version groups of ONE OpenAPI
// document. huma.OpenAPI.AddOperation scans the whole document and panics on a duplicate
// operation ID, so a v1 op and its v2 twin — same handler, same path shape under a
// different version prefix — MUST carry distinct operation IDs or the ledger panics at
// boot. The V2 suffix makes that disjunction a boot invariant; it secondarily keeps IDs
// unique across the ledger<->tracer hub-spec join. The v1 suffix is empty so the /v1
// operation IDs — the ones published SDKs already bind to — stay exactly what they were.
//
// One pair serves every family, because every family needs the same two values. A family
// that ever needs a different suffix declares its own rather than changing these, since
// editing a value here moves the operation IDs of everything at once. Fees keep a separate
// feeOpSuffixV2 in fees_v2_register.go.
const (
	v1OpSuffix = ""
	v2OpSuffix = "V2"
)

// SettingsMaxPayloadSize defines the maximum payload size for settings endpoints (64KB).
const SettingsMaxPayloadSize = 64 * 1024

func protectedMidaz(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(midazName, resource, action), routeOptions, handlers...)
}

// registerRoute registers a protected handler chain on a Fiber v3 router. Fiber
// v3's route methods take (handler any, handlers ...any) and a []fiber.Handler
// cannot be spread into ...any, so the chain is split across the fixed first
// handler and the variadic tail. The chain always carries at least the auth
// handler, so index 0 is safe.
func registerRoute(r fiber.Router, method, path string, chain []fiber.Handler) {
	tail := make([]any, len(chain)-1)
	for i, h := range chain[1:] {
		tail[i] = h
	}

	r.Add([]string{method}, path, chain[0], tail...)
}

func routePost(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodPost, path, chain)
}

func routeGet(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodGet, path, chain)
}

func routePatch(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodPatch, path, chain)
}

func routePut(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodPut, path, chain)
}

func routeDelete(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodDelete, path, chain)
}

func routeHead(r fiber.Router, path string, chain []fiber.Handler) {
	registerRoute(r, fiber.MethodHead, path, chain)
}
