// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// accountHolderKeys are the two response keys that are /v2 ONLY. A v1 client parses a
// body it was written against, so neither may appear on /v1 — in the body OR in the
// published schema, since a contract that advertises a key the body never sends is its
// own defect.
var accountHolderKeys = []string{"holderId", "holderCheckSkipped"}

// accountWithHolder is an account carrying BOTH holder fields set, so an assertion that
// a key is absent cannot pass merely because the value was the zero value.
func accountWithHolder() *mmodel.Account {
	holderID := "00000000-0000-0000-0000-000000000002"

	return &mmodel.Account{
		ID:                 "00000000-0000-0000-0000-000000000001",
		AssetCode:          "BRL",
		Type:               "deposit",
		HolderID:           &holderID,
		HolderCheckSkipped: true,
	}
}

// TestAccountV1_BodyWithholdsHolderFields asserts the /v1 projection emits neither holder
// key while the canonical body of the SAME account emits both. Asserting the pair together
// is what makes this a version boundary rather than a field deletion.
func TestAccountV1_BodyWithholdsHolderFields(t *testing.T) {
	t.Parallel()

	acc := accountWithHolder()

	v1, err := json.Marshal(newAccountV1(acc))
	require.NoError(t, err)

	v2, err := json.Marshal(acc)
	require.NoError(t, err)

	var v1Body, v2Body map[string]any
	require.NoError(t, json.Unmarshal(v1, &v1Body))
	require.NoError(t, json.Unmarshal(v2, &v2Body))

	// Guard against a vacuous pass: an empty body would satisfy every NotContains.
	assert.Contains(t, v1Body, "id", "the v1 body must still carry the embedded fields")

	for _, key := range accountHolderKeys {
		assert.NotContainsf(t, v1Body, key, "/v1 must not publish %q", key)
		assert.Containsf(t, v2Body, key, "/v2 must still publish %q", key)
	}
}

// TestAccountV1_PreservesEmbeddedFields asserts the shadow costs nothing but the two keys:
// every other field of the canonical account still reaches the v1 body. The shadow works by
// winning a field-name conflict, so a mistake there silently drops siblings rather than
// failing loudly.
func TestAccountV1_PreservesEmbeddedFields(t *testing.T) {
	t.Parallel()

	acc := accountWithHolder()

	v1, err := json.Marshal(newAccountV1(acc))
	require.NoError(t, err)

	canonical, err := json.Marshal(acc)
	require.NoError(t, err)

	var v1Body, canonicalBody map[string]any
	require.NoError(t, json.Unmarshal(v1, &v1Body))
	require.NoError(t, json.Unmarshal(canonical, &canonicalBody))

	for key, want := range canonicalBody {
		if key == "holderId" || key == "holderCheckSkipped" {
			continue
		}

		assert.Containsf(t, v1Body, key, "the v1 body dropped %q along with the holder fields", key)
		assert.Equalf(t, want, v1Body[key], "the v1 body altered %q", key)
	}

	assert.Len(t, v1Body, len(canonicalBody)-len(accountHolderKeys),
		"the v1 body must differ from the canonical body by exactly the holder fields")
}

// TestAccountV1_NilStaysNil pins the bodiless answer: a nil account must not become a
// non-nil wrapper around nothing, which would serialize as `{}`.
func TestAccountV1_NilStaysNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newAccountV1(nil))
}

// TestAccountV1_ListItemsProjectPage asserts the v1 list re-projects every item. The list
// core is shared with the /v2 read, so the projection happens in the v1 transport; an
// unprojected page would leak both keys through the list route while the by-id route hid
// them.
func TestAccountV1_ListItemsProjectPage(t *testing.T) {
	t.Parallel()

	page := newAccountV1Items(paginationWithItems([]*mmodel.Account{accountWithHolder()}))

	items, ok := page.Items.([]*AccountV1)
	require.Truef(t, ok, "the page items must be re-projected, got %T", page.Items)
	require.Len(t, items, 1)

	raw, err := json.Marshal(items[0])
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))

	for _, key := range accountHolderKeys {
		assert.NotContainsf(t, body, key, "the v1 list must not publish %q", key)
	}
}

// TestAccountV1_ListItemsPassThroughForeignPage asserts a page whose items are not the
// concrete type the core sets is returned untouched rather than emptied.
func TestAccountV1_ListItemsPassThroughForeignPage(t *testing.T) {
	t.Parallel()

	page := newAccountV1Items(paginationWithItems([]string{"not-an-account"}))

	items, ok := page.Items.([]string)
	require.Truef(t, ok, "a foreign page must ride through unchanged, got %T", page.Items)
	assert.Equal(t, []string{"not-an-account"}, items)
}
