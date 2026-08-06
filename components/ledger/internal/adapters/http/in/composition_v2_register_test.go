// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compositionV2Ops enumerates the single composition operation the /v2 group mirrors from
// /v1: the HTTP method, the group-relative op path the Huma document publishes (the shared
// contract prepends "/v2"), and the /v1 operationId the op reuses. Composition is a single
// holder-account orchestration route, so this one-row table is the whole surface. The v2 twin
// is a STRAIGHT MIRROR — same handler method, same input/output types — so its operationId is
// the v1 id with the version suffix appended. That suffix is the only thing that keeps the two
// twins from colliding as a duplicate operationId in the one document.
var compositionV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "createHolderAccount", method: http.MethodPost, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/holders/{id}/accounts", v1OperationID: "createHolderAccount"},
}

// compositionV2OperationSuffix is the version suffix a v2 twin appends to its v1 operationId.
// It is spelled literally here rather than read from routeOpSuffixV2 so a rename of the
// production constant surfaces as a contract change the client SDKs would feel, not a
// silently-tracking test.
const compositionV2OperationSuffix = "V2"

// compositionJSONMediaType is the content type the composition op publishes its request and
// response bodies under. The reuse invariant below is asserted only over these bodies.
const compositionJSONMediaType = "application/json"

// TestRegisterCompositionV2Routes_MirrorsV1UnderV2 asserts the composition operation is
// published on the assembled /v2 surface at the /v1 path shape prefixed with /v2, advertising
// the v1 operationId with the version suffix appended. It reads the REAL unified document
// (buildUnifiedHumaAPI) — the same huma.API the served contract and the committed dump come
// from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterCompositionV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range compositionV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s composition op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+compositionV2OperationSuffix, operation.OperationID,
				"the v2 %s composition op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// compositionOpBodyRefs projects an operation onto the component $refs its JSON request body
// and its 2xx JSON response body name. The composition op carries an opaque RawBody request
// schema Huma describes inline, so its request slot comes back "". Returning the refs — not the
// schemas — is the point: reuse of a v1 Go type is observable precisely as the v2 twin pointing
// at the SAME "#/components/schemas/<Name>" string.
func compositionOpBodyRefs(op *huma.Operation) (reqRef string, respRefs []string) {
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content[compositionJSONMediaType]; ok && media.Schema != nil {
			reqRef = media.Schema.Ref
		}
	}

	for status, resp := range op.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}

		if media, ok := resp.Content[compositionJSONMediaType]; ok && media.Schema != nil {
			respRefs = append(respRefs, media.Schema.Ref)
		}
	}

	return reqRef, respRefs
}

// compositionReferencedComponents gathers the base component names ("#/components/schemas/"
// prefix stripped) the composition op names in its JSON bodies, read off the assembled document
// so a rename of the composition response type is followed here automatically. It walks both
// /v1 and /v2 twins; a straight mirror names the identical set on each side.
func compositionReferencedComponents(paths map[string]*huma.PathItem) map[string]bool {
	const refPrefix = "#/components/schemas/"

	refs := make(map[string]bool)

	collect := func(ref string) {
		if name, ok := strings.CutPrefix(ref, refPrefix); ok {
			refs[name] = true
		}
	}

	for _, op := range compositionV2Ops {
		for _, prefix := range []string{"/v1", "/v2"} {
			item, ok := paths[prefix+op.opPath]
			if !ok {
				continue
			}

			operation := operationForMethod(item, op.method)
			if operation == nil {
				continue
			}

			reqRef, respRefs := compositionOpBodyRefs(operation)

			collect(reqRef)

			for _, r := range respRefs {
				collect(r)
			}
		}
	}

	return refs
}

// TestRegisterCompositionV2Routes_ReusesV1SchemaComponents proves the core correctness claim of
// the straight-mirror approach: the /v2 composition twin REUSES the v1 response Go type, so
// Huma's registry dedups it to ONE schema component and the v2 op's response $ref is
// byte-identical to the v1 op's. It reads the REAL unified document, the same huma.API the
// served contract and the committed dump come from.
//
// Were the v2 twin to mint its own type for its response, its op would $ref a different
// (V2-named) component and the equality below would turn red.
func TestRegisterCompositionV2Routes_ReusesV1SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	// Guards the assertions below against vacuously passing on a document where every ref
	// came back "": the composition op must actually name a response-body component.
	sawSharedResponseRef := false

	for _, op := range compositionV2Ops {
		v1Item, ok := paths["/v1"+op.opPath]
		require.Truef(t, ok, "the /v1 surface must publish the %s composition op", op.action)

		v2Item, ok := paths["/v2"+op.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s composition op", op.action)

		v1Op := operationForMethod(v1Item, op.method)
		require.NotNilf(t, v1Op, "the v1 %s composition op must carry a %s operation", op.action, op.method)

		v2Op := operationForMethod(v2Item, op.method)
		require.NotNilf(t, v2Op, "the v2 %s composition op must carry a %s operation", op.action, op.method)

		v1Req, v1Resp := compositionOpBodyRefs(v1Op)
		v2Req, v2Resp := compositionOpBodyRefs(v2Op)

		assert.Equalf(t, v1Req, v2Req,
			"the v2 %s composition op must name the SAME request-body schema as v1 (a straight mirror mints no new request type)", op.action)
		assert.ElementsMatchf(t, v1Resp, v2Resp,
			"the v2 %s composition op must name the SAME response-body component(s) as v1 (Huma dedups the reused Go type to one schema)", op.action)

		for _, ref := range v2Resp {
			if ref == "" {
				continue
			}

			sawSharedResponseRef = true

			assert.Falsef(t, strings.HasSuffix(ref, compositionV2OperationSuffix),
				"the v2 %s composition op response ref %q must not name a %s-suffixed component — the v1 type is reused, not re-minted",
				op.action, ref, compositionV2OperationSuffix)
		}
	}

	require.True(t, sawSharedResponseRef,
		"the composition op must reference a response-body component, or the reuse claim is vacuous")
}

// TestRegisterCompositionV2Routes_MintsNoV2SchemaComponents guards against accidental new-type
// creation for the straight mirror: no composition response schema component may carry the
// version suffix. The reused v1 type is registered ONCE, so the suffixed twin must be absent.
func TestRegisterCompositionV2Routes_MintsNoV2SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()
	schemas := doc.Components.Schemas.Map()

	// The components the composition op actually names, gathered from the assembled document
	// rather than hardcoded, so a renamed composition type is followed here. The reused v1 type
	// is registered ONCE, so the suffixed twin of each must be absent.
	referenced := compositionReferencedComponents(doc.Paths)
	require.Containsf(t, referenced, "HolderAccountResponse",
		"the composition op must reference the HolderAccountResponse body component, or this test guards nothing")

	for name := range referenced {
		assert.NotContainsf(t, schemas, name+compositionV2OperationSuffix,
			"no %s twin of the reused composition body component %q may be minted", compositionV2OperationSuffix, name)
	}

	// The document-wide guard: no HolderAccountResponse-named schema carries the V2 suffix.
	for name := range schemas {
		if !strings.HasPrefix(name, "HolderAccountResponse") {
			continue
		}

		assert.Falsef(t, strings.HasSuffix(name, compositionV2OperationSuffix),
			"no composition response schema component may carry the %s suffix; found %q", compositionV2OperationSuffix, name)
	}
}
