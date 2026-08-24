// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterEncryptionRoutes registers the two encryption operations on the
// given Huma API. It is the per-file seam the unified server calls (conditionally,
// only in envelope encryption mode — mirroring the nil guard in
// RegisterEncryptionV2RoutesToApp); the auth ("midaz","encryption",verb) + tenant +
// ParseUUIDPathParameters("organization") middleware chain is attached on the
// versioned Fiber group BEFORE the Huma terminal, not here. Paths are GROUP-RELATIVE
// (see asset_handler.go's RegisterAssetRoutes header for the rationale).
//
// opSuffix is appended to every operation ID — see v2OpSuffix.
func RegisterEncryptionRoutes(api huma.API, h *EncryptionHandler, opSuffix string) {
	const (
		provisionPath = "/organizations/{organization_id}/encryption/provision"
		statusPath    = "/organizations/{organization_id}/encryption/status"
		tag           = "Encryption"
	)

	huma.Register(api, huma.Operation{
		OperationID: "provisionEncryption" + opSuffix,
		Method:      http.MethodPost,
		Path:        provisionPath,
		Summary:     "Provision an Organization for Envelope Encryption",
		Tags:        []string{tag},
		Security:    secEncryptionBearer,
		// Body validated imperatively (http.DecodeAndValidate) — see file header.
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, h.Provision)
	attachTypedRequestBody[mmodel.ProvisionEncryptionInput](api, "provisionEncryption"+opSuffix)

	huma.Register(api, huma.Operation{
		OperationID: "getProvisioningStatus" + opSuffix,
		Method:      http.MethodGet,
		Path:        statusPath,
		Summary:     "Get Provisioning Status",
		Tags:        []string{tag},
		Security:    secEncryptionBearer,
	}, h.GetProvisioningStatus)
}

// RegisterEncryptionV2RoutesToApp wires the encryption provisioning surface onto the /v2
// contract, which is the ONLY version group that serves it.
//
// h is non-nil only in envelope encryption mode (KMS_VENDOR=hashicorp-vault); when nil
// the routes stay unregistered, matching the legacy-mode posture where no KMS
// provisioning surface exists.
func RegisterEncryptionV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *EncryptionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	if h == nil {
		return
	}

	registerEncryptionRoutesToApp(group, api, auth, h, routeOptions, v2OpSuffix)
}

// registerEncryptionRoutesToApp is the single description of the encryption route
// surface. For both ops it attaches auth.Authorize("midaz","encryption",verb) + the
// CRM-scoped tenant PostAuthMiddlewares + ParseUUIDPathParameters("organization") as
// MIDDLEWARE ONLY on the versioned group, then registers the Huma terminals on the same
// group's Huma API.
func registerEncryptionRoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *EncryptionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions, opSuffix string) {
	const (
		provisionPath = "/organizations/:organization_id/encryption/provision"
		statusPath    = "/organizations/:organization_id/encryption/status"
	)

	orgParse := pkgHTTP.ParseUUIDPathParameters("organization")

	routePost(group, provisionPath, protectedMidaz(auth, "encryption", "post", routeOptions, orgParse))
	routeGet(group, statusPath, protectedMidaz(auth, "encryption", "get", routeOptions, orgParse))

	RegisterEncryptionRoutes(api, h, opSuffix)
}
