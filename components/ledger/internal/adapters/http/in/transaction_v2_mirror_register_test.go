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

// transactionMirrorV2Ops enumerates the three transaction ops the /v2 group publishes as reads/
// update: the PATCH update and the two reads (get-by-id + list). Each points at a dedicated /v2
// handler method that calls the SAME core its v1 twin does but answers with the /v2 wire shape
// (TransactionV2); its operationId is the v1 id with the version suffix appended. The GET and PATCH
// twins share transactionMirrorIDPath — one path key, two methods.
//
// The legacy-create paths (json/inflow/outflow/annotation) are NOT mirrored onto /v2: they are
// served on /v1 only, and the /v2 transaction create surface is the flat-body direct/hold/
// block/unblock model in transaction_routes_v2.go. block/unblock create and commit/cancel/
// revert lifecycle are likewise absent, because those already carry v2 operationIds via
// RegisterTransactionV2Routes and mirroring them would collide as a duplicate operationId in the
// one document. The retired legacy-create twins are pinned as absent by
// transactionMirrorV2RemovedCreateActions below.
var transactionMirrorV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "update", method: http.MethodPatch, opPath: transactionMirrorIDPath, v1OperationID: "updateTransaction"},
	{action: "getByID", method: http.MethodGet, opPath: transactionMirrorIDPath, v1OperationID: "getTransaction"},
	{action: "list", method: http.MethodGet, opPath: transactionMirrorListPath, v1OperationID: "getAllTransactions"},
}

