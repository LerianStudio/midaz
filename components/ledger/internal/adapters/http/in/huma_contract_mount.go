// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// HumaMountDeps is the single source of truth for the ledger's Huma mount list. It
// carries, by name, the auth client, every handler the registrars consume, and the
// six route-scoped ProtectedRouteOptions the guard chains attach. Production and
// every offline harness build one of these and mount through MountV1/MountV2, so a
// registrar added to the mount reaches all of them at once instead of drifting
// across four hand-maintained copies.
//
// Handlers and options are held by NAME rather than by position: the CRM and Fees
// options both write the generic tenant-context key over different Mongo managers,
// so a positional swap would resolve CRM holder PII against the fees tenant
// database. Named fields remove that whole class of swap at the call site.
type HumaMountDeps struct {
	Auth *middleware.AuthClient

	// Onboarding + Wave-1 handlers.
	Organization  *OrganizationHandler
	Ledger        *LedgerHandler
	Portfolio     *PortfolioHandler
	Segment       *SegmentHandler
	Account       *AccountHandler
	AccountType   *AccountTypeHandler
	MetadataIndex *MetadataIndexHandler
	Asset         *AssetHandler
	AssetRate     *AssetRateHandler

	// Wave-2 money-read + routing handlers.
	Balance          *BalanceHandler
	Operation        *OperationHandler
	OperationRoute   *OperationRouteHandler
	TransactionRoute *TransactionRouteHandler

	// Transaction handler: money-write ops, transaction count, and the /v2 create.
	Transaction *TransactionHandler

	// Wave-3 CRM handlers. HolderAccounts, Encryption and Audit may be nil; the CRM
	// registrar mounts neither the Fiber guard chain nor the Huma terminal for a nil
	// handler, matching the pre-Huma nil-guard posture.
	Holder         *HolderHandler
	Instrument     *InstrumentHandler
	HolderAccounts *HolderAccountsHandler
	Encryption     *EncryptionHandler
	Audit          *AuditHandler

	// Wave-3 fee/billing handlers.
	FeePackage       *PackageHandler
	Fee              *FeeHandler
	BillingPackage   *BillingPackageHandler
	BillingCalculate *BillingCalculateHandler

	// Wave-3 composition handler.
	Composition *CompositionHandler

	// Route-scoped protected options, one instance per role. In multi-tenant mode
	// buildUnifiedRouteSetup builds six distinct instances drawn from four tenant
	// middlewares; in single-tenant mode every field is nil.
	OnboardingOptions  *pkgHTTP.ProtectedRouteOptions
	LedgerOptions      *pkgHTTP.ProtectedRouteOptions
	TransactionOptions *pkgHTTP.ProtectedRouteOptions
	CRMOptions         *pkgHTTP.ProtectedRouteOptions
	FeesOptions        *pkgHTTP.ProtectedRouteOptions
	CompositionOptions *pkgHTTP.ProtectedRouteOptions
}

// MountV1 registers the /v1 Huma terminals + Fiber auth/tenant chain on the shared
// v1 contract. Each RegisterXxxRoutesToApp reproduces the same (resource, verb)
// authz tuple and the same route options the pre-Huma inline route used:
//   - organization/ledger/portfolio/segment/account/asset use OnboardingOptions
//     ([authAssertion, WithTenantDB]).
//   - account-type uses OnboardingOptions too, but authorizes against the "routing"
//     appName (protectedRouting).
//   - asset-rate uses TransactionOptions ([authAssertion, WithTenantDB]) — it is
//     MONEY-adjacent (exchange rates), so it shares the transaction tenant chain.
//   - metadata-index uses LedgerOptions ([authAssertion] ONLY, no WithTenantDB).
//     Passing OnboardingOptions here would inject tenant-DB middleware the inline
//     route never had, so LedgerOptions is load-bearing.
func (d HumaMountDeps) MountV1(group fiber.Router, api huma.API) {
	d.registerWave1(group, api)
	d.registerWave2(group, api)

	// Wave-4 (MONEY-WRITE): the twelve transaction ops (json/inflow/outflow/annotation/
	// block/unblock CREATE, commit/cancel/revert STATE, PATCH update, GET-by-id + list).
	// They carry TransactionOptions ([authAssertion, WithTenantDB]) and authorize
	// against the "midaz" appName (protectedMidaz).
	RegisterTransactionHumaRoutesToApp(group, api, d.Auth, d.Transaction, d.TransactionOptions)

	d.registerWave3(group, api)
}

