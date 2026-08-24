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

// metadataIndexV2Ops enumerates the three metadata-index operations the /v2 group mirrors from
// /v1: the HTTP method, the group-relative op path the Huma document publishes (the shared
// contract prepends "/v2"), and the /v1 operationId the op reuses. metadata-index is a
// LEDGER-AGNOSTIC settings resource, so its paths carry NO org/ledger prefix. The v2 twin is a
// STRAIGHT MIRROR — same handler method, same input/output types — so its operationId is the v1
// id with the version suffix appended. That suffix is the only thing that keeps the two twins
// from colliding as a duplicate operationId in the one document.
var metadataIndexV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "create", method: http.MethodPost, opPath: "/settings/metadata-indexes/entities/{entity_name}", v1OperationID: "createMetadataIndex"},
	{action: "list", method: http.MethodGet, opPath: "/settings/metadata-indexes", v1OperationID: "getAllMetadataIndexes"},
	{action: "delete", method: http.MethodDelete, opPath: "/settings/metadata-indexes/entities/{entity_name}/key/{index_key}", v1OperationID: "deleteMetadataIndex"},
}

// metadataIndexV2OperationSuffix is the version suffix a v2 twin appends to its v1 operationId.
// It is spelled literally here rather than read from v2OpSuffix so a rename of the
// production constant surfaces as a contract change the client SDKs would feel, not a
// silently-tracking test.
const metadataIndexV2OperationSuffix = "V2"

// metadataIndexJSONMediaType is the content type the metadata-index ops publish their request
// and response bodies under. The reuse invariant below is asserted only over these bodies.
const metadataIndexJSONMediaType = "application/json"

// TestRegisterMetadataIndexV2Routes_MirrorsV1UnderV2 asserts each of the three metadata-index
// operations is published on the assembled /v2 surface at the /v1 path shape prefixed with
// /v2, advertising the v1 operationId with the version suffix appended. It reads the REAL
// unified document (buildUnifiedHumaAPI) — the same huma.API the served contract and the
// committed dump come from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterMetadataIndexV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range metadataIndexV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s metadata-index op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+metadataIndexV2OperationSuffix, operation.OperationID,
				"the v2 %s metadata-index op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// metadataIndexOpBodyRefs projects an operation onto the component $refs its JSON request body
// and its 2xx JSON response body name. A body Huma describes inline (the opaque RawBody
// request schema the create op carries, or the array wrapper the list op returns) has no
// top-level $ref, so its slot comes back "". Returning the refs — not the schemas — is the
// point: reuse of a v1 Go type is observable precisely as the v2 twin pointing at the SAME
// "#/components/schemas/<Name>" string.
func metadataIndexOpBodyRefs(op *huma.Operation) (reqRef string, respRefs []string) {
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content[metadataIndexJSONMediaType]; ok && media.Schema != nil {
			reqRef = media.Schema.Ref
		}
	}

	for status, resp := range op.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}

		if media, ok := resp.Content[metadataIndexJSONMediaType]; ok && media.Schema != nil {
			respRefs = append(respRefs, media.Schema.Ref)
		}
	}

	return reqRef, respRefs
}

// metadataIndexReferencedComponents gathers the base component names ("#/components/schemas/"
// prefix stripped) the metadata-index ops name in their JSON bodies, read off the assembled
// document so a rename of a metadata-index type is followed here automatically. It walks both
// /v1 and /v2 twins; a straight mirror names the identical set on each side.
func metadataIndexReferencedComponents(paths map[string]*huma.PathItem) map[string]bool {
	const refPrefix = "#/components/schemas/"

	refs := make(map[string]bool)

	collect := func(ref string) {
		if name, ok := strings.CutPrefix(ref, refPrefix); ok {
			refs[name] = true
		}
	}

	for _, op := range metadataIndexV2Ops {
		for _, prefix := range []string{"/v1", "/v2"} {
			item, ok := paths[prefix+op.opPath]
			if !ok {
				continue
			}

			operation := operationForMethod(item, op.method)
			if operation == nil {
				continue
			}

			reqRef, respRefs := metadataIndexOpBodyRefs(operation)

			collect(reqRef)

			for _, r := range respRefs {
				collect(r)
			}
		}
	}

	return refs
}

