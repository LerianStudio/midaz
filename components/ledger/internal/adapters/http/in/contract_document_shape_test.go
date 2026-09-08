// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	problem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
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
		{"error responses declare the envelope each version serves", assertErrorResponsesMatchServedEnvelope},
		{"HEAD operations declare no response body", assertHeadOperationsDeclareNoBody},
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
// what the redocly join consumes; a "/v1" back here would reintroduce that break, because
// the version rides the operation path, not the server URL.
func assertServersIsRoot(t *testing.T, doc *huma.OpenAPI) {
	require.Len(t, doc.Servers, 1, "the version rides the operation path, so exactly one server is advertised")
	require.Equal(t, "/", doc.Servers[0].URL,
		`servers must be exactly [{URL:"/"}]; a versioned server URL breaks the redocly join`)
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
// transaction_routes_v2.go resolves them, so a schema-namer change surfaces here as a
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
// under /v1 and 68 under /v2, and the /v1 and /v2 operation-ID sets are disjoint. CRM,
// fees/billing, composition and account-block-exceptions are /v2-only, so their path keys count
// toward /v2 and never /v1.
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
	require.Equal(t, 68, v2Keys, "path keys under /v2")

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

// assertHeadOperationsDeclareNoBody locks the one thing HTTP decides for a HEAD
// operation whatever the contract says: the response carries no body. fasthttp sets
// Response.SkipBody for every HEAD request, so a declared body is a shape the
// service is unable to send. A generated client believes the declaration instead —
// an oapi-codegen-style client leaves the typed field nil on a 500, so a failed
// count reads as success, and a strict generator throws a parse error and loses the
// status the caller needed for backoff.
//
// It walks EVERY response, not only the error ones, and both planes, because the
// rule comes from the METHOD: a body declared on the 204 is as wrong as one on the
// 500, and the /v2 half of the metrics/count family has the same constraint as the
// /v1 half. StripHeadResponseContent is what holds it.
func assertHeadOperationsDeclareNoBody(t *testing.T, doc *huma.OpenAPI) {
	heads := map[string]int{}

	for key, item := range doc.Paths {
		if item == nil || item.Head == nil {
			continue
		}

		var plane string

		switch {
		case strings.HasPrefix(key, "/v1/"):
			plane = "/v1"
		case strings.HasPrefix(key, "/v2/"):
			plane = "/v2"
		default:
			continue
		}

		heads[plane]++

		for status, response := range item.Head.Responses {
			if response == nil {
				continue
			}

			require.Emptyf(t, response.Content,
				"%s %s is a HEAD operation, so response %q can never carry a body, but it declares %v",
				key, item.Head.OperationID, status, contentTypesOf(response.Content))
		}
	}

	// Non-vacuity: both planes publish the metrics/count HEAD family, so a walk that
	// found no HEAD operation asserted nothing.
	require.NotZerof(t, heads["/v1"], "no /v1 HEAD operation was inspected")
	require.NotZerof(t, heads["/v2"], "no /v2 HEAD operation was inspected")
}

// problemErrorMediaType is the RFC 9457 media type a plane serves its error bodies
// as when no version envelope reshapes them — every /v2 operation, and every tracer
// operation. Declared here rather than imported because the producer keeps it
// unexported (pkg/net/http/problem.go).
const problemErrorMediaType = "application/problem+json"

// assertErrorResponsesMatchServedEnvelope is the parity between what the contract
// DECLARES for a failure and what the service SERVES for it. Nothing else compares
// the two, which is how a bump that added baseline error statuses could publish the
// wrong error body on 87 /v1 operations and pass every gate.
//
// /v1 runs behind ErrorEnvelope (middleware/envelope.go), which rewrites EVERY
// response with status >= 400 into the legacy {code,title,message} body at
// application/json — the wire side of that is locked by envelope_boundary_test.go.
// /v2 has no entry in that middleware's version registry, so it serves the RFC 9457
// problem document unchanged. Declaring one plane's schema or media type on the
// other tells a code generator to parse a body the service never sends, which is
// worse than declaring nothing: the caller does not discover it at integration time,
// the generated client does at runtime.
//
// It walks the "default" catch-all AND every numeric status in the 4xx/5xx range
// because the numeric ones are exactly what regressed: the lib-commons baseline hook
// materializes them at huma.Register time from the RFC 9457 content, and only
// RepointV1ErrorResponses puts them back on the /v1 envelope.
//
// Which responses to walk, and how many there must be, are decided HERE — see
// isErrorStatusKey and the counters below. Asking isErrorResponseKey, the predicate
// RepointV1ErrorResponses itself applies, made the gate blind to that predicate
// narrowing its own reach, because test and fix then skipped the same responses in
// lockstep.
func assertErrorResponsesMatchServedEnvelope(t *testing.T, doc *huma.OpenAPI) {
	require.NotNil(t, doc.Components, "document must carry components")
	require.NotNil(t, doc.Components.Schemas, "document must carry a schema registry")

	registry := doc.Components.Schemas
	require.Contains(t, registry.Map(), "Error",
		"the RFC 9457 error body must be registered as Error before this can compare against it")
	require.Contains(t, registry.Map(), "LegacyError",
		"the /v1 error body must be registered as LegacyError before this can compare against it")

	legacyRef := registry.Schema(reflect.TypeFor[LegacyError](), true, "LegacyError").Ref
	problemRef := registry.Schema(reflect.TypeFor[problem.Detail](), true, "Error").Ref
	require.NotEmpty(t, legacyRef, "LegacyError must resolve to a component ref")
	require.NotEmpty(t, problemRef, "the RFC 9457 Error body must resolve to a component ref")
	require.NotEqual(t, legacyRef, problemRef,
		"the two planes must publish DISTINCT error components; one ref for both means a pass collapsed them")

	var (
		checked           = map[string]int{} // error bodies this property asserted on
		operations        = map[string]int{} // operations on the plane
		headOperations    = map[string]int{} // of those, the ones serving HEAD
		universalDeclared = map[string]int{} // "default" + "500" keys seen
		universalChecked  = map[string]int{} // of those, the ones carrying a body
	)

	for key, item := range doc.Paths {
		var (
			plane     string
			wantMedia string
			wantRef   string
		)

		switch {
		case strings.HasPrefix(key, "/v1/"):
			plane, wantMedia, wantRef = "/v1", legacyErrorMediaType, legacyRef
		case strings.HasPrefix(key, "/v2/"):
			plane, wantMedia, wantRef = "/v2", problemErrorMediaType, problemRef
		default:
			continue
		}

		for _, op := range operationsOf(item) {
			operations[plane]++

			if op == item.Head {
				headOperations[plane]++
			}

			for status, response := range op.Responses {
				if !isErrorStatusKey(status) {
					continue
				}

				universal := status == "default" || status == serverErrorStatusKey
				if universal {
					universalDeclared[plane]++
				}

				require.NotNilf(t, response, "%s %s declares a nil %q response", key, op.OperationID, status)

				if len(response.Content) == 0 {
					// No content declares no body. Only a HEAD operation may reach
					// here: HTTP forbids it a body, so StripHeadResponseContent drops
					// the declaration. On any other method this is a response the
					// property would silently skip, which is how a narrowed pass
					// hides.
					require.Truef(t, op == item.Head,
						"%s %s response %q declares no body; only a HEAD operation may, because HTTP forbids it one",
						key, op.OperationID, status)

					continue
				}

				require.Lenf(t, response.Content, 1,
					"%s %s response %q declares %d media types; an error body is served as exactly one",
					key, op.OperationID, status, len(response.Content))

				media, ok := response.Content[wantMedia]
				require.Truef(t, ok,
					"%s %s response %q must be declared as %s — what the service actually serves on %s — but is declared as %v",
					key, op.OperationID, status, wantMedia, plane, contentTypesOf(response.Content))

				require.NotNilf(t, media.Schema,
					"%s %s response %q declares %s with no schema", key, op.OperationID, status, wantMedia)
				require.Equalf(t, wantRef, media.Schema.Ref,
					"%s %s response %q must $ref the error body %s serves (%s), not %s",
					key, op.OperationID, status, plane, wantRef, media.Schema.Ref)

				if universal {
					universalChecked[plane]++
				}

				checked[plane]++
			}
		}
	}

	// Independent expectation for how much this property must have reached. Two
	// statuses are universal and neither is decided by a pass under test: huma writes
	// one "default" catch-all per operation, and the lib-commons baseline hook adds
	// one 500 per operation. So each plane declares exactly two per operation, and
	// exactly the non-HEAD ones carry a body — a HEAD operation can never send one
	// (assertHeadOperationsDeclareNoBody). Both totals are read off the document's own
	// operation list, so a response skipped for any reason shows up as a shortfall
	// instead of a quiet pass. require.NotZero alone could not see that: it rules out
	// the empty walk, not a narrowed one.
	for _, plane := range []string{"/v1", "/v2"} {
		require.Equalf(t, 2*operations[plane], universalDeclared[plane],
			`every %s operation must declare both a "default" and a "500" error response; %d operations, %d such responses`,
			plane, operations[plane], universalDeclared[plane])

		require.Equalf(t, 2*(operations[plane]-headOperations[plane]), universalChecked[plane],
			`every non-HEAD %s operation must have had both its "default" and its "500" error body inspected; %d operations (%d HEAD), %d inspected`,
			plane, operations[plane], headOperations[plane], universalChecked[plane])

		require.NotZerof(t, checked[plane], "no %s error response was inspected", plane)
	}
}

// serverErrorStatusKey is the 500 the lib-commons baseline hook adds to every
// operation (commons/net/http/openapi, baselineResponses). Together with huma's
// "default" catch-all it is the pair the counters above use as their per-operation
// yardstick.
const serverErrorStatusKey = "500"

// isErrorStatusKey reports whether an OpenAPI responses key names an error response.
// It deliberately RESTATES the rule RepointV1ErrorResponses applies instead of
// calling isErrorResponseKey: a gate that asks the code under test which responses
// to inspect cannot see that code narrowing its own reach. Narrowing
// isErrorResponseKey to 500-and-up leaves 86 /v1 operations declaring the /v2
// problem document, and the version of this property that called it still passed.
// The bounds are literals here for the same reason.
func isErrorStatusKey(key string) bool {
	if key == "default" {
		return true
	}

	status, err := strconv.Atoi(key)

	return err == nil && status >= 400 && status <= 599
}

// contentTypesOf returns a response's declared media types, sorted, for a readable
// failure message.
func contentTypesOf(content map[string]*huma.MediaType) []string {
	names := make([]string, 0, len(content))
	for name := range content {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
