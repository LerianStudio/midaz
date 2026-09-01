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

		HolderAccountsOptions: routeOptions,
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
// MountV1 must not mount any of its routes. The CRM holder probe anchors on the holders
// segment sitting directly under the organization so it targets only the org-scoped CRM
// holder routes and does not depend on the shape of any deeper holders path.
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

// TestMountV1_OmitsCompositionSurface pins that the /v1 version group serves NO
// composition route. The holder-account composition orchestration
// (POST /organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts) is
// /v2-only in the unified binary, so MountV1 must not mount it. The probe anchors on the
// POST method plus the "/holders/:id/accounts" tail so it cannot match the CRM
// holder-accounts read (GET .../holders/:id/accounts), which is /v2-only anyway.
func TestMountV1_OmitsCompositionSurface(t *testing.T) {
	// NOT parallel: AssembleHumaContract mutates process-global huma state.
	unsetDocsGate(t)

	app := mountV1OnlySurface(t)

	var compositionRoutes []string

	var hasLedgerRoute bool

	for _, r := range app.GetRoutes(true) {
		p := r.Path

		if strings.Contains(p, "/organizations/:organization_id/ledgers") {
			hasLedgerRoute = true
		}

		if r.Method == fiber.MethodPost && strings.Contains(p, "/holders/:id/accounts") {
			compositionRoutes = append(compositionRoutes, r.Method+" "+p)
		}
	}

	// Non-vacuity guard: prove MountV1 actually mounted its non-composition surface, so
	// the absence assertion below cannot pass on an empty mount.
	require.True(t, hasLedgerRoute, "MountV1 must mount its non-composition /v1 surface (ledgers routes)")

	require.Emptyf(t, compositionRoutes,
		"MountV1 must not mount the composition route — composition is /v2-only in the unified binary; found:\n%s",
		strings.Join(compositionRoutes, "\n"))
}

// TestMountV1_OmitsFeesSurface pins that the /v1 version group serves NO fee or billing
// surface. Fees/billing is org-scoped on /v1 but ledger-scoped on /v2; in the unified
// binary the surface is served under /v2 only, so MountV1 must not mount any of its
// routes. The org-scoped fee resources hang directly off the organization
// (/organizations/:organization_id/packages|estimates|billing-packages|billing/calculate),
// so the probe anchors there. Composition's holder-accounts route sits under a ledger
// (/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts) and CRM/v2
// live on /v2, so neither can false-match.
func TestMountV1_OmitsFeesSurface(t *testing.T) {
	// NOT parallel: AssembleHumaContract mutates process-global huma state.
	unsetDocsGate(t)

	app := mountV1OnlySurface(t)

	var feeRoutes []string

	var hasLedgerRoute bool

	for _, r := range app.GetRoutes(true) {
		p := r.Path

		if strings.Contains(p, "/organizations/:organization_id/ledgers") {
			hasLedgerRoute = true
		}

		orgScope := "/organizations/:organization_id"

		isFee := strings.Contains(p, orgScope+"/packages") ||
			strings.Contains(p, orgScope+"/estimates") ||
			strings.Contains(p, orgScope+"/billing-packages") ||
			strings.Contains(p, orgScope+"/billing/calculate")

		if isFee {
			feeRoutes = append(feeRoutes, r.Method+" "+p)
		}
	}

	// Non-vacuity guard: prove MountV1 actually mounted its non-fee surface, so the
	// fee-absence assertion below cannot pass on an empty mount.
	require.True(t, hasLedgerRoute, "MountV1 must mount its non-fee /v1 surface (ledgers routes)")

	require.Emptyf(t, feeRoutes,
		"MountV1 must not mount fee/billing routes — fees are /v2-only in the unified binary; found:\n%s",
		strings.Join(feeRoutes, "\n"))
}
