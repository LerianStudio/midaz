// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// TestContractDocumentShape reads the OpenAPI document the SINGLE unified harness
// assembles (buildUnifiedHumaAPI, the same seam production's mountHumaContracts uses)
// and locks the shape of that in-memory document — NOT the on-disk dump, which is the
// serialized output of this same document. Each property builds its own document so the
// subtests stay isolated under -race/-shuffle (AssembleHumaContract writes the
// process-global huma.NewError under a mutex, so concurrent builds are safe).
func TestContractDocumentShape(t *testing.T) {
	t.Parallel()

	properties := []struct {
		name string
		fn   func(t *testing.T, doc *huma.OpenAPI)
	}{
		{"servers is exactly the root", assertServersIsRoot},
		{"no anonymous-dedup numeric suffix collision", assertNoDedupSuffixCollision},
		{"v2 create bodies ref the typed input and carry the prose", assertV2CreateBodiesTyped},
		{"both prefixes coexist with disjoint operation ids", assertPrefixesCoexist},
		{"security schemes declared with no dangling reference", assertSecuritySchemesResolve},
	}

	for _, prop := range properties {
		t.Run(prop.name, func(t *testing.T) {
			t.Parallel()

			_, api := buildUnifiedHumaAPI()
			prop.fn(t, api.OpenAPI())
		})
	}
}

// assertServersIsRoot locks Servers to exactly [{URL:"/"}]. That single root server is
// what the redocly join consumes and what makes serverPathSegments return empty in the
// Postman generator; a "/v1" back here would reintroduce both breaks, because the version
// rides the operation path, not the server URL.
func assertServersIsRoot(t *testing.T, doc *huma.OpenAPI) {
	require.Len(t, doc.Servers, 1, "the version rides the operation path, so exactly one server is advertised")
	require.Equal(t, "/", doc.Servers[0].URL,
		`servers must be exactly [{URL:"/"}]; a versioned server URL breaks the redocly join and serverPathSegments`)
}

// assertNoDedupSuffixCollision fails ONLY on Huma's anonymous-dedup form: a schema name X
// whose trailing digits, once stripped, leave a base name that is ITSELF registered (a
// "Foo2" sitting alongside "Foo"). It deliberately does NOT flag a name that merely ends
// in or contains a digit, so the four legitimate v2 names (CreateTransactionV2Input,
// TransactionV2, V2LegInput, V2ShareInput) pass untouched.
//
// The NAMED-type collision is out of scope here and is covered upstream: by F3
// (mapRegistry.Schema) a second NAMED type resolving to an already-registered name panics
// at boot, long before this test reads the document. Anonymous inline types are the only
// ones that share a hint-derived name and receive a silent numeric suffix instead of a
// panic — this test asserts the dedup produced no such suffixed pair. It does not,
// positively, prove any anonymous case is exercised; it proves the tolerated collision has
// not occurred.
func assertNoDedupSuffixCollision(t *testing.T, doc *huma.OpenAPI) {
	require.NotNil(t, doc.Components, "document must carry components")
	require.NotNil(t, doc.Components.Schemas, "document must carry a schema registry")

	names := doc.Components.Schemas.Map()
	require.NotEmpty(t, names, "the document must register schemas")

	for name := range names {
		base := strings.TrimRight(name, "0123456789")
		if base == name {
			continue // no trailing digit to strip; not a dedup-suffix candidate
		}

		_, baseExists := names[base]
		require.Falsef(t, baseExists,
			"schema %q is an anonymous-dedup numeric suffix of registered schema %q", name, base)
	}
}

