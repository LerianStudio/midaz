// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// Huma's DefaultSchemaNamer keys the shared schema registry by the BARE Go type
// name, ignoring the package (see huma registry.go). When two distinct types share
// a name across packages, the second huma.Register panics with "duplicate name".
// Its own doc says: "if you plan to use types with the same name from different
// packages, you should implement your own namer function."
//
// The ledger's shared Huma API registers many resources on ONE registry, so any
// cross-package name clash among their response bodies is fatal at startup. The
// clash today is the operation-read ops: they emit operation.Operation, which nests
// operation.{Balance,Status,Amount} — every one of which collides with an
// identically-named mmodel type the balance/other ops already own on the registry.
// The mmodel types own the public bare names (they are the ones in the committed
// swagger contract); the operation package's types must be qualified to avoid the
// panic.
//
// InstallLedgerSchemaNamer swaps in a namer that returns sharedSchemaNamer's name
// (DefaultSchemaNamer, plus the org-wide problem.Detail → "Error" rename shared with
// the tracer plane) for every type EXCEPT those declared in the operation postgres
// adapter package, which it prefixes with "Operation" (idempotent — no double
// prefix). This preserves
// every already-shipped schema name (all mmodel.* bodies plus the wave-1 non-mmodel
// AssetRate/Pagination names) while making the newly-registered operation.* types
// unique. It MUST run after openapi.New and BEFORE any huma.Register on that API,
// because the registry namer is captured lazily on first registration.
//
// ponytail: scoped to the ONE package that nests mmodel-shadowing types. A blanket
// package-qualifying namer is deliberately avoided: it would rename the wave-1
// AssetRate/Pagination schemas and churn the served spec. If a future wave adds a
// clash from another package, that huma.Register panics loudly at startup — extend
// the package check here then.
func InstallLedgerSchemaNamer(api huma.API) {
	installSchemaNamer(api, ledgerSchemaNamer)
}

// InstallSchemaNamer swaps in the tracer plane's namer. The tracer registers no
// mmodel-shadowing types (no operation/transaction/fee packages), so it needs
// only the shared problem.Detail → "Error" rename; every other type keeps its
// DefaultSchemaNamer name. Same lazy-capture ordering rule as
// InstallLedgerSchemaNamer: call after openapi.New and BEFORE any huma.Register.
func InstallSchemaNamer(api huma.API) {
	installSchemaNamer(api, sharedSchemaNamer)
}

// installSchemaNamer replaces the API's schema registry with one keyed by namer.
// Nil-guards the API and its components so a spec-disabled build is a no-op.
func installSchemaNamer(api huma.API, namer func(reflect.Type, string) string) {
	if api == nil {
		return
	}

	oapi := api.OpenAPI()
	if oapi == nil || oapi.Components == nil {
		return
	}

	registry := huma.NewMapRegistry("#/components/schemas/", namer)
	registerDomainSchemaAliases(registry)

	oapi.Components.Schemas = registry
}

// registerDomainSchemaAliases teaches a registry how to schema the domain types Huma
// cannot infer on its own. mtransaction.TransactionDate is a named type over
// time.Time, so Huma sees a struct whose fields are all unexported: it schemas the
// type as an OBJECT and then cannot parse the `format:"date-time"` / `example:` tags
// declared on fields of that type. Aliasing it to time.Time routes it through Huma's
// own time.Time case, which emits exactly {"type":"string","format":"date-time"}.
//
// This lives on the ADAPTER side rather than as a huma.SchemaProvider method on the
// domain type, so pkg/mtransaction — the shared transaction model that the ledger,
// the tracer and the fee engine all compile against — carries no dependency on an
// OpenAPI generator. It affects OpenAPI generation only; JSON decoding stays governed
// by TransactionDate.UnmarshalJSON.
//
// Aliases MUST be seeded before the first huma.Register on the registry, because
// mapRegistry.Schema consults its alias map on every lookup and caches the resulting
// schema under the resolved name. installSchemaNamer satisfies that by seeding the
// registry it is about to install, which is why a harness registering an operation
// whose body carries one of these types must go through an Install*SchemaNamer before
// its first huma.Register, exactly as AssembleHumaContract does.
func registerDomainSchemaAliases(registry huma.Registry) {
	if registry == nil {
		return
	}

	registry.RegisterTypeAlias(reflect.TypeFor[mtransaction.TransactionDate](), reflect.TypeFor[time.Time]())
}

