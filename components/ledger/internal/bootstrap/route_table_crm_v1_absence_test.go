// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"strings"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
)

// mountV1OnlySurface mounts ONLY the /v1 contract with the full production registrar
// set, so a test can inspect what the /v1 version group serves in isolation. It mirrors
// the production mount seam (AssembleHumaContract + huma.NewGroup(api, "/v1") +
// HumaMountDeps.MountV1) that unified-server.go runs, but stops at MountV1 so the /v2
// twins never enter the route set.
//
// NOT parallel: AssembleHumaContract calls problem.Install() and installs a schema
// namer, both process-global.
func mountV1OnlySurface(t *testing.T) *fiber.App {
	t.Helper()

	app := fiber.New()
	auth := fullSurfaceAuthClient()
	routeOptions := fullSurfaceRouteOptions()

	humaDeps := httpin.HumaMountDeps{
		Auth: auth,

		Organization:  &httpin.OrganizationHandler{},
		Ledger:        &httpin.LedgerHandler{},
		Portfolio:     &httpin.PortfolioHandler{},
		Segment:       &httpin.SegmentHandler{},
		Account:       &httpin.AccountHandler{},
		AccountType:   &httpin.AccountTypeHandler{},
		MetadataIndex: &httpin.MetadataIndexHandler{},
		Asset:         &httpin.AssetHandler{},
		AssetRate:     &httpin.AssetRateHandler{},

		Balance:          &httpin.BalanceHandler{},
		Operation:        &httpin.OperationHandler{},
		OperationRoute:   &httpin.OperationRouteHandler{},
		TransactionRoute: &httpin.TransactionRouteHandler{},

		Transaction: &httpin.TransactionHandler{},

		Holder:         &httpin.HolderHandler{},
		Instrument:     &httpin.InstrumentHandler{},
		HolderAccounts: &httpin.HolderAccountsHandler{},
		Encryption:     &httpin.EncryptionHandler{},
		Audit:          &httpin.AuditHandler{},

		FeePackage:       &httpin.PackageHandler{},
		Fee:              &httpin.FeeHandler{},
		BillingPackage:   &httpin.BillingPackageHandler{},
		BillingCalculate: &httpin.BillingCalculateHandler{},

		Composition: &httpin.CompositionHandler{},

		OnboardingOptions:  routeOptions,
		LedgerOptions:      routeOptions,
		TransactionOptions: routeOptions,
		CRMOptions:         routeOptions,
		FeesOptions:        routeOptions,
		CompositionOptions: routeOptions,
	}

	api := httpin.AssembleHumaContract(app, app, openapi.Config{
		Title:   "v1-surface-test",
		Version: "test",
		Servers: []string{"/"},
	})

	fiberGroup := app.Group("/v1")
	humaGroup := huma.NewGroup(api, "/v1")
	humaDeps.MountV1(fiberGroup, humaGroup)

	return app
}

// TestMountV1_OmitsCRMSurface pins that the /v1 version group serves NO CRM surface.
// CRM (holders/instruments/encryption/protection) is v2-only in the unified binary, so
// MountV1 must not mount any of its routes. The composition route
// (/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts) is NOT CRM
// and legitimately stays on v1, so the CRM holder probe anchors on the holders segment
// sitting directly under the organization to avoid matching it.
func TestMountV1_OmitsCRMSurface(t *testing.T) {
	// NOT parallel: AssembleHumaContract mutates process-global huma state.
	unsetDocsGate(t)

	app := mountV1OnlySurface(t)

	var crmRoutes []string

	var hasLedgerRoute bool

	for _, r := range app.GetRoutes(true) {
		p := r.Path

		if strings.Contains(p, "/organizations/:organization_id/ledgers") {
			hasLedgerRoute = true
		}

		isCRM := strings.Contains(p, "/organizations/:organization_id/holders") ||
			strings.Contains(p, "/organizations/:organization_id/instruments") ||
			strings.Contains(p, "/instruments/:instrument_id") ||
			strings.Contains(p, "/encryption/") ||
			strings.Contains(p, "/protection/audit")

		if isCRM {
			crmRoutes = append(crmRoutes, r.Method+" "+p)
		}
	}

	// Non-vacuity guard: prove MountV1 actually mounted its non-CRM surface, so the
	// CRM-absence assertion below cannot pass on an empty mount.
	require.True(t, hasLedgerRoute, "MountV1 must mount its non-CRM /v1 surface (ledgers routes)")

	require.Emptyf(t, crmRoutes,
		"MountV1 must not mount CRM routes — CRM is /v2-only in the unified binary; found:\n%s",
		strings.Join(crmRoutes, "\n"))
}
