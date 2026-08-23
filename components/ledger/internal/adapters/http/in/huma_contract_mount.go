// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"sort"
	"strings"

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

	// Handlers on the onboarding policy group (see registerOnboardingRoutes).
	Organization  *OrganizationHandler
	Ledger        *LedgerHandler
	Portfolio     *PortfolioHandler
	Segment       *SegmentHandler
	Account       *AccountHandler
	AccountType   *AccountTypeHandler
	MetadataIndex *MetadataIndexHandler
	Asset         *AssetHandler
	AssetRate     *AssetRateHandler

	// Handlers on the money-read + routing policy group (see registerMoneyReadRoutes).
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
//   - account-type uses OnboardingOptions too, authorizing against the "midaz"
//     appName (protectedMidaz).
//   - asset-rate uses TransactionOptions ([authAssertion, WithTenantDB]) — it is
//     MONEY-adjacent (exchange rates), so it shares the transaction tenant chain.
//   - metadata-index uses LedgerOptions ([authAssertion] ONLY, no WithTenantDB).
//     Passing OnboardingOptions here would inject tenant-DB middleware the inline
//     route never had, so LedgerOptions is load-bearing.
func (d HumaMountDeps) MountV1(group fiber.Router, api huma.API) {
	d.registerOnboardingRoutes(group, api)
	d.registerMoneyReadRoutes(group, api)

	// Wave-4 (MONEY-WRITE): the twelve transaction ops (json/inflow/outflow/annotation/
	// block/unblock CREATE, commit/cancel/revert STATE, PATCH update, GET-by-id + list).
	// They carry TransactionOptions ([authAssertion, WithTenantDB]) and authorize
	// against the "midaz" appName (protectedMidaz).
	RegisterTransactionHumaRoutesToApp(group, api, d.Auth, d.Transaction, d.TransactionOptions)
}