// assertV2CreateBodiesTyped proves publishV2CreateBodySchema survives the /v2 prefix — the
// single most silent regression in the change. It checks the four v2 create ops carry a
// JSON request body $ref-ing the typed CreateTransactionV2Input component, that all four
// resolve to the SAME ref, and that the two prose rules (v2CreateBodyDescription,
// v2LegDescription) that the flat schemas cannot express structurally are stamped on the
// request and leg components.
//
// The expected refs are resolved off the registry by type, exactly as
// transaction_v2_register.go resolves them, so a schema-namer change surfaces here as a
// mismatch rather than being baked in as a literal ref string.
func assertV2CreateBodiesTyped(t *testing.T, doc *huma.OpenAPI) {
	require.NotNil(t, doc.Components, "document must carry components")
	require.NotNil(t, doc.Components.Schemas, "document must carry a schema registry")

	wantBodyRef := doc.Components.Schemas.Schema(reflect.TypeFor[mtransaction.CreateTransactionV2Input](), true, "").Ref
	wantLegRef := doc.Components.Schemas.Schema(reflect.TypeFor[mtransaction.V2LegInput](), true, "").Ref
	require.NotEmpty(t, wantBodyRef, "CreateTransactionV2Input must be a registered component")
	require.NotEmpty(t, wantLegRef, "V2LegInput must be a registered component")

	createIDs := make(map[string]bool, len(v2CreateActions))
	for _, action := range v2CreateActions {
		createIDs[action.operationID] = true
	}

	found := make(map[string]string, len(v2CreateActions)) // operation ID -> request-body ref

	for _, item := range doc.Paths {
		op := item.Post
		if op == nil || !createIDs[op.OperationID] {
			continue
		}

		require.NotNilf(t, op.RequestBody, "%s must carry a request body", op.OperationID)

		media, ok := op.RequestBody.Content[v2CreateBodyContentType]
		require.Truef(t, ok, "%s must carry a %s request body", op.OperationID, v2CreateBodyContentType)
		require.NotNilf(t, media.Schema, "%s request body must carry a schema", op.OperationID)

		found[op.OperationID] = media.Schema.Ref
	}

	require.Lenf(t, found, len(v2CreateActions),
		"every v2 create action must appear once under a %s path (publishV2CreateBodySchema is all-or-nothing)", v2CreateBodyContentType)

	for id, ref := range found {
		require.Equalf(t, wantBodyRef, ref,
			"%s request body must $ref the typed CreateTransactionV2Input component", id)
	}

	bodyComponent := doc.Components.Schemas.SchemaFromRef(wantBodyRef)
	require.NotNil(t, bodyComponent, "CreateTransactionV2Input component must resolve from its ref")
	require.Equal(t, v2CreateBodyDescription, bodyComponent.Description,
		"the v2 create-body prose must be stamped on the request component")

	legComponent := doc.Components.Schemas.SchemaFromRef(wantLegRef)
	require.NotNil(t, legComponent, "V2LegInput component must resolve from its ref")
	require.Equal(t, v2LegDescription, legComponent.Description,
		"the v2 leg prose must be stamped on the leg component")
}

// assertPrefixesCoexist is the readable gate for boot invariant F2 (AddOperation panics on
// a duplicate ID): both version prefixes live in ONE document, the path-key totals are 55
// under /v1 and 67 under /v2, and the /v1 and /v2 operation-ID sets are disjoint. CRM,
// fees/billing and composition are /v2-only, so their path keys count toward /v2 and never /v1.
func assertPrefixesCoexist(t *testing.T, doc *huma.OpenAPI) {
	var v1Keys, v2Keys int

	v1IDs := map[string]bool{}
	v2IDs := map[string]bool{}

	for key, item := range doc.Paths {
		switch {
		case strings.HasPrefix(key, "/v1/"):
			v1Keys++

			for _, op := range operationsOf(item) {
				v1IDs[op.OperationID] = true
			}
		case strings.HasPrefix(key, "/v2/"):
			v2Keys++

			for _, op := range operationsOf(item) {
				v2IDs[op.OperationID] = true
			}
		}
	}

	require.Equal(t, 55, v1Keys, "path keys under /v1")
	require.Equal(t, 67, v2Keys, "path keys under /v2")

	var overlap []string

	for id := range v2IDs {
		if v1IDs[id] {
			overlap = append(overlap, id)
		}
	}

	sort.Strings(overlap)

	require.Emptyf(t, overlap,
		"operation-ID sets must be disjoint across /v1 and /v2 (F2 panics on a duplicate ID); overlap (%d): %s",
		len(overlap), strings.Join(overlap, ", "))
}

// assertSecuritySchemesResolve reproduces in Go, over the source document, the assertion
// that today lives only as jq over the joined hub. BearerAuth is the ONLY scheme the
// ledger may declare: the Fiber guard chain authorizes a JWT bearer token and nothing
// else, so any other scheme would advertise an auth method the runtime rejects. No
// operation may reference a scheme the document does not declare.
func assertSecuritySchemesResolve(t *testing.T, doc *huma.OpenAPI) {
	require.NotNil(t, doc.Components, "document must carry components")
	require.Equal(t, []string{"BearerAuth"}, declaredSecuritySchemes(doc),
		"ledger contract must declare exactly BearerAuth — it accepts no other scheme")

	for _, name := range referencedSecuritySchemes(doc) {
		require.Containsf(t, doc.Components.SecuritySchemes, name,
			"operation references security scheme %q that Components.SecuritySchemes does not declare (dangling reference)", name)
	}
}
