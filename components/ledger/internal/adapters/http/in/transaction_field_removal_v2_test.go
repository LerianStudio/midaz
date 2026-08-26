// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file locks the /v2 deprecated-field drop: the v2 transaction responses drop the
// transaction-level chartOfAccountsGroupName and route, and the operation-level chartOfAccounts
// and route, while /v1 keeps every one of them. The four assertions are (a) the v2 TransactionV2
// and OperationV2 OpenAPI components omit the fields, (b) the v2 read/update ops reference the v2
// types, (c) the v1 Transaction and Operation components still carry the fields, and (d) the
// newTransactionV2 wire encoding omits the fields at both levels.

const (
	v2OperationSchemaName        = "OperationV2"
	v1TransactionSchemaNameLock  = "Transaction"
	v1OperationSchemaNameLock    = "Operation"
	chartOfAccountsGroupNameJSON = "chartOfAccountsGroupName"
	chartOfAccountsJSON          = "chartOfAccounts"
	routeJSON                    = "route"
)

// schemaByName resolves a registered component schema by its exact name, failing the test when
// the document does not carry it.
func schemaByName(t *testing.T, doc *huma.OpenAPI, name string) *huma.Schema {
	t.Helper()

	require.NotNil(t, doc.Components, "document must carry components")
	require.NotNil(t, doc.Components.Schemas, "document must carry a schema registry")

	schema, ok := doc.Components.Schemas.Map()[name]
	require.Truef(t, ok, "the unified document must register the %q component", name)
	require.NotNilf(t, schema, "%q must resolve to a non-nil schema", name)

	return schema
}

// TestV2TransactionOperationSchemas_OmitDeprecatedFields proves the v2 wire components drop the
// four deprecated fields: TransactionV2 carries neither chartOfAccountsGroupName nor route, and
// OperationV2 carries neither chartOfAccounts nor route.
func TestV2TransactionOperationSchemas_OmitDeprecatedFields(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	txV2 := schemaByName(t, doc, v2TransactionSchemaName)
	assert.NotContains(t, txV2.Properties, chartOfAccountsGroupNameJSON,
		"%s must not carry the deprecated transaction-level chartOfAccountsGroupName", v2TransactionSchemaName)
	assert.NotContains(t, txV2.Properties, routeJSON,
		"%s must not carry the deprecated transaction-level route", v2TransactionSchemaName)

	opV2 := schemaByName(t, doc, v2OperationSchemaName)
	assert.NotContains(t, opV2.Properties, chartOfAccountsJSON,
		"%s must not carry the deprecated operation-level chartOfAccounts", v2OperationSchemaName)
	assert.NotContains(t, opV2.Properties, routeJSON,
		"%s must not carry the deprecated operation-level route", v2OperationSchemaName)
}

// TestV1TransactionOperationSchemas_KeepDeprecatedFields proves the change is scoped to /v2: the
// v1 Transaction and Operation components still carry all four deprecated fields, so /v1 responses
// and the operations endpoints are untouched.
func TestV1TransactionOperationSchemas_KeepDeprecatedFields(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	txV1 := schemaByName(t, doc, v1TransactionSchemaNameLock)
	assert.Contains(t, txV1.Properties, chartOfAccountsGroupNameJSON,
		"the v1 %s must keep chartOfAccountsGroupName", v1TransactionSchemaNameLock)
	assert.Contains(t, txV1.Properties, routeJSON,
		"the v1 %s must keep route", v1TransactionSchemaNameLock)

	opV1 := schemaByName(t, doc, v1OperationSchemaNameLock)
	assert.Contains(t, opV1.Properties, chartOfAccountsJSON,
		"the v1 %s must keep chartOfAccounts", v1OperationSchemaNameLock)
	assert.Contains(t, opV1.Properties, routeJSON,
		"the v1 %s must keep route", v1OperationSchemaNameLock)
}

// TestV2ReadUpdateOps_ReferenceTransactionV2 proves the three mirrored v2 ops (get-by-id, list,
// PATCH update) answer with the v2 wire shape: the get-by-id and update 2xx bodies $ref
// TransactionV2 directly, and the list op's items resolve to TransactionV2. A regression that
// reused the v1 transaction.Transaction body would point these at the v1 "Transaction" component
// and turn the assertions red.
func TestV2ReadUpdateOps_ReferenceTransactionV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	const refPrefix = "#/components/schemas/"
	wantTxV2Ref := refPrefix + v2TransactionSchemaName

	// get-by-id and update bodies must $ref TransactionV2 directly.
	for _, tc := range []struct {
		action string
		method string
		opPath string
	}{
		{"getByID", http.MethodGet, transactionMirrorIDPath},
		{"update", http.MethodPatch, transactionMirrorIDPath},
	} {
		item, ok := doc.Paths["/v2"+tc.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s transaction op", tc.action)

		op := operationForMethod(item, tc.method)
		require.NotNilf(t, op, "the v2 %s op must carry a %s operation", tc.action, tc.method)

		_, respRefs := transactionMirrorOpBodyRefs(op)
		assert.Containsf(t, respRefs, wantTxV2Ref,
			"the v2 %s op response must $ref %s", tc.action, v2TransactionSchemaName)
	}

	// The list op's response body items must resolve to TransactionV2.
	listItem, ok := doc.Paths["/v2"+transactionMirrorListPath]
	require.True(t, ok, "the /v2 surface must publish the list transaction op")

	listOp := operationForMethod(listItem, http.MethodGet)
	require.NotNil(t, listOp, "the v2 list op must carry a GET operation")

	_, listRespRefs := transactionMirrorOpBodyRefs(listOp)
	require.NotEmpty(t, listRespRefs, "the v2 list op must carry a JSON response body component")

	listBody := doc.Components.Schemas.SchemaFromRef(listRespRefs[0])
	require.NotNil(t, listBody, "the v2 list response body component must resolve from its ref")

	itemsSchema, ok := listBody.Properties["items"]
	require.True(t, ok, "the v2 list body must carry an items property")
	require.NotNil(t, itemsSchema, "the items property schema must be present")
	require.NotNil(t, itemsSchema.Items, "the items property must be an array with an element schema")
	assert.Equalf(t, wantTxV2Ref, itemsSchema.Items.Ref,
		"the v2 list op items must $ref %s", v2TransactionSchemaName)
}

// TestNewTransactionV2_OmitsDeprecatedWireFields proves the wire encoding of newTransactionV2's
// output drops the four deprecated fields: chartOfAccountsGroupName and route at the transaction
// level, and chartOfAccounts and route on every nested operation.
func TestNewTransactionV2_OmitsDeprecatedWireFields(t *testing.T) {
	t.Parallel()

	got := newTransactionV2(buildCanonicalTransactionFixture())

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))

	assert.NotContains(t, asMap, chartOfAccountsGroupNameJSON,
		"the v2 wire body must not carry the deprecated chartOfAccountsGroupName")
	assert.NotContains(t, asMap, routeJSON,
		"the v2 wire body must not carry the deprecated transaction-level route")

	ops, ok := asMap["operations"].([]any)
	require.True(t, ok, "the v2 wire body must carry an operations array")
	require.NotEmpty(t, ops, "the fixture must produce at least one operation")

	firstOp, ok := ops[0].(map[string]any)
	require.True(t, ok, "each operation must encode as a JSON object")

	assert.NotContains(t, firstOp, chartOfAccountsJSON,
		"the v2 operation wire body must not carry the deprecated chartOfAccounts")
	assert.NotContains(t, firstOp, routeJSON,
		"the v2 operation wire body must not carry the deprecated operation-level route")
}