// TestRegisterMetadataIndexV2Routes_ReusesV1SchemaComponents proves the core correctness claim
// of the straight-mirror approach: the /v2 metadata-index twin REUSES the v1 request/response
// Go types, so Huma's registry dedups them to ONE schema component and the v2 op's body $ref
// is byte-identical to the v1 op's. It reads the REAL unified document, the same huma.API the
// served contract and the committed dump come from.
//
// Were a v2 twin to mint its own type for any body, its op would $ref a different (V2-named)
// component and the equality below would turn red.
func TestRegisterMetadataIndexV2Routes_ReusesV1SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	// Guards the assertions below against vacuously passing on a document where every ref
	// came back "": at least one metadata-index op must actually name a response-body component.
	sawSharedResponseRef := false

	for _, op := range metadataIndexV2Ops {
		v1Item, ok := paths["/v1"+op.opPath]
		require.Truef(t, ok, "the /v1 surface must publish the %s metadata-index op", op.action)

		v2Item, ok := paths["/v2"+op.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s metadata-index op", op.action)

		v1Op := operationForMethod(v1Item, op.method)
		require.NotNilf(t, v1Op, "the v1 %s metadata-index op must carry a %s operation", op.action, op.method)

		v2Op := operationForMethod(v2Item, op.method)
		require.NotNilf(t, v2Op, "the v2 %s metadata-index op must carry a %s operation", op.action, op.method)

		v1Req, v1Resp := metadataIndexOpBodyRefs(v1Op)
		v2Req, v2Resp := metadataIndexOpBodyRefs(v2Op)

		assert.Equalf(t, v1Req, v2Req,
			"the v2 %s metadata-index op must name the SAME request-body schema as v1 (a straight mirror mints no new request type)", op.action)
		assert.ElementsMatchf(t, v1Resp, v2Resp,
			"the v2 %s metadata-index op must name the SAME response-body component(s) as v1 (Huma dedups the reused Go type to one schema)", op.action)

		for _, ref := range v2Resp {
			if ref == "" {
				continue
			}

			sawSharedResponseRef = true

			assert.Falsef(t, strings.HasSuffix(ref, metadataIndexV2OperationSuffix),
				"the v2 %s metadata-index op response ref %q must not name a %s-suffixed component — the v1 type is reused, not re-minted",
				op.action, ref, metadataIndexV2OperationSuffix)
		}
	}

	require.True(t, sawSharedResponseRef,
		"at least one metadata-index op must reference a response-body component, or the reuse claim is vacuous")
}

// TestRegisterMetadataIndexV2Routes_MintsNoV2SchemaComponents guards against accidental new-type
// creation for the straight mirror: no metadata-index schema component may carry the version
// suffix. The reused v1 type (MetadataIndex) is registered ONCE, so the suffixed twin of each
// referenced component must be absent.
func TestRegisterMetadataIndexV2Routes_MintsNoV2SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()
	schemas := doc.Components.Schemas.Map()

	// The components the metadata-index ops actually name, gathered from the assembled document
	// rather than hardcoded, so a renamed metadata-index type is followed here. The reused v1
	// type is registered ONCE, so the suffixed twin of each must be absent.
	referenced := metadataIndexReferencedComponents(doc.Paths)
	require.Containsf(t, referenced, "MetadataIndex",
		"the metadata-index ops must reference the MetadataIndex body component, or this test guards nothing")

	for name := range referenced {
		assert.NotContainsf(t, schemas, name+metadataIndexV2OperationSuffix,
			"no %s twin of the reused metadata-index body component %q may be minted", metadataIndexV2OperationSuffix, name)
	}

	// The document-wide guard: no metadata-index-named schema carries the V2 suffix.
	for name := range schemas {
		if !strings.HasPrefix(name, "MetadataIndex") {
			continue
		}

		assert.Falsef(t, strings.HasSuffix(name, metadataIndexV2OperationSuffix),
			"no metadata-index schema component may carry the %s suffix; found %q", metadataIndexV2OperationSuffix, name)
	}
}
