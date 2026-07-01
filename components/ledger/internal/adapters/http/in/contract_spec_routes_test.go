// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
)

// buildUnifiedHumaAPI composes the exact registrar set the unified ledger server
// mounts and returns BOTH the Fiber app and the shared huma.API (the latter feeds
// the offline OpenAPI 3.1 spec dump — see openapi_spec_dump_test.go). Registration
// never invokes the handlers, so nil-backed handler structs are safe; this mirrors
// the registrar-composition pattern in fees_routes_test.go / routes_test.go. Routes
// that mount only when their handler is non-nil — the composition GET holder-accounts
// route, and the encryption/audit routes (envelope mode only) — get non-nil
// zero-value handlers so the full surface matches the served contract.
func buildUnifiedHumaAPI() (*fiber.App, huma.API) {
	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	// Mirror the unified server's public infra routes (unified-server.go).
	app.Get("/health", func(c *fiber.Ctx) error { return nil })
	app.Get("/version", func(c *fiber.Ctx) error { return nil })
	app.Get("/readyz", func(c *fiber.Ctx) error { return nil })

	RegisterMetadataRoutesToApp(app, auth, &MetadataIndexHandler{}, nil)
	RegisterOnboardingRoutesToApp(app, auth,
		&AccountHandler{}, &PortfolioHandler{}, &LedgerHandler{},
		&OrganizationHandler{}, &SegmentHandler{}, &AccountTypeHandler{}, nil)

	// Wave-1 Huma-migrated resources (organization, ledger, portfolio, segment,
	// account, account-type, metadata-index, asset, asset-rate) are mounted via the
	// same /v1 group + shared Huma API the unified server's humaMount uses. This block
	// mirrors the production humaMount closure in config.go.
	libProblem.Install()
	apiV1 := app.Group("/v1")
	humaAPI := openapi.New(app, apiV1, openapi.Config{Title: "Midaz Ledger API", Version: "4.0.0", Servers: []string{"/v1"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)
	RegisterOrganizationRoutesToApp(apiV1, humaAPI, auth, &OrganizationHandler{}, nil)
	RegisterLedgerRoutesToApp(apiV1, humaAPI, auth, &LedgerHandler{}, nil)
	RegisterPortfolioRoutesToApp(apiV1, humaAPI, auth, &PortfolioHandler{}, nil)
	RegisterSegmentRoutesToApp(apiV1, humaAPI, auth, &SegmentHandler{}, nil)
	RegisterAccountRoutesToApp(apiV1, humaAPI, auth, &AccountHandler{}, nil)
	RegisterAccountTypeRoutesToApp(apiV1, humaAPI, auth, &AccountTypeHandler{}, nil)
	RegisterMetadataIndexRoutesToApp(apiV1, humaAPI, auth, &MetadataIndexHandler{}, nil)
	RegisterAssetRoutesToApp(apiV1, humaAPI, auth, &AssetHandler{}, nil)
	RegisterAssetRateRoutesToApp(apiV1, humaAPI, auth, &AssetRateHandler{}, nil)

	// Wave-2 Huma-migrated resources (balance, operation-read, transaction-count,
	// operation-route, transaction-route) are mounted via the same /v1 group + shared
	// Huma API the unified server's humaMount uses. The operation PATCH (UpdateOperation,
	// Wave-4 money-write leg) is now ALSO mounted by RegisterOperationRoutesToApp.
	// RegisterTransactionRoutesToApp below mounts only the non-migrated transaction
	// write/DSL routes.
	RegisterBalanceRoutesToApp(apiV1, humaAPI, auth, &BalanceHandler{}, nil)
	RegisterOperationRoutesToApp(apiV1, humaAPI, auth, &OperationHandler{}, nil)
	RegisterCountTransactionRoutesToApp(apiV1, humaAPI, auth, &TransactionHandler{}, nil)
	RegisterOperationRouteRoutesToApp(apiV1, humaAPI, auth, &OperationRouteHandler{}, nil)
	RegisterTransactionRouteRoutesToApp(apiV1, humaAPI, auth, &TransactionRouteHandler{}, nil)

	// Wave-4 (MONEY-WRITE) Huma-migrated transaction ops (json/inflow/outflow/annotation
	// CREATE, commit/cancel/revert STATE, PATCH update, GET-by-id + list) are mounted via
	// the same /v1 group + shared Huma API the unified server's humaMount uses.
	// RegisterTransactionRoutesToApp below mounts only the non-migrated POST
	// /transactions/dsl route.
	RegisterTransactionHumaRoutesToApp(apiV1, humaAPI, auth, &TransactionHandler{}, nil)

	RegisterTransactionRoutesToApp(app, auth,
		&TransactionHandler{}, &OperationHandler{}, &AssetRateHandler{},
		&BalanceHandler{}, &OperationRouteHandler{}, &TransactionRouteHandler{}, nil)

	// Wave-3 (additive) Huma-migrated resources (CRM holders/instruments/holder-
	// accounts/encryption/audit, fees/billing, composition) are mounted via the same
	// /v1 group + shared Huma API the unified server's humaMount uses. The conditional
	// CRM handlers (holder-accounts, encryption, audit) get non-nil zero-value handlers
	// so the FULL surface is registered.
	RegisterCRMRoutesToApp(apiV1, humaAPI, auth,
		&HolderHandler{}, &InstrumentHandler{}, &HolderAccountsHandler{},
		&EncryptionHandler{}, &AuditHandler{}, nil)
	RegisterFeesRoutesToApp(apiV1, humaAPI, auth,
		&PackageHandler{}, &FeeHandler{}, &BillingPackageHandler{}, &BillingCalculateHandler{}, nil)
	RegisterCompositionRoutesToApp(apiV1, humaAPI, auth, &CompositionHandler{}, nil)

	return app, humaAPI
}