// problemDetailPkgPath is the import path of the lib-commons RFC 9457 problem
// package. Its problem.Detail is the error body Huma emits on EVERY plane once
// problem.Install() overrides huma.NewError; without a rename it schemas as
// "Detail" (the bare Go type name). Both planes name it "Error" so the served
// spec's error schema reads as the org-wide error model, not an incidental type
// name. Matched as a STRING so this shared pkg never imports lib-commons/problem
// just to reference the type (no runtime coupling; the dump is offline).
const problemDetailPkgPath = "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"

// sharedSchemaNamer is the base namer both planes route through: it renames the
// shared problem.Detail error body to "Error" and defers everything else to
// DefaultSchemaNamer. ledgerSchemaNamer layers its plane-specific package
// qualifications on top of this.
func sharedSchemaNamer(t reflect.Type, hint string) string {
	dt := t
	for dt.Kind() == reflect.Pointer {
		dt = dt.Elem()
	}

	if dt.Name() == "Detail" && dt.PkgPath() == problemDetailPkgPath {
		return "Error"
	}

	return huma.DefaultSchemaNamer(t, hint)
}

// operationPkgPath is the import path of the operation postgres adapter package,
// whose types (Balance/Status/Amount nested in operation.Operation) collide with
// identically-named mmodel types on the shared registry. Matched as a STRING so this
// shared pkg never imports the component-internal adapter (which would invert
// layering / cycle through pkg).
const operationPkgPath = "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"

// transactionPkgPath is the import path of the transaction postgres adapter package.
// Its transaction.Transaction (the Wave-4 money-write Huma response body) nests a
// transaction.Status that collides with the mmodel.Status the Wave-1 onboarding bodies
// already own on the shared registry. Qualifying the transaction package with a
// "Transaction" prefix disambiguates Status ("TransactionStatus") while leaving the
// top-level body name unchanged (qualify() is idempotent, so "Transaction" stays
// "Transaction"). Matched as a STRING for the same layering reason as operationPkgPath.
// Only renames the NATIVE Huma OAS 3.1 schemas (openapi.ServeSpec, docs-gated); the
// swaggo swagger.json contract is generated independently and untouched.
const transactionPkgPath = "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"

// mtransactionPkgPath is the import path of the transaction domain package. Its
// mtransaction.Transaction (the transaction projection embedded in the fee-estimate
// request body) carries the bare name "Transaction", which collides on the shared
// registry with the postgres transaction.Transaction response body already named
// "Transaction". The type documents its own contract name (`// @name TransactionInput`),
// so it is published under that name — additive, since mtransaction.Transaction was
// never schema-generated before typed request bodies. Only the "Transaction" name is
// remapped: every other mtransaction type (CreateTransactionV2Input, V2LegInput, and the
// v1 create-input graph — Send/Source/FromTo/Amount/Share/Rate/…) keeps its bare name,
// which is what the already-published v2 contract binds to. Matched as a STRING for the
// same layering reason as operationPkgPath.
const mtransactionPkgPath = "github.com/LerianStudio/midaz/v4/pkg/mtransaction"

// ledgerHTTPInPkgPath is the import path of the ledger's inbound HTTP adapter, which
// declares the per-version response projections. Matched as a STRING for the same
// layering reason as the adapter paths above.
const ledgerHTTPInPkgPath = "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"

// v1ProjectionNames maps a /v1 response projection declared in the inbound HTTP adapter
// to the CANONICAL component name it publishes under. The Go types carry a version
// suffix so each reads as the sibling of its newer twin, while the published name stays
// what the generated v1 SDKs already bind to — renaming a v1 component would churn every
// v1 SDK, which is the opposite of what a backward-compatibility projection is for.
//
// The consequence is that the projection, not the domain type, owns the canonical name:
// see mmodelV2Names for the newer shape each domain type is pushed onto.
var v1ProjectionNames = map[string]string{
	"TransactionV1": "Transaction",
	"AccountV1":     "Account",
}

