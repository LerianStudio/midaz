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

// transactionMirrorListPath and transactionMirrorIDPath are the organization/ledger-scoped
// collection and item paths the seven mirrored transaction ops hang off, in OpenAPI template
// syntax. They are the v1 transaction paths verbatim (RegisterTransactionRoutes); the shared
// contract prepends "/v2" when the ops are mounted on the /v2 group.
const (
	transactionMirrorListPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions"
	transactionMirrorIDPath   = transactionMirrorListPath + "/{transaction_id}"
)

// transactionMirrorV2Ops enumerates the seven v1 transaction ops the /v2 group mirrors:
// the four legacy-create twins (json/inflow/outflow/annotation), the PATCH update, and the two
// reads (get-by-id + list). It is NOT the whole v1 transaction surface — block/unblock create
// and commit/cancel/revert lifecycle are DELIBERATELY absent, because those already carry v2
// operationIds via RegisterTransactionV2Routes and mirroring them would collide as a duplicate
// operationId in the one document. Each twin is a STRAIGHT MIRROR: same handler method, same
// input/output types, so its operationId is the v1 id with the version suffix appended. The GET
// and PATCH twins share transactionMirrorIDPath — one path key, two methods.
var transactionMirrorV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "createJSON", method: http.MethodPost, opPath: transactionMirrorListPath + "/json", v1OperationID: "createTransactionJSON"},
	{action: "createInflow", method: http.MethodPost, opPath: transactionMirrorListPath + "/inflow", v1OperationID: "createTransactionInflow"},
	{action: "createOutflow", method: http.MethodPost, opPath: transactionMirrorListPath + "/outflow", v1OperationID: "createTransactionOutflow"},
	{action: "createAnnotation", method: http.MethodPost, opPath: transactionMirrorListPath + "/annotation", v1OperationID: "createTransactionAnnotation"},
	{action: "update", method: http.MethodPatch, opPath: transactionMirrorIDPath, v1OperationID: "updateTransaction"},
	{action: "getByID", method: http.MethodGet, opPath: transactionMirrorIDPath, v1OperationID: "getTransaction"},
	{action: "list", method: http.MethodGet, opPath: transactionMirrorListPath, v1OperationID: "getAllTransactions"},
}

// transactionMirrorV2OperationSuffix is the version suffix a v2 twin appends to its v1
// operationId. It is spelled literally rather than read from routeOpSuffixV2 so a rename of the
// production constant surfaces as a contract change client SDKs would feel, not a silently-
// tracking test.
const transactionMirrorV2OperationSuffix = "V2"

// transactionMirrorJSONMediaType is the content type the mirrored ops publish their request and
// response bodies under. The reuse invariant below is asserted only over these bodies.
const transactionMirrorJSONMediaType = "application/json"

// TestRegisterTransactionMirrorV2Routes_MirrorsV1UnderV2 asserts each of the seven mirrored
// transaction ops is published on the assembled /v2 surface at its v1 path shape prefixed with
// /v2, advertising the v1 operationId with the version suffix appended. It reads the REAL unified
// document (buildUnifiedHumaAPI) — the same huma.API the served contract and the committed dump
// come from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterTransactionMirrorV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range transactionMirrorV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s transaction op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+transactionMirrorV2OperationSuffix, operation.OperationID,
				"the v2 %s transaction op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// transactionMirrorOpBodyRefs projects an operation onto the component $refs its JSON request
// body and its 2xx JSON response body name. The legacy-create/update ops carry RawBody, whose
// request schema Huma describes inline with no $ref, so that slot comes back "". Returning the
// refs — not the schemas — is the point: reuse of a v1 Go type is observable precisely as the v2
// twin pointing at the SAME "#/components/schemas/<Name>" string.
func transactionMirrorOpBodyRefs(op *huma.Operation) (reqRef string, respRefs []string) {
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content[transactionMirrorJSONMediaType]; ok && media.Schema != nil {
			reqRef = media.Schema.Ref
		}
	}

	for status, resp := range op.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}

		if media, ok := resp.Content[transactionMirrorJSONMediaType]; ok && media.Schema != nil {
			respRefs = append(respRefs, media.Schema.Ref)
		}
	}

	return reqRef, respRefs
}