// registerWave1 mounts organization, ledger, portfolio, segment, account,
// account-type, metadata-index, asset and asset-rate. See MountV1 for the
// per-registrar options rationale.
func (d HumaMountDeps) registerWave1(group fiber.Router, api huma.API) {
	RegisterOrganizationRoutesToApp(group, api, d.Auth, d.Organization, d.OnboardingOptions)
	RegisterLedgerRoutesToApp(group, api, d.Auth, d.Ledger, d.OnboardingOptions)
	RegisterPortfolioRoutesToApp(group, api, d.Auth, d.Portfolio, d.OnboardingOptions)
	RegisterSegmentRoutesToApp(group, api, d.Auth, d.Segment, d.OnboardingOptions)
	RegisterAccountRoutesToApp(group, api, d.Auth, d.Account, d.OnboardingOptions)
	RegisterAccountTypeRoutesToApp(group, api, d.Auth, d.AccountType, d.OnboardingOptions)
	RegisterMetadataIndexRoutesToApp(group, api, d.Auth, d.MetadataIndex, d.LedgerOptions)
	RegisterAssetRoutesToApp(group, api, d.Auth, d.Asset, d.OnboardingOptions)
	RegisterAssetRateRoutesToApp(group, api, d.Auth, d.AssetRate, d.TransactionOptions)
}

// registerWave2 mounts balance, operation-read, transaction-count, operation-route
// and transaction-route. All carry TransactionOptions ([authAssertion,
// WithTenantDB]). balance/operation/count authorize against the "midaz" appName
// (protectedMidaz); operation-route/transaction-route authorize against the
// "routing" appName (protectedRouting).
func (d HumaMountDeps) registerWave2(group fiber.Router, api huma.API) {
	RegisterBalanceRoutesToApp(group, api, d.Auth, d.Balance, d.TransactionOptions)
	RegisterOperationRoutesToApp(group, api, d.Auth, d.Operation, d.TransactionOptions)
	RegisterCountTransactionRoutesToApp(group, api, d.Auth, d.Transaction, d.TransactionOptions)
	RegisterOperationRouteRoutesToApp(group, api, d.Auth, d.OperationRoute, d.TransactionOptions)
	RegisterTransactionRouteRoutesToApp(group, api, d.Auth, d.TransactionRoute, d.TransactionOptions)
}

// registerWave3 mounts the additive resources: CRM (holders/instruments/holder-
// accounts/encryption/audit) under "midaz", fees/billing under "plugin-fees", and
// composition under "midaz". Each carries its OWN route-scoped tenant options so the
// CRM/fee/composition tenant Mongo never overwrites the onboarding/transaction
// tenant DB. The nil-guards (holder-accounts, encryption, audit) are preserved
// inside RegisterCRMRoutesToApp: a nil handler mounts neither the Fiber auth chain
// nor the Huma terminal.
func (d HumaMountDeps) registerWave3(group fiber.Router, api huma.API) {
	RegisterCRMRoutesToApp(group, api, d.Auth, d.Holder, d.Instrument, d.HolderAccounts, d.Encryption, d.Audit, d.CRMOptions)
	RegisterFeesRoutesToApp(group, api, d.Auth, d.FeePackage, d.Fee, d.BillingPackage, d.BillingCalculate, d.FeesOptions)
	RegisterCompositionRoutesToApp(group, api, d.Auth, d.Composition, d.CompositionOptions)
}