// transactionMirrorV2RemovedCreateActions enumerates the four legacy-create transaction ops that
// are served on /v1 ONLY and MUST NOT be mirrored onto /v2. Each entry pairs the /v1-relative
// path with the v1 operationId whose "+V2" twin must be absent from the unified document.
var transactionMirrorV2RemovedCreateActions = []struct {
	action        string
	opPath        string
	v1OperationID string
}{
	{action: "createJSON", opPath: transactionMirrorListPath + "/json", v1OperationID: "createTransactionJSON"},
	{action: "createInflow", opPath: transactionMirrorListPath + "/inflow", v1OperationID: "createTransactionInflow"},
	{action: "createOutflow", opPath: transactionMirrorListPath + "/outflow", v1OperationID: "createTransactionOutflow"},
	{action: "createAnnotation", opPath: transactionMirrorListPath + "/annotation", v1OperationID: "createTransactionAnnotation"},
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

// TestRegisterTransactionMirrorV2Routes_UpdateReusesV1RequestSchema proves the request side of the
// PATCH update stays a straight mirror: the v2 update op reuses the v1
// transaction.UpdateTransactionInput request type, so Huma dedups it to ONE component and the v2
// op's request-body $ref is byte-identical to the v1 op's. Only the RESPONSE diverges to the /v2
// wire shape. The two GET reads carry no request body, so this claim is scoped to the update op.
func TestRegisterTransactionMirrorV2Routes_UpdateReusesV1RequestSchema(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	v1Item, ok := paths["/v1"+transactionMirrorIDPath]
	require.True(t, ok, "the /v1 surface must publish the update transaction op")

	v2Item, ok := paths["/v2"+transactionMirrorIDPath]
	require.True(t, ok, "the /v2 surface must publish the update transaction op")

	v1Op := operationForMethod(v1Item, http.MethodPatch)
	require.NotNil(t, v1Op, "the v1 update op must carry a PATCH operation")

	v2Op := operationForMethod(v2Item, http.MethodPatch)
	require.NotNil(t, v2Op, "the v2 update op must carry a PATCH operation")

	v1Req, _ := transactionMirrorOpBodyRefs(v1Op)
	v2Req, _ := transactionMirrorOpBodyRefs(v2Op)

	require.NotEmpty(t, v1Req, "the v1 update op must $ref a request-body component, or the reuse claim is vacuous")
	assert.Equal(t, v1Req, v2Req,
		"the v2 update op must reuse the v1 request-body component (only the response diverges to the /v2 shape)")
}

// TestRegisterTransactionMirrorV2Routes_ResponsesDoNotReferenceV1Components proves the response
// side diverges to the /v2 wire shape: none of the three reads/update v2 ops reference the v1
// "Transaction" or "Pagination" response components, while their v1 twins still do. This is the
// negative complement to TestV2ReadUpdateOps_ReferenceTransactionV2, which positively locks the
// TransactionV2 references. ("Pagination" is Huma's component name for the v1 list body type
// pkgHTTP.Pagination — Huma names by Go type, ignoring the swaggo @name annotation.)
func TestRegisterTransactionMirrorV2Routes_ResponsesDoNotReferenceV1Components(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	const refPrefix = "#/components/schemas/"

	v1ResponseComponents := map[string]bool{"Transaction": true, "Pagination": true}

	sawV2ResponseRef := false

	for _, op := range transactionMirrorV2Ops {
		v1Item, ok := paths["/v1"+op.opPath]
		require.Truef(t, ok, "the /v1 surface must publish the %s transaction op", op.action)

		v2Item, ok := paths["/v2"+op.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s transaction op", op.action)

		v1Op := operationForMethod(v1Item, op.method)
		require.NotNilf(t, v1Op, "the v1 %s transaction op must carry a %s operation", op.action, op.method)

		v2Op := operationForMethod(v2Item, op.method)
		require.NotNilf(t, v2Op, "the v2 %s transaction op must carry a %s operation", op.action, op.method)

		_, v1Resp := transactionMirrorOpBodyRefs(v1Op)
		_, v2Resp := transactionMirrorOpBodyRefs(v2Op)

		v1RefsV1Component := false

		for _, ref := range v1Resp {
			if name, ok := strings.CutPrefix(ref, refPrefix); ok && v1ResponseComponents[name] {
				v1RefsV1Component = true
			}
		}

		assert.Truef(t, v1RefsV1Component,
			"the v1 %s transaction op must still reference a v1 response component", op.action)

		for _, ref := range v2Resp {
			name, ok := strings.CutPrefix(ref, refPrefix)
			if !ok {
				continue
			}

			sawV2ResponseRef = true

			assert.Falsef(t, v1ResponseComponents[name],
				"the v2 %s transaction op must not reference the v1 response component %q", op.action, name)
		}
	}

	require.True(t, sawV2ResponseRef,
		"at least one reads/update v2 op must reference a response-body component, or this test guards nothing")
}

// TestRegisterTransactionMirrorV2Routes_LegacyCreateOpsRetired asserts the four legacy-create
// transaction ops (json/inflow/outflow/annotation) are absent from the /v2 surface: neither their
// "+V2" operationId nor a POST at their /v2 path may exist. It reads the REAL unified document
// (buildUnifiedHumaAPI), so it proves the retirement against the mount a client hits. The v1
// originals are unaffected — they keep serving on /v1 — which the test also confirms.
func TestRegisterTransactionMirrorV2Routes_LegacyCreateOpsRetired(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	// Every operationId published anywhere in the unified document.
	publishedIDs := make(map[string]bool)

	for _, item := range paths {
		for _, op := range operationsOf(item) {
			publishedIDs[op.OperationID] = true
		}
	}

	for _, op := range transactionMirrorV2RemovedCreateActions {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2ID := op.v1OperationID + transactionMirrorV2OperationSuffix

			assert.Falsef(t, publishedIDs[v2ID],
				"the legacy-create v2 twin %q must be retired from the unified contract", v2ID)

			assert.Truef(t, publishedIDs[op.v1OperationID],
				"the v1 %s transaction op (%q) must still be published on /v1", op.action, op.v1OperationID)

			if v2Item, ok := paths["/v2"+op.opPath]; ok {
				assert.Nilf(t, operationForMethod(v2Item, http.MethodPost),
					"the /v2 surface must not publish a POST at %q: the legacy-create v2 twin is retired", "/v2"+op.opPath)
			}
		})
	}
}
