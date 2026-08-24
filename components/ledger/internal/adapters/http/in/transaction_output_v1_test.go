// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// skipFieldKeys are the two response keys that are /v2 ONLY. A v1 client parses a body
// it was written against, so neither may appear on /v1 — in the body OR in the
// published schema, since a contract that advertises a key the body never sends is its
// own defect.
var skipFieldKeys = []string{"feesSkipped", "tracerSkipped"}

// transactionWithSkips is a transaction carrying BOTH skip flags set, so an assertion
// that a key is absent cannot pass merely because the value was the zero value.
func transactionWithSkips() *transaction.Transaction {
	return &transaction.Transaction{
		ID:            "00000000-0000-0000-0000-000000000001",
		AssetCode:     "BRL",
		FeesSkipped:   true,
		TracerSkipped: true,
	}
}

// TestTransactionV1_BodyWithholdsSkipFields asserts the /v1 projection emits neither
// skip key while the /v2 projection of the SAME transaction emits both. Asserting the
// pair together is what makes this a version boundary rather than a field deletion.
func TestTransactionV1_BodyWithholdsSkipFields(t *testing.T) {
	t.Parallel()

	tran := transactionWithSkips()

	v1, err := json.Marshal(newTransactionV1(tran))
	require.NoError(t, err)

	v2, err := json.Marshal(newTransactionV2(tran))
	require.NoError(t, err)

	var v1Body, v2Body map[string]any
	require.NoError(t, json.Unmarshal(v1, &v1Body))
	require.NoError(t, json.Unmarshal(v2, &v2Body))

	// Guard against a vacuous pass: an empty body would satisfy every NotContains.
	assert.Contains(t, v1Body, "id", "the v1 body must still carry the embedded fields")

	for _, key := range skipFieldKeys {
		assert.NotContainsf(t, v1Body, key, "/v1 must not publish %q", key)
		assert.Containsf(t, v2Body, key, "/v2 must still publish %q", key)
	}
}

// TestTransactionV1_PreservesEmbeddedFields asserts the shadow costs nothing but the two
// keys: every other field of the canonical transaction still reaches the v1 body. The
// shadow works by winning a field-name conflict, so a mistake there silently drops
// siblings rather than failing loudly.
func TestTransactionV1_PreservesEmbeddedFields(t *testing.T) {
	t.Parallel()

	tran := transactionWithSkips()

	v1, err := json.Marshal(newTransactionV1(tran))
	require.NoError(t, err)

	canonical, err := json.Marshal(tran)
	require.NoError(t, err)

	var v1Body, canonicalBody map[string]any
	require.NoError(t, json.Unmarshal(v1, &v1Body))
	require.NoError(t, json.Unmarshal(canonical, &canonicalBody))

	for key, want := range canonicalBody {
		if key == "feesSkipped" || key == "tracerSkipped" {
			continue
		}

		assert.Containsf(t, v1Body, key, "the v1 body dropped %q along with the skip fields", key)
		assert.Equalf(t, want, v1Body[key], "the v1 body altered %q", key)
	}

	assert.Len(t, v1Body, len(canonicalBody)-len(skipFieldKeys),
		"the v1 body must differ from the canonical body by exactly the skip fields")
}

// TestTransactionV1_NilStaysNil pins the bodiless answer: a nil transaction must not
// become a non-nil wrapper around nothing, which would serialize as `{}`.
func TestTransactionV1_NilStaysNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newTransactionV1(nil))
}

// paginationWithItems builds the page shape the list core returns, carrying items and
// nothing else — the projection under test reads only Items.
func paginationWithItems(items any) pkgHTTP.Pagination {
	page := pkgHTTP.Pagination{Limit: 10, Page: 1}
	page.SetItems(items)

	return page
}

// TestTransactionV1_ListItemsProjectPage asserts the v1 list re-projects every item.
// The list core is shared with the /v2 mirror read, so the projection happens in the v1
// transport; an unprojected page would leak both keys through the list route while the
// by-id route hid them.
func TestTransactionV1_ListItemsProjectPage(t *testing.T) {
	t.Parallel()

	page := newTransactionV1Items(paginationWithItems([]*transaction.Transaction{transactionWithSkips()}))

	items, ok := page.Items.([]*TransactionV1)
	require.Truef(t, ok, "the page items must be re-projected, got %T", page.Items)
	require.Len(t, items, 1)

	raw, err := json.Marshal(items[0])
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))

	for _, key := range skipFieldKeys {
		assert.NotContainsf(t, body, key, "the v1 list must not publish %q", key)
	}
}

// TestTransactionV1_ListItemsPassThroughForeignPage asserts a page whose items are not
// the concrete type the core sets is returned untouched rather than emptied.
func TestTransactionV1_ListItemsPassThroughForeignPage(t *testing.T) {
	t.Parallel()

	page := newTransactionV1Items(paginationWithItems([]string{"not-a-transaction"}))

	items, ok := page.Items.([]string)
	require.Truef(t, ok, "a foreign page must ride through unchanged, got %T", page.Items)
	assert.Equal(t, []string{"not-a-transaction"}, items)
}

// TestTransactionV1_SchemaWithholdsSkipFields asserts the PUBLISHED contract agrees with
// the body. It reads the REAL assembled document and serializes the component, because
// Huma keeps a hidden property in its in-memory map and drops it only at marshal time —
// so inspecting Schema.Properties directly would report a key clients never see.
//
// The v1 component is looked up as "Transaction", NOT "TransactionV1": the Go type
// carries the version suffix to read as TransactionV2's sibling, while ledgerSchemaNamer
// pins the published name so generated SDKs keep the type they already bind to. A rename
// here would churn every v1 SDK, which is the opposite of this change's point.
func TestTransactionV1_SchemaWithholdsSkipFields(t *testing.T) {
	_, api := buildUnifiedHumaAPI()

	schemas := api.OpenAPI().Components.Schemas.Map()

	v1Schema, ok := schemas["Transaction"]
	require.True(t, ok, "the v1 body must stay published as the \"Transaction\" component")

	_, renamed := schemas["TransactionV1"]
	require.False(t, renamed, "the Go type name must not leak into the published contract")

	v2Schema, ok := schemas["TransactionV2"]
	require.True(t, ok, "the assembled document must publish a TransactionV2 component")

	v1Raw, err := json.Marshal(v1Schema)
	require.NoError(t, err)

	v2Raw, err := json.Marshal(v2Schema)
	require.NoError(t, err)

	var v1Doc, v2Doc struct {
		Properties map[string]any `json:"properties"`
	}

	require.NoError(t, json.Unmarshal(v1Raw, &v1Doc))
	require.NoError(t, json.Unmarshal(v2Raw, &v2Doc))

	// Guard against a vacuous pass on an empty component.
	assert.Contains(t, v1Doc.Properties, "id", "the TransactionV1 component must document the embedded fields")

	for _, key := range skipFieldKeys {
		assert.NotContainsf(t, v1Doc.Properties, key, "the TransactionV1 contract must not advertise %q", key)
		assert.Containsf(t, v2Doc.Properties, key, "the TransactionV2 contract must still advertise %q", key)
	}
}
