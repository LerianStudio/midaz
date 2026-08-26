// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accountV2Ops enumerates the eight account operations the /v2 group mirrors from /v1:
// the HTTP method, the group-relative op path the Huma document publishes (the shared
// contract prepends "/v2"), and the /v1 operationId the op reuses. Unlike account-type,
// account carries the alias/external reads and the HEAD-count op, so the full CRUD +
// list + alias + external + count set is the whole surface. Every op's operationId is
// the v1 id with the version suffix appended; that suffix is the only thing that keeps
// the two twins from colliding as a duplicate operationId in the one document.
//
// The account surface is NOT a straight mirror on the RESPONSE side. The holder seam is
// /v2 only, so the ops whose 2xx body IS an account project onto AccountV1 on /v1
// (holderId + holderCheckSkipped withheld) and onto the canonical Account on /v2.
// accountBody marks those ops. Everything else — the request bodies, the paginated list
// envelope, the bodiless DELETE and HEAD count — is identical across the two contracts.
var accountV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
	accountBody   bool
}{
	{action: "create", method: http.MethodPost, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts", v1OperationID: "createAccount", accountBody: true},
	{action: "list", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts", v1OperationID: "listAccounts"},
	{action: "getByID", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", v1OperationID: "getAccountByID", accountBody: true},
	{action: "getByAlias", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}", v1OperationID: "getAccountByAlias", accountBody: true},
	{action: "getExternalByCode", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/external/{code}", v1OperationID: "getAccountExternalByCode", accountBody: true},
	{action: "update", method: http.MethodPatch, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", v1OperationID: "updateAccount", accountBody: true},
	{action: "delete", method: http.MethodDelete, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", v1OperationID: "deleteAccount"},
	{action: "count", method: http.MethodHead, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/metrics/count", v1OperationID: "countAccounts"},
}

// accountV1ComponentName / accountV2ComponentName are the two account body components the
// two contracts publish. The /v1 projection keeps the CANONICAL "Account" name so the
// generated v1 SDKs bind to the type they already have (ledgerSchemaNamer pins it), which
// pushes the holder-seam-bearing shape onto "AccountV2". Both are spelled literally so a
// rename shows up here as the contract change the client SDKs would feel.
const (
	accountV1ComponentName = "#/components/schemas/Account"
	accountV2ComponentName = "#/components/schemas/AccountV2"
)

// accountV2OperationSuffix is the version suffix a v2 twin appends to its v1 operationId.
// It is spelled literally here rather than read from v2OpSuffix so a rename of the
// production constant surfaces as a contract change the client SDKs would feel, not a
// silently-tracking test.
const accountV2OperationSuffix = "V2"

// accountJSONMediaType is the content type the account ops publish their request and
// response bodies under. The reuse invariant below is asserted only over these bodies.
const accountJSONMediaType = "application/json"

// TestRegisterAccountV2Routes_MirrorsV1UnderV2 asserts each of the eight account operations
// is published on the assembled /v2 surface at the /v1 path shape prefixed with /v2,
// advertising the v1 operationId with the version suffix appended. It reads the REAL unified
// document (buildUnifiedHumaAPI) — the same huma.API the served contract and the committed
// dump come from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterAccountV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range accountV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s account op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+accountV2OperationSuffix, operation.OperationID,
				"the v2 %s account op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// accountOpBodyRefs projects an operation onto the component $refs its JSON request body and
// its 2xx JSON response body name. A body Huma describes inline (the opaque RawBody request
// schema the create/update ops carry) has no $ref, so its slot comes back "". A bodiless op
// (the HEAD count) names no component either. Returning the refs — not the schemas — is the
// point: reuse of a v1 Go type is observable precisely as the v2 twin pointing at the SAME
// "#/components/schemas/<Name>" string.
func accountOpBodyRefs(op *huma.Operation) (reqRef string, respRefs []string) {
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content[accountJSONMediaType]; ok && media.Schema != nil {
			reqRef = media.Schema.Ref
		}
	}

	for status, resp := range op.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}

		if media, ok := resp.Content[accountJSONMediaType]; ok && media.Schema != nil {
			respRefs = append(respRefs, media.Schema.Ref)
		}
	}

	return reqRef, respRefs
}

// accountReferencedComponents gathers the base component names ("#/components/schemas/"
// prefix stripped) the account ops name in their JSON bodies, read off the assembled
// document so a rename of an account type is followed here automatically. It walks both /v1
// and /v2 twins, so the set spans the AccountV1 projection and its canonical twin.
func accountReferencedComponents(paths map[string]*huma.PathItem) map[string]bool {
	const refPrefix = "#/components/schemas/"

	refs := make(map[string]bool)

	collect := func(ref string) {
		if name, ok := strings.CutPrefix(ref, refPrefix); ok {
			refs[name] = true
		}
	}

	for _, op := range accountV2Ops {
		for _, prefix := range []string{"/v1", "/v2"} {
			item, ok := paths[prefix+op.opPath]
			if !ok {
				continue
			}

			operation := operationForMethod(item, op.method)
			if operation == nil {
				continue
			}

			reqRef, respRefs := accountOpBodyRefs(operation)

			collect(reqRef)

			for _, r := range respRefs {
				collect(r)
			}
		}
	}

	return refs
}

// TestRegisterAccountV2Routes_SplitsAccountBodyByVersion proves the holder seam's contract
// boundary as the assembled document publishes it: the /v1 account ops answer with the
// AccountV1 component (holderId + holderCheckSkipped withheld) and their /v2 twins with the
// canonical Account component, while every REQUEST body and every non-account response stays
// byte-identical across the two contracts. It reads the REAL unified document, the same
// huma.API the served contract and the committed dump come from.
//
// Were the /v1 ops to fall back to the canonical Account, the two holder keys would reappear
// on a contract that never carried them and the equality below would turn red.
func TestRegisterAccountV2Routes_SplitsAccountBodyByVersion(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	// Guards the assertions below against vacuously passing on a document where every ref
	// came back "": at least one account op must actually name an account body component.
	sawSplitResponseRef := false

	for _, op := range accountV2Ops {
		v1Item, ok := paths["/v1"+op.opPath]
		require.Truef(t, ok, "the /v1 surface must publish the %s account op", op.action)

		v2Item, ok := paths["/v2"+op.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s account op", op.action)

		v1Op := operationForMethod(v1Item, op.method)
		require.NotNilf(t, v1Op, "the v1 %s account op must carry a %s operation", op.action, op.method)

		v2Op := operationForMethod(v2Item, op.method)
		require.NotNilf(t, v2Op, "the v2 %s account op must carry a %s operation", op.action, op.method)

		v1Req, v1Resp := accountOpBodyRefs(v1Op)
		v2Req, v2Resp := accountOpBodyRefs(v2Op)

		assert.Equalf(t, v1Req, v2Req,
			"the v2 %s account op must name the SAME request-body schema as v1 (the holder split is response-only)", op.action)

		if !op.accountBody {
			assert.ElementsMatchf(t, v1Resp, v2Resp,
				"the %s account op carries no account body, so both contracts must name the SAME response component(s)", op.action)

			continue
		}

		sawSplitResponseRef = true

		assert.ElementsMatchf(t, []string{accountV1ComponentName}, v1Resp,
			"the v1 %s account op must answer with the holder-withholding Account component", op.action)
		assert.ElementsMatchf(t, []string{accountV2ComponentName}, v2Resp,
			"the v2 %s account op must answer with the holder-bearing AccountV2 component", op.action)
	}

	require.True(t, sawSplitResponseRef,
		"at least one account op must carry an account response body, or the split claim is vacuous")
}

// TestAccountV1Component_WithholdsHolderKeys locks the /v1 account projection at the schema
// level: the published "Account" component must advertise neither holderId nor
// holderCheckSkipped, while "AccountV2" must still advertise both. The pair is what makes
// the withholding a projection rather than a removal — /v2 clients keep the holder seam.
//
// It serializes the components rather than reading Schema.Properties, because Huma keeps a
// hidden property in its in-memory map and drops it only at marshal time — so inspecting
// the map directly would report keys clients never see.
func TestAccountV1Component_WithholdsHolderKeys(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	v1Props := marshalledSchemaProperties(t, schemaByName(t, doc, "Account"))
	v2Props := marshalledSchemaProperties(t, schemaByName(t, doc, "AccountV2"))

	// Guard against a vacuous pass on an empty component.
	assert.Contains(t, v1Props, "id", "the v1 Account component must document the embedded fields")

	for _, key := range accountHolderKeys {
		assert.NotContainsf(t, v1Props, key, "the /v1 Account contract must not advertise %q", key)
		assert.Containsf(t, v2Props, key, "the AccountV2 contract must still advertise %q", key)
	}
}

// marshalledSchemaProperties serializes a component and returns the property set the
// document actually publishes.
func marshalledSchemaProperties(t *testing.T, schema *huma.Schema) map[string]any {
	t.Helper()

	raw, err := json.Marshal(schema)
	require.NoError(t, err, "serialize schema component")

	var doc struct {
		Properties map[string]any `json:"properties"`
	}

	require.NoError(t, json.Unmarshal(raw, &doc), "decode serialized schema component")

	return doc.Properties
}

// TestAccountSchemaComponents_PinV1CanonicalName guards the DIRECTION of the account version
// split, which is the part a rename would silently invert. The /v1 projection owns the
// canonical "Account" name — that is what keeps the generated v1 SDKs binding to the type
// they already have — so the holder-bearing shape is the one that carries the version
// suffix, and the Go type name AccountV1 must not leak into the published document.
//
// Exactly one versioned account twin may exist: AccountV2. Any other V2-suffixed
// account-family component (AccountTypeV2, AccountRuleV2, …) means a sibling resource
// accidentally minted a twin instead of reusing its published type.
func TestAccountSchemaComponents_PinV1CanonicalName(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()
	schemas := doc.Components.Schemas.Map()

	require.Contains(t, schemas, "Account",
		"the /v1 body must stay published as the \"Account\" component")
	require.Contains(t, schemas, "AccountV2",
		"the holder-bearing body must be published as the \"AccountV2\" component")
	require.NotContains(t, schemas, "AccountV1",
		"the Go type name AccountV1 must not leak into the published contract")

	// The components the account ops actually name, gathered from the assembled document
	// rather than hardcoded, so a renamed account type is followed here.
	referenced := accountReferencedComponents(doc.Paths)
	require.Containsf(t, referenced, "Account",
		"the account ops must reference the Account body component, or this test guards nothing")
	require.Containsf(t, referenced, "AccountV2",
		"the account ops must reference the AccountV2 body component, or the split is not published")

	// AccountV2 is the ONE legitimate versioned account twin. TransactionV2 is the sibling
	// precedent on the transaction surface; no other Account* family member may mint one.
	for name := range schemas {
		if !strings.HasPrefix(name, "Account") || name == "AccountV2" {
			continue
		}

		assert.Falsef(t, strings.HasSuffix(name, accountV2OperationSuffix),
			"AccountV2 is the only account schema component that may carry the %s suffix; found %q",
			accountV2OperationSuffix, name)
	}
}