// registerOnboardingRoutes mounts the /v1 resources whose guard chain is the
// onboarding one — organization, ledger, portfolio, segment, account, account-type
// and asset all carry OnboardingOptions ([authAssertion, WithTenantDB]) — plus the
// two members whose policy deviates and is therefore load-bearing at the call site:
//
//   - metadata-index carries LedgerOptions ([authAssertion] ONLY, no WithTenantDB).
//     Passing OnboardingOptions here would inject tenant-DB middleware the route
//     never had.
//   - asset-rate carries TransactionOptions because it is MONEY-adjacent (exchange
//     rates), so it shares the transaction tenant chain.
//
// account-type authorizes against the "midaz" appName (protectedMidaz).
func (d HumaMountDeps) registerOnboardingRoutes(group fiber.Router, api huma.API) {
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

// registerMoneyReadRoutes mounts the /v1 resources that share the money-read guard
// chain: balance, operation-read, transaction-count, operation-route and
// transaction-route. Every member carries TransactionOptions ([authAssertion,
// WithTenantDB]) and authorizes against the "midaz" appName (protectedMidaz) — a
// uniform policy, unlike the onboarding group above.
func (d HumaMountDeps) registerMoneyReadRoutes(group fiber.Router, api huma.API) {
	RegisterBalanceRoutesToApp(group, api, d.Auth, d.Balance, d.TransactionOptions)
	RegisterOperationRoutesToApp(group, api, d.Auth, d.Operation, d.TransactionOptions)
	RegisterCountTransactionRoutesToApp(group, api, d.Auth, d.Transaction, d.TransactionOptions)
	RegisterOperationRouteRoutesToApp(group, api, d.Auth, d.OperationRoute, d.TransactionOptions)
	RegisterTransactionRouteRoutesToApp(group, api, d.Auth, d.TransactionRoute, d.TransactionOptions)
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
// with no new policy surface. account-types authorizes against the "midaz" appName
// (protectedMidaz), exactly as on v1 (see registerOnboardingRoutes / registerAccountTypeRoutesToApp).
//
// metadata-index is the LEDGER-AGNOSTIC settings resource: it carries LedgerOptions
// ([authAssertion] ONLY, no WithTenantDB) and authorizes against the "midaz" appName under
// the "settings" resource, exactly as on v1 (see registerOnboardingRoutes / registerMetadataIndexRoutesToApp).
// Passing OnboardingOptions here would inject tenant-DB middleware the route never had, so
// LedgerOptions is load-bearing.
//
// The transaction ops carry TransactionOptions ([authAssertion, WithTenantDB]) and
// authorize against the "midaz" appName (protectedMidaz) — the same auth + tenant
// chain the v1 transaction CREATE ops use, no new policy. RegisterTransactionMirrorV2RoutesToApp
// additionally mirrors the three v1 ops that have no dedicated v2 wire shape — the PATCH update
// and the two reads (get-by-id + list) — as straight v1 twins carrying the same TransactionOptions
// and "midaz" appName. The legacy-create ops (json/inflow/outflow/annotation) are served on /v1
// only; block/unblock create and commit/cancel/revert lifecycle are also absent from that mirror,
// because they are already published as v2 ops by RegisterTransactionV2RoutesToApp. asset-rate is
// served on /v1 only and has no v2 twin.
// balance (resource "balances") and operation (resource "operations") also carry
// TransactionOptions and authorize against the "midaz" appName, matching their v1 twins.
// The transaction-count HEAD op (RegisterCountTransactionV2RoutesToApp) is a straight mirror:
// it carries TransactionOptions and authorizes against the "midaz" appName under the
// "transactions" resource with the "head" verb, exactly as on v1 (see registerMoneyReadRoutes /
// RegisterCountTransactionRoutesToApp).
//
// CRM carries its OWN CRMOptions and authorizes against the "midaz" holders/instruments/
// encryption/protection tuples; it is served ONLY on this /v2 contract. The nil-guards
// (holder-accounts, encryption, audit) leave a route unregistered when its handler is
// nil. The fee/billing ops carry FeesOptions and authorize against the "plugin-fees"
// tuples at ledger scope: the path names the ledger, so a package another ledger owns
// is out of reach. Fees/billing are served ONLY on this /v2 contract. composition
// carries CompositionOptions and authorizes under the "midaz" appName's "accounts"
// resource; it is served ONLY on this /v2 contract (see RegisterCompositionV2RoutesToApp).
//
// operation-routes carry TransactionOptions ([authAssertion, WithTenantDB]) and authorize
// against the "midaz" appName (protectedMidaz), exactly as on v1 (see registerMoneyReadRoutes /
// RegisterOperationRouteRoutesToApp). transaction-routes likewise carry TransactionOptions
// and authorize against the "midaz" appName (protectedMidaz), exactly as on v1 (see
// registerMoneyReadRoutes / RegisterTransactionRouteRoutesToApp).
func (d HumaMountDeps) MountV2(group fiber.Router, api huma.API) {
	RegisterOrganizationV2RoutesToApp(group, api, d.Auth, d.Organization, d.OnboardingOptions)
	RegisterLedgerV2RoutesToApp(group, api, d.Auth, d.Ledger, d.OnboardingOptions)
	RegisterPortfolioV2RoutesToApp(group, api, d.Auth, d.Portfolio, d.OnboardingOptions)
	RegisterSegmentV2RoutesToApp(group, api, d.Auth, d.Segment, d.OnboardingOptions)
	RegisterAccountV2RoutesToApp(group, api, d.Auth, d.Account, d.OnboardingOptions)
	RegisterAccountTypeV2RoutesToApp(group, api, d.Auth, d.AccountType, d.OnboardingOptions)
	RegisterMetadataIndexV2RoutesToApp(group, api, d.Auth, d.MetadataIndex, d.LedgerOptions)
	RegisterAssetV2RoutesToApp(group, api, d.Auth, d.Asset, d.OnboardingOptions)
	RegisterTransactionV2RoutesToApp(group, api, d.Auth, d.Transaction, d.TransactionOptions)
	RegisterTransactionMirrorV2RoutesToApp(group, api, d.Auth, d.Transaction, d.TransactionOptions)
	RegisterCountTransactionV2RoutesToApp(group, api, d.Auth, d.Transaction, d.TransactionOptions)
	RegisterBalanceV2RoutesToApp(group, api, d.Auth, d.Balance, d.TransactionOptions)
	RegisterOperationV2RoutesToApp(group, api, d.Auth, d.Operation, d.TransactionOptions)
	RegisterCRMV2RoutesToApp(group, api, d.Auth, d.Holder, d.Instrument, d.HolderAccounts, d.Encryption, d.Audit, d.CRMOptions)
	RegisterFeesV2RoutesToApp(group, api, d.Auth, d.FeePackage, d.Fee, d.BillingPackage, d.BillingCalculate, d.FeesOptions)
	RegisterCompositionV2RoutesToApp(group, api, d.Auth, d.Composition, d.CompositionOptions)
	RegisterOperationRouteV2RoutesToApp(group, api, d.Auth, d.OperationRoute, d.TransactionOptions)
	RegisterTransactionRouteV2RoutesToApp(group, api, d.Auth, d.TransactionRoute, d.TransactionOptions)
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
//  5. openapi.DeclareBearerAuth — SPEC-ONLY security scheme metadata, so the
//     per-operation Security references resolve in the generated spec. BearerAuth is
//     the only scheme the ledger accepts; runtime auth is the Fiber guard chain the
//     mount closure attaches, which authorizes a JWT bearer token and nothing else.
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

	return api
}

// MarkV1OperationsDeprecated flags every operation whose path key is under "/v1/"
// as deprecated on the assembled document, leaving "/v2/" untouched. Run it AFTER
// the last huma.Register and BEFORE the spec is snapshotted so the served spec and
// the committed dump carry the flag identically.
func MarkV1OperationsDeprecated(api huma.API) {
	for key, item := range api.OpenAPI().Paths {
		if !strings.HasPrefix(key, "/v1/") {
			continue
		}

		for _, op := range operationsOf(item) {
			op.Deprecated = true
		}
	}
}

// operationsOf returns every declared operation on a PathItem, in a fixed order, so a
// caller can walk the whole served surface without re-listing the eight method fields at
// each call site. Nil methods are filtered out, and a nil item yields nil.
func operationsOf(item *huma.PathItem) []*huma.Operation {
	if item == nil {
		return nil
	}

	candidates := []*huma.Operation{
		item.Get, item.Put, item.Post, item.Delete,
		item.Options, item.Head, item.Patch, item.Trace,
	}

	ops := make([]*huma.Operation, 0, len(candidates))

	for _, op := range candidates {
		if op != nil {
			ops = append(ops, op)
		}
	}

	return ops
}

// v1TagSuffix and v2TagSuffix are appended to each operation's resource tag so a
// v1 op and its v2 twin, which share one resource tag, become distinct tags that
// Scalar can group under separate sidebar sections.
const (
	v1TagSuffix = " (v1)"
	v2TagSuffix = " (v2)"

	v1TagGroupName = "V1 (deprecated)"
	v2TagGroupName = "V2"
)

// ApplyVersionTagGroups splits the sidebar into two version-scoped sections in the
// rendered docs. It rewrites every operation's resource tag to a version-suffixed
// tag ("Organizations" -> "Organizations (v1)" / "Organizations (v2)"), declares
// the resulting tags at the document root, and sets the x-tagGroups vendor
// extension with a "V1 (deprecated)" group (all v1 tags) followed by a "V2" group
// (all v2 tags).
//
// Run it AFTER MarkV1OperationsDeprecated and BEFORE the spec is snapshotted so the
// served spec and the committed dump carry the tags identically. Output is
// deterministic: the root Tags slice and every group's tag list are sorted, so the
// byte-reproducible golden dump does not flap on Go's randomized map iteration.
func ApplyVersionTagGroups(api huma.API) {
	v1Set := map[string]struct{}{}
	v2Set := map[string]struct{}{}

	for key, item := range api.OpenAPI().Paths {
		var (
			suffix string
			target map[string]struct{}
		)

		switch {
		case strings.HasPrefix(key, "/v1/"):
			suffix, target = v1TagSuffix, v1Set
		case strings.HasPrefix(key, "/v2/"):
			suffix, target = v2TagSuffix, v2Set
		default:
			continue
		}

		for _, op := range operationsOf(item) {
			for i, tag := range op.Tags {
				suffixed := tag + suffix
				op.Tags[i] = suffixed
				target[suffixed] = struct{}{}
			}
		}
	}

	v1Tags := sortedKeys(v1Set)
	v2Tags := sortedKeys(v2Set)

	rootTags := make([]*huma.Tag, 0, len(v1Tags)+len(v2Tags))
	for _, name := range append(append([]string{}, v1Tags...), v2Tags...) {
		rootTags = append(rootTags, &huma.Tag{Name: name})
	}

	sort.Slice(rootTags, func(i, j int) bool { return rootTags[i].Name < rootTags[j].Name })
	api.OpenAPI().Tags = rootTags

	if api.OpenAPI().Extensions == nil {
		api.OpenAPI().Extensions = map[string]any{}
	}

	api.OpenAPI().Extensions["x-tagGroups"] = []map[string]any{
		{"name": v1TagGroupName, "tags": v1Tags},
		{"name": v2TagGroupName, "tags": v2Tags},
	}
}

// sortedKeys returns the keys of set as a sorted slice, or an empty (non-nil)
// slice so the resulting group renders as [] rather than null.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
