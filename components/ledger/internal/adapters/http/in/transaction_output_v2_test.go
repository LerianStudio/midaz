// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// v2TransactionSchemaName is the component name the /v2 response envelope publishes its
// transaction body under. It is spelled literally, distinct from "Transaction" (the v1
// component name), so a rename of the registered Go type shows up here instead of silently
// reintroducing a name collision between v1's and v2's differently-shaped bodies in the
// shared document.
const v2TransactionSchemaName = "TransactionV2"

// buildCanonicalTransactionFixture returns a fully-populated canonical transaction.Transaction,
// the shape newTransactionV2 converts from. Every field is set to a distinct, recognizable value
// so a conversion that drops or mismaps a field is caught by field-by-field assertions rather
// than by two zero values comparing equal.
func buildCanonicalTransactionFixture() *transaction.Transaction {
	amount := decimal.NewFromInt(1500)
	parentID := "11111111-1111-1111-1111-111111111111"
	routeID := "22222222-2222-2222-2222-222222222222"
	statusDescription := "Active status"
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	deletedAt := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	return &transaction.Transaction{
		ID:                       "33333333-3333-3333-3333-333333333333",
		ParentTransactionID:      &parentID,
		Description:              "v2 fixture transaction",
		Status:                   transaction.Status{Code: "APPROVED", Description: &statusDescription},
		Amount:                   &amount,
		AssetCode:                "BRL",
		ChartOfAccountsGroupName: "group-name",
		Source:                   []string{"@person1"},
		Destination:              []string{"@person2"},
		LedgerID:                 "44444444-4444-4444-4444-444444444444",
		OrganizationID:           "55555555-5555-5555-5555-555555555555",
		Route:                    "66666666-6666-6666-6666-666666666666",
		RouteID:                  &routeID,
		FeesSkipped:              true,
		TracerSkipped:            false,
		CreatedAt:                createdAt,
		UpdatedAt:                updatedAt,
		DeletedAt:                &deletedAt,
		Metadata:                 map[string]any{"purpose": "test"},
		Operations:               []*operation.Operation{{ID: "77777777-7777-7777-7777-777777777777"}},
	}
}

// TestNewTransactionV2_RenamesSourceDestinationKeepsEverythingElse proves newTransactionV2 maps
// Source->Debit and Destination->Credit while leaving every other field byte-identical to the
// canonical transaction.Transaction it converts from.
func TestNewTransactionV2_RenamesSourceDestinationKeepsEverythingElse(t *testing.T) {
	t.Parallel()

	canonical := buildCanonicalTransactionFixture()

	got := newTransactionV2(canonical)

	require.NotNil(t, got)
	assert.Equal(t, canonical.Source, got.Debit, "Debit must carry the canonical Source values")
	assert.Equal(t, canonical.Destination, got.Credit, "Credit must carry the canonical Destination values")

	assert.Equal(t, canonical.ID, got.ID)
	assert.Equal(t, canonical.ParentTransactionID, got.ParentTransactionID)
	assert.Equal(t, canonical.Description, got.Description)
	assert.Equal(t, canonical.Status.Code, got.Status.Code)
	assert.Equal(t, canonical.Status.Description, got.Status.Description)
	assert.True(t, canonical.Amount.Equal(*got.Amount))
	assert.Equal(t, canonical.AssetCode, got.AssetCode)
	assert.Equal(t, canonical.LedgerID, got.LedgerID)
	assert.Equal(t, canonical.OrganizationID, got.OrganizationID)
	assert.Equal(t, canonical.RouteID, got.RouteID)
	assert.Equal(t, canonical.FeesSkipped, got.FeesSkipped)
	assert.Equal(t, canonical.TracerSkipped, got.TracerSkipped)
	assert.Equal(t, canonical.CreatedAt, got.CreatedAt)
	assert.Equal(t, canonical.UpdatedAt, got.UpdatedAt)
	assert.Equal(t, canonical.DeletedAt, got.DeletedAt)
	assert.Equal(t, canonical.Metadata, got.Metadata)

	require.Len(t, got.Operations, len(canonical.Operations),
		"every canonical operation must map to an OperationV2")
	assert.Equal(t, canonical.Operations[0].ID, got.Operations[0].ID,
		"the operation ID must survive the OperationV2 mapping")
}

