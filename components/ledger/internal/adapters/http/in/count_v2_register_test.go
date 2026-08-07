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

// The transaction-count HEAD op is a single, standalone operation (no CRUD siblings) that the
// /v2 group mirrors from /v1. Unlike the asset/account-type mirrors it carries NO JSON body on
// either side — it is a pure header op (X-Total-Count on a bodiless 204) — so the reuse and
// no-new-schema invariants below are framed against an EMPTY body-ref footprint rather than a
// shared body component. The v2 twin is a STRAIGHT MIRROR (same handler method, same input/
// output types); its operationId is the v1 id with the version suffix appended, which is the
// only thing keeping the two twins from colliding as a duplicate operationId in the one document.
const (
	countTransactionV2OpPath          = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/metrics/count"
	countTransactionV1OperationID     = "countTransactionsByFilters"
	countTransactionV2OperationSuffix = "V2"
	countTransactionJSONMediaType     = "application/json"
)

// TestRegisterCountTransactionV2Routes_MirrorsV1UnderV2 asserts the single transaction-count
// HEAD operation is published on the assembled /v2 surface at the /v1 path shape prefixed with
// /v2, advertising the v1 operationId with the version suffix appended. It reads the REAL
// unified document (buildUnifiedHumaAPI) — the same huma.API the served contract and the
// committed dump come from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterCountTransactionV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	v2Key := "/v2" + countTransactionV2OpPath

	item, ok := paths[v2Key]
	require.Truef(t, ok, "the /v2 surface must publish the transaction-count HEAD op at %q", v2Key)

	operation := operationForMethod(item, http.MethodHead)
	require.NotNilf(t, operation, "%q must carry a HEAD operation", v2Key)

	assert.Equalf(t, countTransactionV1OperationID+countTransactionV2OperationSuffix, operation.OperationID,
		"the v2 transaction-count op must advertise the v1 id with the version suffix")
}

// countTransactionOpBodyRefs projects the count op onto the component $refs its JSON request
// body and its 2xx JSON response body name. The HEAD-count op names no JSON body at all, so
// both slots come back empty; returning the refs (not the schemas) makes the "mints no new
// type" claim observable as an empty, identical footprint on both version twins.
func countTransactionOpBodyRefs(op *huma.Operation) (reqRef string, respRefs []string) {
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content[countTransactionJSONMediaType]; ok && media.Schema != nil {
			reqRef = media.Schema.Ref
		}
	}

	for status, resp := range op.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}

		if media, ok := resp.Content[countTransactionJSONMediaType]; ok && media.Schema != nil {
			respRefs = append(respRefs, media.Schema.Ref)
		}
	}

	return reqRef, respRefs
}

// TestRegisterCountTransactionV2Routes_ReusesV1SchemaComponents proves the straight-mirror
// correctness claim for a body-less op: the /v2 count twin REUSES the v1 handler's input/output
// Go types, so its body-ref footprint is byte-identical to v1's — and for this pure header op
// that footprint is EMPTY on both sides. The explicit emptiness assertions are the non-vacuous
// anchor (replacing the CRUD mirrors' sawSharedResponseRef guard): a v2 twin that sprouted its
// own request or response body type would break the emptiness and turn this red.
func TestRegisterCountTransactionV2Routes_ReusesV1SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	v1Item, ok := paths["/v1"+countTransactionV2OpPath]
	require.True(t, ok, "the /v1 surface must publish the transaction-count HEAD op")

	v2Item, ok := paths["/v2"+countTransactionV2OpPath]
	require.True(t, ok, "the /v2 surface must publish the transaction-count HEAD op")

	v1Op := operationForMethod(v1Item, http.MethodHead)
	require.NotNil(t, v1Op, "the v1 transaction-count op must carry a HEAD operation")

	v2Op := operationForMethod(v2Item, http.MethodHead)
	require.NotNil(t, v2Op, "the v2 transaction-count op must carry a HEAD operation")

	v1Req, v1Resp := countTransactionOpBodyRefs(v1Op)
	v2Req, v2Resp := countTransactionOpBodyRefs(v2Op)

	assert.Equal(t, v1Req, v2Req,
		"the v2 transaction-count op must name the SAME request body as v1 (a straight mirror mints no new request type)")
	assert.ElementsMatch(t, v1Resp, v2Resp,
		"the v2 transaction-count op must name the SAME response-body component(s) as v1")

	assert.Empty(t, v1Req, "the transaction-count op is header-only: no v1 request body schema")
	assert.Empty(t, v2Req, "the transaction-count op is header-only: no v2 request body schema")
	assert.Empty(t, v1Resp, "the transaction-count op is header-only: no v1 JSON response body schema")
	assert.Empty(t, v2Resp, "the transaction-count op is header-only: no v2 JSON response body schema")
}

// TestRegisterCountTransactionV2Routes_MintsNoV2SchemaComponents guards against accidental
// new-type creation for the straight mirror. The transaction-count op is a pure HEAD/header op
// that names no request or response body, so mirroring it to /v2 must add ZERO schema
// components: no count-named schema, and in particular no V2-suffixed count twin (which a
// regression that gave the v2 op its own input/output body would create). The non-empty
// registry assertion anchors the guard against a vacuously empty document.
func TestRegisterCountTransactionV2Routes_MintsNoV2SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()
	schemas := doc.Components.Schemas.Map()

	require.NotEmpty(t, schemas, "the assembled document must register schema components, or this test guards nothing")

	for name := range schemas {
		if !strings.Contains(name, "CountTransaction") {
			continue
		}

		t.Errorf("the header-only transaction-count op must mint no schema component; found %q", name)
	}
}
