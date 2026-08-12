// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"regexp"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/gofiber/fiber/v3"
)

// feesApplicationName is the auth resource namespace for fee/billing routes. It
// is preserved verbatim from the standalone plugin-fees service: tenant-manager
// RBAC policies key on this string, so it MUST NOT be renamed (R9).
const feesApplicationName = "plugin-fees"

// feeSpecPathParam matches a single OpenAPI path-parameter segment, "{name}".
var feeSpecPathParam = regexp.MustCompile(`\{([^{}/]+)\}`)

// feeChainPath restates an OpenAPI path template in Fiber's route syntax, carrying the
// parameter names through unchanged, so the guard chain and the Huma terminal are cut
// from one path value instead of two hand-written ones.
//
// The names have to agree, and nothing else in the build reports it when they don't.
// ParseUUIDPathParameters reads the names the FIBER route declares and only parses the
// ones pkg/constant.UUIDPathParameters lists, so a name spelled differently on the chain
// than in the contract is passed through as an unvalidated string. Fiber matches on
// segment structure rather than on names, so the request still routes and the chain
// still runs; the spec-vs-routes gate compares parameter positions, so it stays green
// too.
func feeChainPath(specPath string) string {
	return feeSpecPathParam.ReplaceAllString(specPath, ":$1")
}

// feeGuardRoute is one operation of the fee and billing authorization surface: the
// verb and the sub-path it is served at — relative to whatever scope mounts it — and
// the (resource, action) tuple its guard authorizes under.
type feeGuardRoute struct {
	method   string
	suffix   string
	resource string
	action   string
}

// feeGuardRoutes is the fee and billing authorization surface, written down once.
// The ledger-scoped surface attaches it through RegisterFeesV2RoutesToApp. Keeping the
// (resource, verb) tuples in one table is what lets the guard chain and the Huma
// contract be cut from one source (see fees_v2_register.go).
//
// The resource doubles as the ParseUUIDPathParameters label: it is the span-attribute
// name the pre-Huma inline routes used, and the middleware validates every UUID path
// parameter regardless of the label.
//
// The fee calculate endpoint (POST /fees) is absent on purpose: in the unified binary
// fees run in-process via the transaction seam, so only the dry-run estimate is
// exposed over HTTP.
var feeGuardRoutes = []feeGuardRoute{
	{fiber.MethodPost, "/packages", "packages", "post"},
	{fiber.MethodGet, "/packages", "packages", "get"},
	{fiber.MethodGet, "/packages/:id", "packages", "get"},
	{fiber.MethodPatch, "/packages/:id", "packages", "patch"},
	{fiber.MethodDelete, "/packages/:id", "packages", "delete"},

	{fiber.MethodPost, "/estimates", "estimates", "post"},

	{fiber.MethodPost, "/billing-packages", "billing-packages", "post"},
	{fiber.MethodGet, "/billing-packages", "billing-packages", "get"},
	{fiber.MethodGet, "/billing-packages/:id", "billing-packages", "get"},
	{fiber.MethodPatch, "/billing-packages/:id", "billing-packages", "patch"},
	{fiber.MethodDelete, "/billing-packages/:id", "billing-packages", "delete"},

	{fiber.MethodPost, "/billing/calculate", "billing-calculate", "post"},
}

// attachFeeGuards mounts the guard chain of every fee and billing operation under
// chainBase, in Fiber path syntax: auth.Authorize("plugin-fees",resource,verb) + the
// fees-scoped tenant PostAuthMiddlewares (routeOptions) + ParseUUIDPathParameters, as
// MIDDLEWARE ONLY — no terminal handler, and no body binder, because the Huma terminal
// decodes and validates the body imperatively.
func attachFeeGuards(group fiber.Router, auth *middleware.AuthClient, routeOptions *http.ProtectedRouteOptions, chainBase string) {
	for _, route := range feeGuardRoutes {
		chain := protectedFees(auth, route.resource, route.action, routeOptions, http.ParseUUIDPathParameters(route.resource))
		registerRoute(group, route.method, chainBase+route.suffix, chain)
	}
}

// protectedFees is the plugin-fees analogue of protectedMidaz/protectedRouting: it
// builds the auth-attaching Fiber chain under the "plugin-fees" authz appName.
func protectedFees(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(feesApplicationName, resource, action), routeOptions, handlers...)
}