// TestNewTransactionV2_NilInput proves the converter answers nil for a nil canonical
// transaction rather than dereferencing it, matching the nil-guard convention the callers
// (createTransactionV2, the v2 lifecycle shells) rely on when the core returns no transaction.
func TestNewTransactionV2_NilInput(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newTransactionV2(nil))
}

// TestTransactionV2_JSONUsesDebitCreditKeys proves the wire encoding of TransactionV2 carries
// `debit`/`credit` keys and never the v1 `source`/`destination` keys, at the byte level — the
// contract a client actually reads, independent of the OpenAPI schema.
func TestTransactionV2_JSONUsesDebitCreditKeys(t *testing.T) {
	t.Parallel()

	got := newTransactionV2(buildCanonicalTransactionFixture())

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))

	assert.Contains(t, asMap, "debit", "the v2 wire body must carry the debit key")
	assert.Contains(t, asMap, "credit", "the v2 wire body must carry the credit key")
	assert.NotContains(t, asMap, "source", "the v2 wire body must not carry the v1 source key")
	assert.NotContains(t, asMap, "destination", "the v2 wire body must not carry the v1 destination key")
}

// TestRegisterTransactionV2Routes_ResponseSchemaNotNamedTransaction locks the v2 response
// component's name away from "Transaction": v1 and v2 already share 38 identically-shaped
// schema names on the merged docs hub, and a v2 "Transaction" with a DIFFERENT shape (debit/
// credit instead of source/destination) would be the first name shared between the two
// documents with diverging shapes — untested territory for the redocly join that builds the
// hub. The schema must also expose debit/credit and never source/destination.
func TestRegisterTransactionV2Routes_ResponseSchemaNotNamedTransaction(t *testing.T) {
	t.Parallel()

	oapi := registerIsolatedV2TransactionContractForTest()
	schemas := oapi.Components.Schemas.Map()

	schema, ok := schemas[v2TransactionSchemaName]
	require.Truef(t, ok, "v2 contract should publish the response component %s", v2TransactionSchemaName)
	require.NotNil(t, schema)

	assert.Contains(t, schema.Properties, "debit", "%s must document the debit field", v2TransactionSchemaName)
	assert.Contains(t, schema.Properties, "credit", "%s must document the credit field", v2TransactionSchemaName)
	assert.NotContains(t, schema.Properties, "source", "%s must not carry the v1 source field", v2TransactionSchemaName)
	assert.NotContains(t, schema.Properties, "destination", "%s must not carry the v1 destination field", v2TransactionSchemaName)
}

// TestTransactionV2_MirrorsTheCanonicalFieldSet is the drift lock between the canonical
// transaction and its /v2 wire shape.
//
// TransactionV2 is a hand-written mirror, so nothing makes it follow the canonical struct.
// Adding a field to the canonical one publishes it on /v1 and makes it absent from all seven
// /v2 responses, and no other test notices: the shape tests assert the keys they know about,
// and a field nobody wrote an assertion for is invisible to them.
//
// The intended differences this asserts are exactly the two renames (source→debit,
// destination→credit) and the two dropped deprecated fields (chartOfAccountsGroupName, route), so a
// further divergence — an unlisted rename, an unlisted dropped field, a stray `omitempty` that
// changes when a key appears — fails here and names the field. `json:"-"` fields are excluded on
// both sides: they never reach the wire.
func TestTransactionV2_MirrorsTheCanonicalFieldSet(t *testing.T) {
	t.Parallel()

	renames := map[string]string{"source": "debit", "destination": "credit"}
	// Deprecated fields the /v2 wire shape intentionally drops from the canonical transaction.
	dropped := map[string]struct{}{"chartOfAccountsGroupName": {}, "route": {}}

	canonical := wireFieldNames(t, transaction.Transaction{})
	v2 := wireFieldNames(t, TransactionV2{})

	want := make(map[string]struct{}, len(canonical))

	for name := range canonical {
		if _, isDropped := dropped[name]; isDropped {
			continue
		}

		if renamed, ok := renames[name]; ok {
			want[renamed] = struct{}{}

			continue
		}

		want[name] = struct{}{}
	}

	for name := range want {
		assert.Containsf(t, v2, name,
			"the canonical transaction carries %q on the wire and the v2 shape does not: a field added "+
				"to the canonical struct has to be mirrored here or it vanishes from every v2 response", name)
	}

	for name := range v2 {
		assert.Containsf(t, want, name,
			"the v2 shape carries %q and the canonical transaction does not expose it under that name: "+
				"the only intended differences are source→debit, destination→credit, and the dropped "+
				"chartOfAccountsGroupName/route", name)
	}
}