// MountV2 registers the /v2 Huma terminals + Fiber auth/tenant chain on the /v2 version
// group of the shared Huma contract. Both version groups share one huma.API and one
// component registry, so a NAMED-type schema-name collision between a v1 type and a v2
// type is a deliberate boot panic in mapRegistry.Schema — distinct wire shapes MUST
// carry distinct schema names.
//
// The seven onboarding families — organizations, ledgers, portfolios, segments,
// accounts, account-types, assets — carry OnboardingOptions and reuse the same authz
// tuples and tenant chain as their v1 twins; they are straight mirrors, additive over v1,
// with no new policy surface. account-types is the one nuance: it authorizes against the
// "routing" appName (protectedRouting), NOT "midaz", exactly as on v1 (see
// registerWave1 / registerAccountTypeRoutesToApp).
//
// metadata-index is the LEDGER-AGNOSTIC settings resource: it carries LedgerOptions
// ([authAssertion] ONLY, no WithTenantDB) and authorizes against the "midaz" appName under
// the "settings" resource, exactly as on v1 (see registerWave1 / registerMetadataIndexRoutesToApp).
// Passing OnboardingOptions here would inject tenant-DB middleware the route never had, so
// LedgerOptions is load-bearing.
//
// The transaction ops carry TransactionOptions ([authAssertion, WithTenantDB]) and
// authorize against the "midaz" appName (protectedMidaz) — the same auth + tenant
// chain the v1 transaction CREATE ops use, no new policy. asset-rate is MONEY-adjacent
// (exchange rates): it likewise carries TransactionOptions and authorizes against the
// "midaz" appName, exactly as on v1 (see registerWave1 / registerAssetRateRoutesToApp).
//
// CRM carries its OWN CRMOptions and authorizes against the same "midaz" tuples the
// v1 CRM routes use; the nil-guards (holder-accounts, encryption, audit) hold here
// exactly as on v1. The fee/billing ops carry FeesOptions and authorize against the
// same "plugin-fees" tuples as v1 — they differ from v1 in scope only: the path
// names the ledger, so a package another ledger owns is out of reach.
func (d HumaMountDeps) MountV2(group fiber.Router, api huma.API) {
	RegisterOrganizationV2RoutesToApp(group, api, d.Auth, d.Organization, d.OnboardingOptions)
	RegisterLedgerV2RoutesToApp(group, api, d.Auth, d.Ledger, d.OnboardingOptions)
	RegisterPortfolioV2RoutesToApp(group, api, d.Auth, d.Portfolio, d.OnboardingOptions)
	RegisterSegmentV2RoutesToApp(group, api, d.Auth, d.Segment, d.OnboardingOptions)
	RegisterAccountV2RoutesToApp(group, api, d.Auth, d.Account, d.OnboardingOptions)
	RegisterAccountTypeV2RoutesToApp(group, api, d.Auth, d.AccountType, d.OnboardingOptions)
	RegisterMetadataIndexV2RoutesToApp(group, api, d.Auth, d.MetadataIndex, d.LedgerOptions)
	RegisterAssetV2RoutesToApp(group, api, d.Auth, d.Asset, d.OnboardingOptions)
	RegisterAssetRateV2RoutesToApp(group, api, d.Auth, d.AssetRate, d.TransactionOptions)
	RegisterTransactionV2RoutesToApp(group, api, d.Auth, d.Transaction, d.TransactionOptions)
	RegisterBalanceV2RoutesToApp(group, api, d.Auth, d.Balance, d.TransactionOptions)
	RegisterOperationV2RoutesToApp(group, api, d.Auth, d.Operation, d.TransactionOptions)
	RegisterCRMV2RoutesToApp(group, api, d.Auth, d.Holder, d.Instrument, d.HolderAccounts, d.Encryption, d.Audit, d.CRMOptions)
	RegisterFeesV2RoutesToApp(group, api, d.Auth, d.FeePackage, d.Fee, d.BillingPackage, d.BillingCalculate, d.FeesOptions)
}

// AssembleHumaContract builds one independent Huma contract instance on group and
// runs the INVARIANT scaffolding every version shares, in its load-bearing order:
//
//  1. problem.Install() — idempotent RFC 9457 model override; MUST precede any
//     huma.Register so framework errors render as problem+json.
//  2. InstallHumaFrameworkErrors() — maps Huma's own framework errors onto the
//     canonical Midaz envelope; order relative to problem.Install does not matter.
//  3. openapi.New — creates the Huma document bound to the caller's Fiber group.
//  4. InstallLedgerSchemaNamer — EXACTLY ONCE: it REPLACES Components.Schemas with a
//     fresh registry, so a second call would discard every schema registered after
//     the first. It must run after openapi.New and before any huma.Register (the
//     mount), because the registry namer is captured lazily on first registration.
//  5. openapi.DeclareBearerAuth + the ApiKeyAuth block — SPEC-ONLY security scheme
//     metadata, so the per-operation Security references resolve in the generated
//     spec. Runtime auth stays the Fiber guard chain the mount closure attaches.
//
// It does NOT serve the spec: openapi.ServeSpec is bootstrap exposure policy, gated
// on openAPIDocsEnabled(), and stays with the caller. The caller creates the Fiber
// group (it needs it for the mount) and runs the mount AFTER this returns.
func AssembleHumaContract(app *fiber.App, group fiber.Router, cfg openapi.Config) huma.API {
	problem.Install()
	pkgHTTP.InstallHumaFrameworkErrors()

	api := openapi.New(app, group, cfg)

	pkgHTTP.InstallLedgerSchemaNamer(api)

	openapi.DeclareBearerAuth(api)

	components := api.OpenAPI().Components
	if components.SecuritySchemes == nil {
		components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}

	components.SecuritySchemes["ApiKeyAuth"] = &huma.SecurityScheme{
		Type:        "apiKey",
		In:          "header",
		Name:        "X-API-Key",
		Description: "Static API key presented in the X-API-Key header.",
	}

	return api
}
