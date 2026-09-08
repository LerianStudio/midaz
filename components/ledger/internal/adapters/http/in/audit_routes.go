// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterAuditRoutes registers the protection-audit operation on the
// given Huma API. It is the per-file seam the unified server calls (conditionally,
// only in envelope encryption mode — mirroring the nil guard
// in RegisterAuditV2RoutesToApp); the auth ("midaz","protection","get") + tenant +
// ParseUUIDPathParameters("organization") middleware chain is attached on the
// versioned Fiber group BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE
// (see asset_handler.go's RegisterAssetRoutes header for the rationale).
//
// opSuffix is appended to the operation ID — see v2OpSuffix.
func RegisterAuditRoutes(api huma.API, h *AuditHandler, opSuffix string) {
	huma.Register(api, huma.Operation{
		OperationID: "getAuditEvents" + opSuffix,
		Method:      http.MethodGet,
		Path:        "/organizations/{organization_id}/protection/audit",
		Summary:     "List Protection Audit Events",
		Tags:        []string{"Protection"},
		Security:    secAuditBearer,
	}, h.GetAuditEvents)
}

// RegisterAuditV2RoutesToApp wires the protection-audit surface onto the /v2 contract,
// which is the ONLY version group that serves it.
//
// h is non-nil only in envelope encryption mode (KMS_VENDOR=hashicorp-vault); when nil
// the route stays unregistered, matching the legacy-mode posture where no protection
// audit surface exists.
func RegisterAuditV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AuditHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	if h == nil {
		return
	}

	registerAuditRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerAuditRoutesToApp is the single description of the protection-audit route
// surface. It attaches auth.Authorize("midaz","protection","get") + the CRM-scoped tenant
// PostAuthMiddlewares + ParseUUIDPathParameters("organization") as MIDDLEWARE ONLY on the
// versioned group, then registers the Huma terminal on the same group's Huma API.
func registerAuditRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *AuditHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const auditPath = "/organizations/:organization_id/protection/audit"

	routeGet(group, auditPath, protectedMidaz(auth, "protection", "get", routeOptions, pkgHTTP.ParseUUIDPathParameters("organization")))

	RegisterAuditRoutes(api, h, opSuffix)
}
