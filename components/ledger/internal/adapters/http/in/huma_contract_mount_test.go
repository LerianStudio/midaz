// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"sort"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// referencedSecuritySchemes collects the DISTINCT security-scheme names every
// operation (and the document-level Security) references across the whole
// document. A name here that is absent from Components.SecuritySchemes is a
// dangling reference: the served spec points at a scheme it never declares.
func referencedSecuritySchemes(doc *huma.OpenAPI) []string {
	seen := map[string]bool{}

	collect := func(requirements []map[string][]string) {
		for _, requirement := range requirements {
			for name := range requirement {
				seen[name] = true
			}
		}
	}

	collect(doc.Security)

	for _, item := range doc.Paths {
		for _, op := range operationsOf(item) {
			collect(op.Security)
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// declaredSecuritySchemes returns the sorted names Components.SecuritySchemes declares.
func declaredSecuritySchemes(doc *huma.OpenAPI) []string {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return nil
	}

	names := make([]string, 0, len(doc.Components.SecuritySchemes))
	for name := range doc.Components.SecuritySchemes {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// TestUnifiedContractDeclaresSecuritySchemes proves the harness-produced ledger
// contract carries BearerAuth — the only scheme the ledger accepts — and that every
// per-operation security reference resolves against it: the property the committed
// dump must describe. The single document carries both the /v1 and /v2 operations, so
// one contract built through the dump seam covers the whole surface; a contract missing
// the scheme is a dump with dangling security references.
func TestUnifiedContractDeclaresSecuritySchemes(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	require.Equal(t, []string{"BearerAuth"}, declaredSecuritySchemes(doc),
		"contract must declare exactly the BearerAuth security scheme")

	for _, name := range referencedSecuritySchemes(doc) {
		require.Containsf(t, doc.Components.SecuritySchemes, name,
			"operation references security scheme %q that Components.SecuritySchemes does not declare (dangling reference)", name)
	}
}

// crmV2Org is the /v2-mounted organization scope every CRM route hangs off, in Fiber
// path syntax.
const crmV2Org = "/v2/organizations/:organization_id"

// humaPathOperations projects a Huma path item onto the HTTP methods a route may carry,
// keyed by the Fiber method constant. A nil value means the method is not published on
// that path.
func humaPathOperations(item *huma.PathItem) map[string]*huma.Operation {
	return map[string]*huma.Operation{
		fiber.MethodGet:    item.Get,
		fiber.MethodPost:   item.Post,
		fiber.MethodPut:    item.Put,
		fiber.MethodPatch:  item.Patch,
		fiber.MethodDelete: item.Delete,
		fiber.MethodHead:   item.Head,
	}
}

// TestCRMV2RoutesRespectNilGuards asserts the conditional CRM seams stay unregistered
// on the /v2 contract when their handler is nil — the complement of the full-handler
// surface TestCRMCompositionSurfaceMountedOnGroup pins. Absence is checked on BOTH
// surfaces a registrar writes to: the Fiber router (nothing is served) and the Huma
// document (nothing is published). Encryption and protection are nil outside envelope
// mode, and holder-accounts is nil without a ledger account-query backing.
//
// It drives the same per-resource registrars MountV2 calls, so the guard under test is
// the one production runs.
func TestCRMV2RoutesRespectNilGuards(t *testing.T) {
	// NOT parallel: huma registration mutates process-global state.
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	libProblem.Install()
	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-v2-nilguard", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	// The unconditional pair mounts; the three conditional handlers are nil, so
	// holder-accounts + encryption + audit must stay absent.
	RegisterHolderV2RoutesToApp(apiV2, hAPI, auth, &HolderHandler{}, nil)
	RegisterInstrumentV2RoutesToApp(apiV2, hAPI, auth, &InstrumentHandler{}, nil)
	RegisterHolderAccountsV2RoutesToApp(apiV2, hAPI, auth, nil, nil)
	RegisterEncryptionV2RoutesToApp(apiV2, hAPI, auth, nil, nil)
	RegisterAuditV2RoutesToApp(apiV2, hAPI, auth, nil, nil)

	fiberRoutes := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		fiberRoutes[r.Method+":"+r.Path] = true
	}

	humaOps := make(map[string]bool)

	for path, item := range hAPI.OpenAPI().Paths {
		for method, op := range humaPathOperations(item) {
			if op != nil {
				humaOps[method+":"+path] = true
			}
		}
	}

	// The unconditional surface MUST still mount, so an all-absent document cannot
	// satisfy the assertions below vacuously.
	unconditional := []struct {
		fiberRoute string
		humaOp     string
	}{
		{fiber.MethodPost + ":" + crmV2Org + "/holders", "POST:/organizations/{organization_id}/holders"},
		{fiber.MethodGet + ":" + crmV2Org + "/instruments", "GET:/organizations/{organization_id}/instruments"},
	}

	for _, u := range unconditional {
		assert.Truef(t, fiberRoutes[u.fiberRoute], "%s must be served on /v2 regardless of the nil guards", u.fiberRoute)
		assert.Truef(t, humaOps[u.humaOp], "%s must be published on the /v2 contract regardless of the nil guards", u.humaOp)
	}

	guarded := []struct {
		name       string
		nilHandler string
		fiberRoute string
		humaOp     string
	}{
		{
			name:       "holder_accounts_absent_when_handler_nil",
			nilHandler: "HolderAccounts",
			fiberRoute: fiber.MethodGet + ":" + crmV2Org + "/holders/:id/accounts",
			humaOp:     "GET:/organizations/{organization_id}/holders/{id}/accounts",
		},
		{
			name:       "encryption_provision_absent_when_handler_nil",
			nilHandler: "Encryption",
			fiberRoute: fiber.MethodPost + ":" + crmV2Org + "/encryption/provision",
			humaOp:     "POST:/organizations/{organization_id}/encryption/provision",
		},
		{
			name:       "encryption_status_absent_when_handler_nil",
			nilHandler: "Encryption",
			fiberRoute: fiber.MethodGet + ":" + crmV2Org + "/encryption/status",
			humaOp:     "GET:/organizations/{organization_id}/encryption/status",
		},
		{
			name:       "protection_audit_absent_when_handler_nil",
			nilHandler: "Audit",
			fiberRoute: fiber.MethodGet + ":" + crmV2Org + "/protection/audit",
			humaOp:     "GET:/organizations/{organization_id}/protection/audit",
		},
	}

	for _, tt := range guarded {
		t.Run(tt.name, func(t *testing.T) {
			assert.Falsef(t, fiberRoutes[tt.fiberRoute],
				"%s must NOT be served on /v2 when %s is nil", tt.fiberRoute, tt.nilHandler)
			assert.Falsef(t, humaOps[tt.humaOp],
				"%s must NOT be published on the /v2 contract when %s is nil", tt.humaOp, tt.nilHandler)
		})
	}
}
