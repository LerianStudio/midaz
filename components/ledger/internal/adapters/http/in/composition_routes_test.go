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

// compositionV2Ops enumerates the single composition operation the /v2 group publishes:
// the HTTP method, the group-relative op path the Huma document publishes (the shared
// contract prepends "/v2"), and the base operationId the op carries before the version
// suffix. Composition is a single holder-account orchestration route, so this one-row table
// is the whole surface. The published operationId is the base id with the version suffix
// appended; the suffix is what keeps the id distinct within the shared document.
var compositionV2Ops = []struct {
	action          string
	method          string
	opPath          string
	baseOperationID string
}{
	{action: "createHolderAccount", method: http.MethodPost, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/holders/{id}/accounts", baseOperationID: "createHolderAccount"},
}

// compositionV2OperationSuffix is the version suffix the v2 op appends to its base operationId.
// It is spelled literally here rather than read from v2OpSuffix so a rename of the
// production constant surfaces as a contract change the client SDKs would feel, not a
// silently-tracking test.
const compositionV2OperationSuffix = "V2"

// compositionJSONMediaType is the content type the composition op publishes its request and
// response bodies under. The reuse invariant below is asserted only over these bodies.
const compositionJSONMediaType = "application/json"

// TestRegisterCompositionV2Routes_PublishesUnderV2 asserts the composition operation is
// published on the assembled /v2 surface at its op path prefixed with /v2, advertising the
// base operationId with the version suffix appended. It reads the REAL unified document
// (buildUnifiedHumaAPI) — the same huma.API the served contract and the committed dump come
// from — so it proves the op against the mount a client hits.
func TestRegisterCompositionV2Routes_PublishesUnderV2(t *testing.T) {
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

			assert.Equalf(t, op.baseOperationID+compositionV2OperationSuffix, operation.OperationID,
				"the v2 %s composition op must advertise the base id with the version suffix", op.action)
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
// so a rename of the composition response type is followed here automatically.
func compositionReferencedComponents(paths map[string]*huma.PathItem) map[string]bool {
	const refPrefix = "#/components/schemas/"

	refs := make(map[string]bool)

	collect := func(ref string) {
		if name, ok := strings.CutPrefix(ref, refPrefix); ok {
			refs[name] = true
		}
	}

	for _, op := range compositionV2Ops {
		item, ok := paths["/v2"+op.opPath]
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

	return refs
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