// TestOperationV2_MirrorsTheCanonicalFieldSetMinusDropped is the drift lock between the canonical
// operation.Operation and its /v2 wire shape. OperationV2 is a hand-written mirror, so a field
// added to the canonical operation must be mirrored here or it silently vanishes from every v2
// response. The only intended differences are the two dropped deprecated fields: operation-level
// chartOfAccounts and route. The canonical operation's Snapshot is `json:"-"` so it is off the wire
// on that side and absent from OperationV2 entirely; either way it is outside this comparison.
func TestOperationV2_MirrorsTheCanonicalFieldSetMinusDropped(t *testing.T) {
	t.Parallel()

	dropped := map[string]struct{}{"chartOfAccounts": {}, "route": {}}

	canonical := wireFieldNames(t, operation.Operation{})
	v2 := wireFieldNames(t, OperationV2{})

	want := make(map[string]struct{}, len(canonical))

	for name := range canonical {
		if _, isDropped := dropped[name]; isDropped {
			continue
		}

		want[name] = struct{}{}
	}

	for name := range want {
		assert.Containsf(t, v2, name,
			"the canonical operation carries %q on the wire and the v2 shape does not: mirror it here "+
				"or it vanishes from every v2 response", name)
	}

	for name := range v2 {
		assert.Containsf(t, want, name,
			"the v2 operation shape carries %q and the canonical operation does not expose it under that "+
				"name: the only intended differences are the dropped chartOfAccounts/route", name)
	}
}

// TestUnifiedDocument_V1AndV2TransactionSchemasCoexist locks the shared-registry property at the
// heart of the single-document consolidation: the one huma.API that generates the committed dump
// carries BOTH the v1 "Transaction" component and the v2 "TransactionV2" component together. The
// two bodies differ in shape (source/destination vs debit/credit), so their coexistence under
// distinct names in one component registry is exactly what keeps the merged document consistent —
// a regression that reused a single name for both shapes would drop one key from this map.
func TestUnifiedDocument_V1AndV2TransactionSchemasCoexist(t *testing.T) {
	t.Parallel()

	const v1TransactionSchemaName = "Transaction"

	_, api := buildUnifiedHumaAPI()
	schemas := api.OpenAPI().Components.Schemas.Map()

	v1, okV1 := schemas[v1TransactionSchemaName]
	v2, okV2 := schemas[v2TransactionSchemaName]

	require.Truef(t, okV1, "the unified document must register the v1 %q component", v1TransactionSchemaName)
	require.Truef(t, okV2, "the unified document must register the v2 %q component", v2TransactionSchemaName)
	require.NotNil(t, v1, "%q must resolve to a non-nil schema", v1TransactionSchemaName)
	require.NotNil(t, v2, "%q must resolve to a non-nil schema", v2TransactionSchemaName)
	assert.NotSame(t, v1, v2, "the two coexisting components must be distinct schema objects")
}

// wireFieldNames returns the json key of every field of v that reaches the wire, keyed for
// set comparison. A field tagged `json:"-"` is skipped, and an absent tag falls back to the
// Go field name, which is what encoding/json itself would emit.
func wireFieldNames(t *testing.T, v any) map[string]struct{} {
	t.Helper()

	typ := reflect.TypeOf(v)
	require.Equal(t, reflect.Struct, typ.Kind(), "wireFieldNames needs a struct")

	names := make(map[string]struct{}, typ.NumField())

	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}

		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = field.Name
		}

		names[name] = struct{}{}
	}

	return names
}