// mmodelPkgPath is the import path of the shared domain-model package. Matched as a
// STRING for symmetry with the paths above (mtransaction is imported here only for its
// TransactionDate type alias).
const mmodelPkgPath = "github.com/LerianStudio/midaz/v4/pkg/mmodel"

// mmodelV2Names remaps a domain type whose /v1 projection took over its canonical
// component name (see v1ProjectionNames) onto the versioned name of the NEWER wire
// shape it actually describes.
//
// mmodel.Account is the account WITH the holder seam — holderId and holderCheckSkipped —
// which is the /v2 account contract; the /v1 ops answer with in.AccountV1, which
// withholds both keys and publishes as "Account". The two shapes are distinct, so they
// must carry distinct names or huma.Register panics on the shared registry. Only the
// exact name "Account" is remapped: every sibling mmodel type (Accounts, AccountType,
// CreateAccountInput, …) keeps its bare name, which is what the published contract
// already binds to.
var mmodelV2Names = map[string]string{
	"Account": "AccountV2",
}

// feePkgPathPrefix roots the Wave-3 fee/billing packages whose response-body types
// register on the shared ledger Huma registry: feeshared/model (Pagination,
// BillingPackage, BillingCalculateResponse, and their nested tiers) and
// adapters/mongodb/fees/pack (Package). feeshared/model.Pagination collides with
// pkg/net/http.Pagination — the name the wave-1 ledger list ops already own on the
// registry and in the committed swagger contract — so every fee-side type is
// qualified with a "Fee" prefix (mirroring the operation-package precedent above).
// Matched by prefix as a STRING so this shared pkg never imports the
// component-internal fee adapters. This only renames the NATIVE Huma OAS 3.1
// schemas (openapi.ServeSpec, docs-gated); the swaggo swagger.json contract is
// generated independently and untouched.
const feePkgPathPrefix = "github.com/LerianStudio/midaz/v4/components/ledger/"

// feePkgPaths is the exact set of fee/billing packages to qualify. A prefix alone is
// too broad (it would sweep every ledger-internal type through the "Fee" prefix); an
// explicit set keeps the qualification scoped to the packages that actually register
// fee schemas.
var feePkgPaths = map[string]bool{
	feePkgPathPrefix + "pkg/feeshared/model":                 true,
	feePkgPathPrefix + "internal/adapters/mongodb/fees/pack": true,
}

func ledgerSchemaNamer(t reflect.Type, hint string) string {
	dt := t
	for dt.Kind() == reflect.Pointer {
		dt = dt.Elem()
	}

	name := dt.Name()
	if name == "" {
		return huma.DefaultSchemaNamer(t, hint)
	}

	if dt.PkgPath() == operationPkgPath {
		return qualify(name, "Operation")
	}

	if dt.PkgPath() == transactionPkgPath {
		return qualify(name, "Transaction")
	}

	if dt.PkgPath() == mtransactionPkgPath && name == "Transaction" {
		return "TransactionInput"
	}

	if dt.PkgPath() == ledgerHTTPInPkgPath {
		if canonical, ok := v1ProjectionNames[name]; ok {
			return canonical
		}
	}

	if dt.PkgPath() == mmodelPkgPath {
		if versioned, ok := mmodelV2Names[name]; ok {
			return versioned
		}
	}

	if feePkgPaths[dt.PkgPath()] {
		return qualify(name, "Fee")
	}

	// Fall through to the shared namer so the ledger also renames problem.Detail
	// → "Error" (it defers to DefaultSchemaNamer for everything else).
	return sharedSchemaNamer(t, hint)
}

// qualify prefixes name with the given package qualifier, idempotently.
func qualify(name, prefix string) string {
	if strings.HasPrefix(name, prefix) {
		return name
	}

	return prefix + name
}
