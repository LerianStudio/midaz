// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
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
// InstallLedgerSchemaNamer swaps in a namer that returns DefaultSchemaNamer's name
// for every type EXCEPT those declared in the operation postgres adapter package,
// which it prefixes with "Operation" (idempotent — no double prefix). This preserves
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
	if api == nil {
		return
	}

	oapi := api.OpenAPI()
	if oapi == nil || oapi.Components == nil {
		return
	}

	oapi.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", ledgerSchemaNamer)
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

	if feePkgPaths[dt.PkgPath()] {
		return qualify(name, "Fee")
	}

	return huma.DefaultSchemaNamer(t, hint)
}

// qualify prefixes name with the given package qualifier, idempotently.
func qualify(name, prefix string) string {
	if strings.HasPrefix(name, prefix) {
		return name
	}

	return prefix + name
}
