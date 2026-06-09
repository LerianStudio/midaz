// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDecodeMaybeJSON(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]any{"k": "v"}, decodeMaybeJSON([]byte(`{"k":"v"}`)))
	assert.Equal(t, []any{1.0, 2.0}, decodeMaybeJSON([]byte(`[1,2]`)))
	assert.Equal(t, "hello", decodeMaybeJSON([]byte(`"hello"`)))
	// Non-JSON bytes fall back to the raw value.
	raw := []byte("not json")
	assert.Equal(t, raw, decodeMaybeJSON(raw))
	// Non-byte values pass through untouched.
	assert.Equal(t, 42, decodeMaybeJSON(42))
}

func TestSplitQualified(t *testing.T) {
	t.Parallel()

	schema, table := splitQualified("public.accounts")
	assert.Equal(t, "public", schema)
	assert.Equal(t, "accounts", table)

	schema, table = splitQualified("accounts")
	assert.Empty(t, schema)
	assert.Equal(t, "accounts", table)
}

func TestQuoteIdentifier_EscapesQuotes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `"weird""name"`, quoteIdentifier(`weird"name`))
}

func TestBuildPostgresSelect_QualifiedAndUnqualified(t *testing.T) {
	t.Parallel()

	q, _, err := buildPostgresSelect("public.accounts", []string{"id"})
	require.NoError(t, err)
	assert.Contains(t, q, `FROM "public"."accounts"`)

	q, _, err = buildPostgresSelect("accounts", []string{"*"})
	require.NoError(t, err)
	assert.Contains(t, q, `FROM "accounts"`)
	assert.Contains(t, q, "SELECT *")
}

func TestConvertBSONValue(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)

	doc := bson.M{
		"nested": bson.M{"inner": "v"},
		"list":   bson.A{"a", bson.M{"x": 1}},
		"date":   bson.NewDateTimeFromTime(now),
		"plain":  "string",
	}

	out := convertBSON(doc)

	assert.Equal(t, map[string]any{"inner": "v"}, out["nested"])
	assert.Equal(t, []any{"a", map[string]any{"x": 1}}, out["list"])
	assert.Equal(t, now.UTC(), out["date"].(time.Time).UTC())
	assert.Equal(t, "string", out["plain"])
}

func TestConvertBSONValue_DType(t *testing.T) {
	t.Parallel()

	out := convertBSONValue(bson.D{{Key: "k", Value: "v"}})
	assert.Equal(t, map[string]any{"k": "v"}, out)
}

func TestConvertBSONValue_BinaryUUID(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	out := convertBSONValue(bson.Binary{Subtype: 0x04, Data: id[:]})
	assert.Equal(t, id.String(), out)
}

func TestConvertBSONValue_BinaryNonUUIDToHex(t *testing.T) {
	t.Parallel()

	data := []byte{0xde, 0xad, 0xbe, 0xef}

	out := convertBSONValue(bson.Binary{Subtype: 0x00, Data: data})
	assert.Equal(t, hex.EncodeToString(data), out)
}

func TestConvertBSONValue_ObjectID(t *testing.T) {
	t.Parallel()

	oid := bson.NewObjectID()

	out := convertBSONValue(oid)
	assert.Equal(t, oid.Hex(), out)
}

func TestConvertBSONValue_DateTime(t *testing.T) {
	t.Parallel()

	ts := time.UnixMilli(1_700_000_000_000).UTC()

	out := convertBSONValue(bson.NewDateTimeFromTime(ts))
	assert.Equal(t, ts, out.(time.Time).UTC())
}

func TestConvertBSONValue_Nil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, convertBSONValue(nil))
}

func TestRejectUnsupportedFilters(t *testing.T) {
	t.Parallel()

	// A request with no filters for this datasource is accepted.
	require.NoError(t, rejectUnsupportedFilters("ledger", fetcher.ExtractionRequest{}))

	// A filter targeting a different datasource does not block this one.
	require.NoError(t, rejectUnsupportedFilters("ledger", fetcher.ExtractionRequest{
		Filters: map[string]any{"other": map[string]any{"accounts": map[string]any{"status": "ACTIVE"}}},
	}))

	// A filter for this datasource fails closed with a validation category,
	// rather than silently extracting the entire table/collection.
	err := rejectUnsupportedFilters("ledger", fetcher.ExtractionRequest{
		Filters: map[string]any{"ledger": map[string]any{"accounts": map[string]any{"status": "ACTIVE"}}},
	})
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestFilterNestedFields_DropsChildOfSelectedParent(t *testing.T) {
	t.Parallel()

	out := filterNestedFields([]string{"contact", "contact.email", "name"})
	assert.Equal(t, []string{"contact", "name"}, out)
}

func TestMongoProjection_Star(t *testing.T) {
	t.Parallel()

	// A "*" selection sets no projection (builder remains default).
	opts := mongoProjection([]string{"*"})
	assert.NotNil(t, opts)
}

func TestSchemaOverride_AnySlice(t *testing.T) {
	t.Parallel()

	desc := fetcher.ConnectionDescriptor{
		HostAttributes: map[string]any{
			hostAttrSchemas: []any{"public", "audit", 123},
		},
	}

	assert.Equal(t, []string{"public", "audit"}, schemaOverrideFromDescriptor(desc))
}

func TestSchemaOverride_AbsentReturnsNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, schemaOverrideFromDescriptor(fetcher.ConnectionDescriptor{}))
}

func TestSingleTenantResolver_ResolveMongoError(t *testing.T) {
	t.Parallel()

	ds := &fakeSingleTenantDatasources{mongoErr: errors.New("down")}
	r := NewSingleTenantResolver(ds)

	_, err := r.ResolveMongo(context.Background(), "", "crm")
	require.Error(t, err)
}

func TestMultiTenantResolver_ResolveMongoForwardsTenant(t *testing.T) {
	t.Parallel()

	mgr := &fakeMongoManager{err: errors.New("no db")}
	r := NewMultiTenantResolver(&fakePGManager{}, mgr, nil)

	_, err := r.ResolveMongo(context.Background(), "tenant-q", "crm")
	require.Error(t, err)
	assert.Equal(t, "tenant-q", mgr.gotTenant)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryUnavailable, engineErr.Category)
}

func TestClassifyQueryError_TimeoutAndDefault(t *testing.T) {
	t.Parallel()

	timeoutErr := classifyQueryError(context.Background(), "q", context.DeadlineExceeded)
	var engineErr *fetcher.EngineError
	require.ErrorAs(t, timeoutErr, &engineErr)
	assert.Equal(t, fetcher.CategoryTimeout, engineErr.Category)

	other := classifyQueryError(context.Background(), "q", errors.New("boom"))
	require.ErrorAs(t, other, &engineErr)
	assert.Equal(t, fetcher.CategoryUnavailable, engineErr.Category)
}