// TestRegisterTransactionMirrorV2Routes_ReusesV1SchemaComponents proves the core correctness
// claim of the straight-mirror approach: the /v2 twin REUSES the v1 request/response Go types,
// so Huma's registry dedups them to ONE schema component and the v2 op's body $ref is byte-
// identical to the v1 op's. It reads the REAL unified document.
//
// Were a v2 twin to mint its own type for any body, its op would $ref a different (V2-named)
// component and the equality below would turn red.
func TestRegisterTransactionMirrorV2Routes_ReusesV1SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	// Guards the assertions below against vacuously passing on a document where every ref came
	// back "": at least one mirrored op must actually name a response-body component.
	sawSharedResponseRef := false

	for _, op := range transactionMirrorV2Ops {
		v1Item, ok := paths["/v1"+op.opPath]
		require.Truef(t, ok, "the /v1 surface must publish the %s transaction op", op.action)

		v2Item, ok := paths["/v2"+op.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s transaction op", op.action)

		v1Op := operationForMethod(v1Item, op.method)
		require.NotNilf(t, v1Op, "the v1 %s transaction op must carry a %s operation", op.action, op.method)

		v2Op := operationForMethod(v2Item, op.method)
		require.NotNilf(t, v2Op, "the v2 %s transaction op must carry a %s operation", op.action, op.method)

		v1Req, v1Resp := transactionMirrorOpBodyRefs(v1Op)
		v2Req, v2Resp := transactionMirrorOpBodyRefs(v2Op)

		assert.Equalf(t, v1Req, v2Req,
			"the v2 %s transaction op must name the SAME request-body schema as v1 (a straight mirror mints no new request type)", op.action)
		assert.ElementsMatchf(t, v1Resp, v2Resp,
			"the v2 %s transaction op must name the SAME response-body component(s) as v1 (Huma dedups the reused Go type to one schema)", op.action)

		for _, ref := range v2Resp {
			if ref != "" {
				sawSharedResponseRef = true
			}
		}
	}

	require.True(t, sawSharedResponseRef,
		"at least one mirrored transaction op must reference a response-body component, or the reuse claim is vacuous")
}

// TestRegisterTransactionMirrorV2Routes_MintsNoV2SchemaComponents guards against accidental new-
// type creation for the straight mirror: every component the mirrored ops NAME must be an
// unsuffixed v1 component, so none of their refs may carry the version suffix.
//
// The check is scoped to the components the mirror ops actually reference, NOT to every
// Transaction-named schema in the document: the non-mirror v2 create/lifecycle ops legitimately
// mint TransactionV2 / CreateTransactionV2Input / V2LegInput, so a document-wide "no Transaction*
// carries V2" assertion would be wrong. The mirror twins must reuse the v1 (unsuffixed) types and
// so must not point at any of those V2 components.
func TestRegisterTransactionMirrorV2Routes_MintsNoV2SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	const refPrefix = "#/components/schemas/"

	referenced := make(map[string]bool)

	for _, op := range transactionMirrorV2Ops {
		item, ok := paths["/v2"+op.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s transaction op", op.action)

		operation := operationForMethod(item, op.method)
		require.NotNilf(t, operation, "the v2 %s transaction op must carry a %s operation", op.action, op.method)

		reqRef, respRefs := transactionMirrorOpBodyRefs(operation)
		for _, ref := range append(respRefs, reqRef) {
			if name, ok := strings.CutPrefix(ref, refPrefix); ok {
				referenced[name] = true
			}
		}
	}

	require.NotEmpty(t, referenced,
		"the mirrored transaction ops must reference at least one body component, or this test guards nothing")

	for name := range referenced {
		assert.Falsef(t, strings.HasSuffix(name, transactionMirrorV2OperationSuffix),
			"the mirrored transaction ops must reuse the unsuffixed v1 component, not a %s twin; found %q",
			transactionMirrorV2OperationSuffix, name)
	}
}
