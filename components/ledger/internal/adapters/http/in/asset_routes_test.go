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

// assetV2Ops enumerates the six asset operations the /v2 group mirrors from /v1: the HTTP
// method, the group-relative op path the Huma document publishes (the shared contract
// prepends "/v2"), and the /v1 operationId the op reuses. Unlike account-type, asset carries
// the HEAD-count op, so the CRUD-plus-list-plus-count set is the whole surface. The v2 twin
// is a STRAIGHT MIRROR — same handler method, same input/output types — so its operationId is
// the v1 id with the version suffix appended. That suffix is the only thing that keeps the two
// twins from colliding as a duplicate operationId in the one document.
var assetV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "create", method: http.MethodPost, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/assets", v1OperationID: "createAsset"},
	{action: "list", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/assets", v1OperationID: "listAssets"},
	{action: "getByID", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", v1OperationID: "getAssetByID"},
	{action: "update", method: http.MethodPatch, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", v1OperationID: "updateAsset"},
	{action: "delete", method: http.MethodDelete, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", v1OperationID: "deleteAsset"},
	{action: "count", method: http.MethodHead, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/assets/metrics/count", v1OperationID: "countAssets"},
}

// assetV2OperationSuffix is the version suffix a v2 twin appends to its v1 operationId. It is
// spelled literally here rather than read from routeOpSuffixV2 so a rename of the production
// constant surfaces as a contract change the client SDKs would feel, not a silently-tracking
// test.
const assetV2OperationSuffix = "V2"

// assetJSONMediaType is the content type the asset ops publish their request and response
// bodies under. The reuse invariant below is asserted only over these bodies.
const assetJSONMediaType = "application/json"

// TestRegisterAssetV2Routes_MirrorsV1UnderV2 asserts each of the six asset operations is
// published on the assembled /v2 surface at the /v1 path shape prefixed with /v2, advertising
// the v1 operationId with the version suffix appended. It reads the REAL unified document
// (buildUnifiedHumaAPI) — the same huma.API the served contract and the committed dump come
// from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterAssetV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range assetV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s asset op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+assetV2OperationSuffix, operation.OperationID,
				"the v2 %s asset op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// assetOpBodyRefs projects an operation onto the component $refs its JSON request body and its
// 2xx JSON response body name. A body Huma describes inline (the opaque RawBody request schema
// the create/update ops carry) has no $ref, so its slot comes back ""; the HEAD-count op names
// no JSON body at all, so both slots come back empty. Returning the refs — not the schemas —
// is the point: reuse of a v1 Go type is observable precisely as the v2 twin pointing at the
// SAME "#/components/schemas/<Name>" string.
func assetOpBodyRefs(op *huma.Operation) (reqRef string, respRefs []string) {
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content[assetJSONMediaType]; ok && media.Schema != nil {
			reqRef = media.Schema.Ref
		}
	}

	for status, resp := range op.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}

		if media, ok := resp.Content[assetJSONMediaType]; ok && media.Schema != nil {
			respRefs = append(respRefs, media.Schema.Ref)
		}
	}

	return reqRef, respRefs
}

// assetReferencedComponents gathers the base component names ("#/components/schemas/" prefix
// stripped) the asset ops name in their JSON bodies, read off the assembled document so a
// rename of an asset type is followed here automatically. It walks both /v1 and /v2 twins; a
// straight mirror names the identical set on each side.
func assetReferencedComponents(paths map[string]*huma.PathItem) map[string]bool {
	const refPrefix = "#/components/schemas/"

	refs := make(map[string]bool)

	collect := func(ref string) {
		if name, ok := strings.CutPrefix(ref, refPrefix); ok {
			refs[name] = true
		}
	}

	for _, op := range assetV2Ops {
		for _, prefix := range []string{"/v1", "/v2"} {
			item, ok := paths[prefix+op.opPath]
			if !ok {
				continue
			}

			operation := operationForMethod(item, op.method)
			if operation == nil {
				continue
			}

			reqRef, respRefs := assetOpBodyRefs(operation)

			collect(reqRef)

			for _, r := range respRefs {
				collect(r)
			}
		}
	}

	return refs
}

// TestRegisterAssetV2Routes_ReusesV1SchemaComponents proves the core correctness claim of the
// straight-mirror approach: the /v2 asset twin REUSES the v1 request/response Go types, so
// Huma's registry dedups them to ONE schema component and the v2 op's body $ref is
// byte-identical to the v1 op's. It reads the REAL unified document, the same huma.API the
// served contract and the committed dump come from.
//
// Were a v2 twin to mint its own type for any body, its op would $ref a different (V2-named)
// component and the equality below would turn red.
func TestRegisterAssetV2Routes_ReusesV1SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	// Guards the assertions below against vacuously passing on a document where every ref
	// came back "": at least one asset op must actually name a response-body component.
	sawSharedResponseRef := false

	for _, op := range assetV2Ops {
		v1Item, ok := paths["/v1"+op.opPath]
		require.Truef(t, ok, "the /v1 surface must publish the %s asset op", op.action)

		v2Item, ok := paths["/v2"+op.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s asset op", op.action)

		v1Op := operationForMethod(v1Item, op.method)
		require.NotNilf(t, v1Op, "the v1 %s asset op must carry a %s operation", op.action, op.method)

		v2Op := operationForMethod(v2Item, op.method)
		require.NotNilf(t, v2Op, "the v2 %s asset op must carry a %s operation", op.action, op.method)

		v1Req, v1Resp := assetOpBodyRefs(v1Op)
		v2Req, v2Resp := assetOpBodyRefs(v2Op)

		assert.Equalf(t, v1Req, v2Req,
			"the v2 %s asset op must name the SAME request-body schema as v1 (a straight mirror mints no new request type)", op.action)
		assert.ElementsMatchf(t, v1Resp, v2Resp,
			"the v2 %s asset op must name the SAME response-body component(s) as v1 (Huma dedups the reused Go type to one schema)", op.action)

		for _, ref := range v2Resp {
			if ref == "" {
				continue
			}

			sawSharedResponseRef = true

			assert.Falsef(t, strings.HasSuffix(ref, assetV2OperationSuffix),
				"the v2 %s asset op response ref %q must not name a %s-suffixed component — the v1 type is reused, not re-minted",
				op.action, ref, assetV2OperationSuffix)
		}
	}

	require.True(t, sawSharedResponseRef,
		"at least one asset op must reference a response-body component, or the reuse claim is vacuous")
}

// TestRegisterAssetV2Routes_MintsNoV2SchemaComponents guards against accidental new-type
// creation for the straight mirror: no asset schema component may carry the version suffix.
// TransactionV2 is a legitimate component (transaction v2 is NOT a straight mirror and DOES
// introduce its own types); the asset mirror must add no such twin.
func TestRegisterAssetV2Routes_MintsNoV2SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()
	schemas := doc.Components.Schemas.Map()

	// The components the asset ops actually name, gathered from the assembled document rather
	// than hardcoded, so a renamed asset type is followed here. The reused v1 type is
	// registered ONCE, so the suffixed twin of each must be absent.
	referenced := assetReferencedComponents(doc.Paths)
	require.Containsf(t, referenced, "Asset",
		"the asset ops must reference the Asset body component, or this test guards nothing")

	for name := range referenced {
		assert.NotContainsf(t, schemas, name+assetV2OperationSuffix,
			"no %s twin of the reused asset body component %q may be minted", assetV2OperationSuffix, name)
	}

	// The document-wide guard: no asset-named schema carries the V2 suffix. The "Asset" prefix
	// also spans the AssetRate family, which is intentional — asset-rate is mirrored to v2 as a
	// straight mirror too, so no AssetRate schema may sprout a V2 twin either.
	for name := range schemas {
		if !strings.HasPrefix(name, "Asset") {
			continue
		}

		assert.Falsef(t, strings.HasSuffix(name, assetV2OperationSuffix),
			"no asset schema component may carry the %s suffix; found %q", assetV2OperationSuffix, name)
	}
}
